package pb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// RouterBand identifies a configured radio; ID is a fixed helper argument, never a shell fragment.
type RouterBand struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Interface string `json:"interface"`
	On        *bool  `json:"on"`
}

// RouterDefinition supplies public metadata and pinned SSH algorithms, without router passwords/control keys.
type RouterDefinition struct {
	LEDControl bool         `json:"led_control"`
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	IP         string       `json:"ip"`
	Port       int          `json:"port"`
	Username   string       `json:"username"`
	Platform   string       `json:"platform"`
	Model      string       `json:"model"`
	Bands      []RouterBand `json:"bands"`
	LEDScope   string       `json:"led_scope"`
	LCD        bool         `json:"lcd"`
	Algorithms []string     `json:"host_key_algorithms"`
}

// RouterReading is a public live diagnostic/control snapshot; nil counters and switches mean unknown.
type RouterReading struct {
	Device      *DeviceEnvelope `json:"device,omitempty"`
	LEDControl  bool            `json:"led_control"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	IP          string          `json:"ip"`
	Model       string          `json:"model"`
	Status      string          `json:"status"`
	Bands       []RouterBand    `json:"bands"`
	LEDs        *bool           `json:"leds"`
	LEDScope    string          `json:"led_scope"`
	LCD         bool            `json:"lcd"`
	LCDOn       *bool           `json:"lcd_on"`
	Firmware    string          `json:"firmware"`
	Uptime      string          `json:"uptime"`
	Load        []string        `json:"load"`
	CPU         *string         `json:"cpu"`
	MemoryTotal *uint64         `json:"memory_total"`
	MemoryFree  *uint64         `json:"memory_free"`
	CheckedAt   int64           `json:"checked_at"`
}

var routerIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
var routerCache = struct {
	sync.Mutex
	Values   []RouterReading
	Snapshot atomic.Pointer[[]RouterReading]
}{}

// routerDefinitions reads a trusted, bounded JSON file; invalid addresses, IDs and SSH algorithms fail closed.
func routerDefinitions() ([]RouterDefinition, error) {
	filename := os.Getenv("UPSNAP_ROUTERS_FILE")
	if filename == "" {
		filename = "/app/ssh/routers.json"
	}
	f, err := os.Open(filename)
	if os.IsNotExist(err) {
		return []RouterDefinition{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var values []RouterDefinition
	if err := json.NewDecoder(io.LimitReader(f, 65536)).Decode(&values); err != nil {
		return nil, err
	}
	if len(values) > 8 {
		return nil, fmt.Errorf("too many routers")
	}
	seen := map[string]bool{}
	for _, r := range values {
		if !routerIDPattern.MatchString(r.ID) || seen[r.ID] || net.ParseIP(r.IP) == nil || r.Port < 1 || r.Port > 65535 || len(r.Algorithms) == 0 {
			return nil, fmt.Errorf("invalid router definition")
		}
		seen[r.ID] = true
		for _, a := range r.Algorithms {
			if a != "ssh-ed25519" && a != "ssh-rsa" && a != "ecdsa-sha2-nistp256" && a != "ecdsa-sha2-nistp384" && a != "ecdsa-sha2-nistp521" {
				return nil, fmt.Errorf("unsupported pinned host key")
			}
		}
	}
	return values, nil
}

// routerPermission checks the collecting Pi's read or power relation for authenticated e; no network work occurs on denial.
func routerPermission(e *core.RequestEvent, power bool) bool {
	host, err := collectorHost(e.App)
	if err != nil {
		return false
	}
	access := deviceAccess(e, host)
	if power {
		return access.Control
	}
	return access.Read
}

// routerCommand invokes only the Pi's forced-command dispatcher with validated args, returning at most 256KiB JSON.
// ctx bounds the entire SSH round trip; errors hide router/controller stderr and credentials.
func routerCommand(ctx context.Context, args ...string) ([]byte, error) {
	cfg := os.Getenv("UPSNAP_STATS_SSH_CONFIG")
	if cfg == "" {
		return nil, fmt.Errorf("router controller not configured")
	}
	command := "router"
	for _, arg := range args {
		if !routerIDPattern.MatchString(arg) {
			return nil, fmt.Errorf("invalid command argument")
		}
		command += " " + arg
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", "-F", cfg, "-o", "ConnectTimeout=3", "192.168.50.152", command).Output()
	if err != nil || len(out) > 262144 {
		return nil, fmt.Errorf("router controller unavailable")
	}
	return out, nil
}

// routerReadLocks deduplicates probes per configured router while unrelated devices refresh independently.
var routerReadLocks sync.Map
var routerActionLock sync.Mutex

// routerLock returns the shared probe/action lock for an allowlisted id; callers release it after use.
func routerLock(id string) *sync.Mutex {
	lock, _ := routerReadLocks.LoadOrStore(id, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// publishRouterReading replaces one observation under a short lock and publishes an immutable inventory snapshot.
// reading must be driver-owned and must not contain a caller-specific Device permission envelope.
func publishRouterReading(reading RouterReading) {
	routerCache.Lock()
	defer routerCache.Unlock()
	values := append([]RouterReading(nil), routerCache.Values...)
	found := false
	for i := range values {
		if values[i].ID == reading.ID {
			values[i] = reading
			found = true
			break
		}
	}
	if !found {
		values = append(values, reading)
	}
	routerCache.Values = values
	routerCache.Snapshot.Store(&values)
}

// readRouter returns a shared observation for def, collecting only that router when stale.
// ctx bounds SSH; force bypasses the 29s cache but retains a two-second minimum interval.
// The per-router lock also prevents an earlier read from overwriting action readback.
func readRouter(ctx context.Context, def RouterDefinition, force bool) (RouterReading, error) {
	lock := routerLock(def.ID)
	lock.Lock()
	defer lock.Unlock()
	for _, cached := range cachedRouterReadings() {
		age := time.Since(time.Unix(cached.CheckedAt, 0))
		if cached.ID == def.ID && age >= 0 && age < 29*time.Second && (!force || age < 2*time.Second) {
			return cached, nil
		}
	}
	out, err := routerCommand(ctx, "status", def.ID)
	if err != nil {
		return RouterReading{}, err
	}
	var reading RouterReading
	if json.Unmarshal(out, &reading) != nil || reading.ID != def.ID {
		return RouterReading{}, fmt.Errorf("invalid router diagnostics")
	}
	reading.Device = nil
	publishRouterReading(reading)
	return reading, nil
}

// HandlerRouters serves the legacy batch or one allowlisted router to authenticated Pi readers.
// A router path parameter enables independent refresh; refresh=1 retains the bounded cache bypass.
// Errors expose no controller stderr and action authorization remains separate.
func HandlerRouters(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	if !routerPermission(e, false) {
		return apis.NewForbiddenError("Networking access denied.", nil)
	}
	defs, err := routerDefinitions()
	if err != nil {
		return apis.NewApiError(503, "Router configuration unavailable.", nil)
	}
	id := e.Request.PathValue("router")
	if id != "" {
		var matched []RouterDefinition
		for _, def := range defs {
			if def.ID == id {
				matched = append(matched, def)
			}
		}
		if len(matched) == 0 {
			return apis.NewNotFoundError("Router unavailable.", nil)
		}
		defs = matched
	}
	values := make([]RouterReading, len(defs))
	errors := make([]error, len(defs))
	var workers sync.WaitGroup
	for i, def := range defs {
		workers.Add(1)
		go func(i int, def RouterDefinition) {
			defer workers.Done()
			values[i], errors[i] = readRouter(e.Request.Context(), def, e.Request.URL.Query().Get("refresh") == "1")
		}(i, def)
	}
	workers.Wait()
	host, _ := collectorHost(e.App)
	access := deviceAccess(e, host)
	for i := range values {
		if errors[i] != nil {
			return apis.NewApiError(503, "Router diagnostics unavailable.", nil)
		}
		common := routerEnvelope(defs[i], &values[i], collectorReference(host), access, time.Now())
		values[i].Device = &common
	}
	if id != "" {
		return e.JSON(200, values[0])
	}
	return e.JSON(200, map[string]any{"routers": values, "can_control": access.Control})
}

// validateRouterAction accepts only supported on/off switches and configured radio IDs; it never executes a command.
func validateRouterAction(r RouterDefinition, action, value, band string) error {
	if value != "on" && value != "off" {
		return fmt.Errorf("invalid switch state")
	}
	if action == "wifi" {
		for _, b := range r.Bands {
			if b.ID == band {
				return nil
			}
		}
		return fmt.Errorf("unsupported radio")
	}
	if band != "" {
		return fmt.Errorf("unexpected radio")
	}
	if action == "lcd" && r.LCD {
		return nil
	}
	if action == "leds" && r.LEDControl && r.Platform != "routeros" {
		return nil
	}
	return fmt.Errorf("unsupported switch")
}

// HandlerRouterAction applies a validated, power-authorized toggle and returns observed state; errors never imply success.
func HandlerRouterAction(e *core.RequestEvent) error {
	if !routerPermission(e, true) {
		return apis.NewForbiddenError("Router control permission required.", nil)
	}
	defs, err := routerDefinitions()
	if err != nil {
		return apis.NewApiError(503, "Router configuration unavailable.", nil)
	}
	var target *RouterDefinition
	for i := range defs {
		if defs[i].ID == e.Request.PathValue("router") {
			target = &defs[i]
		}
	}
	if target == nil {
		return apis.NewNotFoundError("Router unavailable.", nil)
	}
	var body struct {
		Value string `json:"value"`
		Band  string `json:"band"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(e.Response, e.Request.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return apis.NewBadRequestError("Invalid switch request.", nil)
	}
	action := e.Request.PathValue("action")
	if validateRouterAction(*target, action, body.Value, body.Band) != nil {
		return apis.NewBadRequestError("Unsupported router switch.", nil)
	}
	args := []string{target.ID, action, body.Value}
	if action == "wifi" {
		args = append(args, body.Band)
	}
	routerActionLock.Lock()
	defer routerActionLock.Unlock()
	lock := routerLock(target.ID)
	lock.Lock()
	defer lock.Unlock()
	// Invalidate the prior observation before a control with an uncertain outcome.
	for _, cached := range cachedRouterReadings() {
		if cached.ID == target.ID {
			cached.CheckedAt = 0
			publishRouterReading(cached)
		}
	}
	out, err := routerCommand(e.Request.Context(), args...)
	if err != nil {
		return apis.NewApiError(503, "Router operation failed; refresh to check state.", nil)
	}
	var reading RouterReading
	if json.Unmarshal(out, &reading) != nil || reading.ID != target.ID {
		return apis.NewApiError(503, "Router returned an invalid state.", nil)
	}
	reading.Device = nil
	publishRouterReading(reading)
	e.App.Logger().Info("Router control", "user", e.Auth.Id, "router", target.ID, "action", action, "state", body.Value)
	host, _ := collectorHost(e.App)
	if host != nil {
		common := routerEnvelope(*target, &reading, deviceReference("computer", host.Id), deviceAccess(e, host), time.Now())
		reading.Device = &common
	}
	return e.JSON(200, reading)
}

// routerByTarget finds the allowlisted router matching an SSH host:port; unknown targets use the computer policy.
func routerByTarget(target string) *RouterDefinition {
	defs, err := routerDefinitions()
	if err != nil {
		return nil
	}
	for i := range defs {
		if net.JoinHostPort(defs[i].IP, strconv.Itoa(defs[i].Port)) == target {
			return &defs[i]
		}
	}
	return nil
}

// routerTerminalRecord creates unsaved terminal metadata for a configured router after a bounded TCP readiness check.
// No persistent host or power-control record is created; returned records are owned by the caller.
func routerTerminalRecord(app core.App, id string) (*core.Record, string, error) {
	defs, err := routerDefinitions()
	if err != nil {
		return nil, "", err
	}
	for _, r := range defs {
		if r.ID == id {
			target := net.JoinHostPort(r.IP, strconv.Itoa(r.Port))
			conn, err := net.DialTimeout("tcp", target, 3*time.Second)
			if err != nil {
				return nil, "", fmt.Errorf("router SSH unavailable")
			}
			conn.Close()
			collection, err := app.FindCollectionByNameOrId("devices")
			if err != nil {
				return nil, "", err
			}
			d := core.NewRecord(collection)
			d.Id = r.ID
			d.Set("name", r.Name)
			d.Set("ip", r.IP)
			d.Set("status", "online")
			return d, target, nil
		}
	}
	return nil, "", nil
}
