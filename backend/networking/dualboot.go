package networking

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type BootProfile struct {
	Alias           string `json:"alias"`
	Username        string `json:"username"`
	HelperInstalled bool   `json:"helper_installed"`
}
type BootHost struct {
	Profiles       map[string]BootProfile `json:"profiles"`
	WritesEnabled  bool                   `json:"writes_enabled"`
	RestartEnabled bool                   `json:"restart_enabled"`
}
type BootTarget struct {
	Available bool   `json:"available"`
	Entry     string `json:"entry"`
	Path      string `json:"path"`
}
type BootStatus struct {
	OS                 string                `json:"os"`
	Ready              bool                  `json:"ready"`
	WritesEnabled      bool                  `json:"writes_enabled"`
	RestartEnabled     bool                  `json:"restart_enabled"`
	Targets            map[string]BootTarget `json:"targets"`
	BootNext           *string               `json:"boot_next"`
	BootOrder          string                `json:"boot_order"`
	LoaderOrder        string                `json:"loader_order"`
	BootID             string                `json:"boot_id"`
	ValidatedTarget    string                `json:"validated_target,omitempty"`
	BootOrderUnchanged bool                  `json:"boot_order_unchanged"`
	Message            string                `json:"message,omitempty"`
	RestartQueued      bool                  `json:"restart_queued,omitempty"`
}

var profileAlias = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var bootDetect = struct {
	sync.Mutex
	ip      string
	os      string
	expires time.Time
}{}

// BootConfiguration reads the trusted host map for literal ip; nil means unmanaged, errors fail closed.
// Returned configuration is newly allocated. No request-controlled alias or command is accepted.
func BootConfiguration(ip string) (*BootHost, error) {
	path := os.Getenv("UPSNAP_BOOT_HOSTS_FILE")
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var hosts map[string]BootHost
	if err = json.Unmarshal(data, &hosts); err != nil {
		return nil, err
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		ip = parsed.String()
	} else {
		resolved, resolveErr := ResolveToIPAddr(ip)
		if resolveErr != nil {
			return nil, resolveErr
		}
		ip = resolved
	}
	host, ok := hosts[ip]
	if !ok {
		return nil, nil
	}
	for _, name := range []string{"windows", "ubuntu"} {
		p, exists := host.Profiles[name]
		if !exists || !profileAlias.MatchString(p.Alias) || p.Username == "" {
			return nil, errors.New("invalid boot profile configuration")
		}
	}
	return &host, nil
}

// runBootProfile runs one fixed action through server-configured SSH alias p; ctx bounds total runtime.
// Returns owned stdout bytes or an error without forwarding SSH diagnostics or accepting shell commands.
func runBootProfile(ctx context.Context, p BootProfile, action string) ([]byte, error) {
	switch action {
	case "check", "boot-status", "validate-windows", "validate-ubuntu", "prepare-windows", "prepare-ubuntu", "restart-windows", "restart-ubuntu":
	default:
		return nil, errors.New("invalid boot action")
	}
	config := os.Getenv("UPSNAP_STATS_SSH_CONFIG")
	if config == "" {
		return nil, errors.New("SSH profiles unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ssh", "-F", config, "-o", "BatchMode=yes", "-o", "ConnectTimeout=2", p.Alias, action).Output()
}

// DetectBootOS identifies the authenticated OS at ip using pinned profiles; returns windows/ubuntu or an error.
// ctx bounds SSH. Successful results are cached for three seconds; no WoL or power command is issued.
func DetectBootOS(ctx context.Context, ip string) (string, error) {
	host, err := BootConfiguration(ip)
	if err != nil {
		return "", err
	}
	if host == nil {
		return "", errors.New("host is not boot-managed")
	}
	bootDetect.Lock()
	defer bootDetect.Unlock()
	if bootDetect.ip == ip && time.Now().Before(bootDetect.expires) {
		return bootDetect.os, nil
	}
	for _, name := range []string{"windows", "ubuntu"} {
		data, err := runBootProfile(ctx, host.Profiles[name], "check")
		if err != nil {
			continue
		}
		var result BootStatus
		valid := json.Unmarshal(data, &result) == nil && result.Ready && result.OS == name
		if name == "ubuntu" && strings.TrimSpace(string(data)) == "upsnap-suspend-ready" {
			valid = true
		}
		if valid {
			bootDetect.ip = ip
			bootDetect.os = name
			bootDetect.expires = time.Now().Add(3 * time.Second)
			return name, nil
		}
	}
	return "", errors.New("neither OS has authenticated SSH readiness")
}

// ReadBootStatus returns OS/loader capabilities for ip; ctx bounds SSH and no firmware or power state changes.
// Missing helpers return a known OS with explanatory message; unmanaged or unreachable hosts return errors.
func ReadBootStatus(ctx context.Context, ip string) (*BootStatus, error) {
	host, err := BootConfiguration(ip)
	if err != nil {
		return nil, err
	}
	if host == nil {
		return nil, errors.New("host is not boot-managed")
	}
	name, err := DetectBootOS(ctx, ip)
	if err != nil {
		return nil, err
	}
	data, err := runBootProfile(ctx, host.Profiles[name], "boot-status")
	if err != nil {
		return &BootStatus{OS: name, Ready: true, Message: "Boot helper is not installed or firmware inspection failed.", Targets: map[string]BootTarget{}}, nil
	}
	var status BootStatus
	if err = json.Unmarshal(data, &status); err != nil || status.OS != name || !status.Ready {
		return nil, errors.New("invalid boot helper response")
	}
	status.WritesEnabled = status.WritesEnabled && host.WritesEnabled
	status.LoaderOrder = bootLoaderOrder(&status)
	status.RestartEnabled = status.RestartEnabled && status.WritesEnabled && host.RestartEnabled && host.Profiles["windows"].HelperInstalled && host.Profiles["ubuntu"].HelperInstalled
	if !host.Profiles["windows"].HelperInstalled {
		status.Message = "Windows helper installation is pending; cross-OS transition checks are not enabled."
	}
	return &status, nil
}

// bootLoaderOrder projects raw EFI BootOrder onto verified Ubuntu/Windows IDs in status; malformed bytes return empty.
// Returns an owned comma-separated sequence, ignoring firmware-generated legacy BBS entries across OS boots.
func bootLoaderOrder(status *BootStatus) string {
	order, err := base64.StdEncoding.DecodeString(status.BootOrder)
	if err != nil || len(order) == 0 || len(order)%2 != 0 {
		return ""
	}
	ids := map[string]bool{}
	for _, target := range status.Targets {
		if target.Available {
			ids[target.Entry] = true
		}
	}
	var result []string
	for i := 0; i < len(order); i += 2 {
		id := fmt.Sprintf("%04X", uint16(order[i])|uint16(order[i+1])<<8)
		if ids[id] {
			result = append(result, id)
		}
	}
	return strings.Join(result, ",")
}

// CheckBootTarget validates or prepares target (windows/ubuntu) on ip; prepare writes only when both gates permit.
// ctx bounds SSH; result is verified helper JSON. This function never restarts, suspends, or sends WoL.
func CheckBootTarget(ctx context.Context, ip, target string, prepare bool) (*BootStatus, error) {
	if target != "windows" && target != "ubuntu" {
		return nil, errors.New("unsupported boot target")
	}
	host, err := BootConfiguration(ip)
	if err != nil {
		return nil, err
	}
	if host == nil {
		return nil, errors.New("host is not boot-managed")
	}
	if prepare && !host.WritesEnabled {
		return nil, errors.New("boot selection is locked pending explicit authorization")
	}
	status, err := ReadBootStatus(ctx, ip)
	if err != nil {
		return nil, err
	}
	if !status.Targets[target].Available {
		return nil, errors.New("target firmware loader is missing or ambiguous")
	}
	if prepare && !status.WritesEnabled {
		return nil, errors.New("host firmware writes are locked")
	}
	action := "validate-" + target
	if prepare {
		action = "prepare-" + target
	}
	data, err := runBootProfile(ctx, host.Profiles[status.OS], action)
	if err != nil {
		return nil, fmt.Errorf("boot helper refused %s", action)
	}
	var result BootStatus
	if err = json.Unmarshal(data, &result); err != nil || result.OS != status.OS || !result.Ready || result.ValidatedTarget != target || !result.BootOrderUnchanged || result.BootOrder != status.BootOrder {
		return nil, errors.New("boot target verification failed")
	}
	if prepare && (result.BootNext == nil || *result.BootNext != result.Targets[target].Entry) {
		return nil, errors.New("BootNext read-back mismatch")
	}
	result.RestartEnabled = false
	result.WritesEnabled = result.WritesEnabled && host.WritesEnabled
	return &result, nil
}

// RestartBootTarget queues one graceful restart to target on ip, requiring matching source bootID/order and BootNext.
// ctx bounds SSH; returns verified queue acknowledgement or error. Never retries or falls back to power cycling.
func RestartBootTarget(ctx context.Context, ip, target, bootID, order string) (*BootStatus, error) {
	if target != "ubuntu" && target != "windows" {
		return nil, errors.New("unsupported boot target")
	}
	host, err := BootConfiguration(ip)
	if err != nil || host == nil || !host.WritesEnabled || !host.RestartEnabled {
		return nil, errors.New("restart execution is locked")
	}
	status, err := ReadBootStatus(ctx, ip)
	if err != nil {
		return nil, err
	}
	if !status.RestartEnabled || status.BootID == "" || status.BootID != bootID || status.BootOrder != order || status.BootNext == nil || *status.BootNext != status.Targets[target].Entry || !status.Targets[target].Available {
		return nil, errors.New("source identity or prepared boot selection changed; restart refused")
	}
	data, err := runBootProfile(ctx, host.Profiles[status.OS], "restart-"+target)
	if err != nil {
		return nil, errors.New("graceful restart request failed; inspect BootNext before retrying")
	}
	var result BootStatus
	if json.Unmarshal(data, &result) != nil || !result.RestartQueued || !result.BootOrderUnchanged || result.BootOrder != order || result.OS != status.OS || result.BootID != bootID || result.BootNext == nil || *result.BootNext != status.Targets[target].Entry {
		return nil, errors.New("restart acknowledgement is uncertain; do not retry automatically")
	}
	return &result, nil
}

// CheckSuspendSupport refuses legacy Linux power commands on a managed non-Ubuntu or unknown OS.
// ctx bounds SSH; ip is the device's configured address. Nil means the existing suspend path may continue.
func CheckSuspendSupport(ctx context.Context, ip string) error {
	host, err := BootConfiguration(ip)
	if err != nil {
		return err
	}
	if host == nil {
		return nil
	}
	name, err := DetectBootOS(ctx, ip)
	if err != nil || name != "ubuntu" {
		return errors.New("suspend is not yet supported on this operating system")
	}
	status, err := ReadBootStatus(ctx, ip)
	if err != nil || status.BootNext != nil {
		return errors.New("suspend is unavailable while a boot selection is pending")
	}
	return nil
}
