package pb

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// DeviceAccess describes granted permissions, independently of hardware support; commands still enforce their own policy.
type DeviceAccess struct {
	Read    bool `json:"read"`
	Control bool `json:"control"`
}

// DeviceEnvelope is the version-one common device contract. IDs survive address/name changes;
// checked_at is UTC RFC3339 or null when the driver has no persisted observation timestamp.
// Capabilities describe supported features, never authorization or current availability.
type DeviceEnvelope struct {
	ID                string       `json:"id"`
	Driver            string       `json:"driver"`
	DriverID          string       `json:"driver_id"`
	Category          string       `json:"category"`
	Name              string       `json:"name"`
	Address           string       `json:"address"`
	CollectorDeviceID string       `json:"collector_device_id,omitempty"`
	State             string       `json:"state"`
	CheckedAt         *time.Time   `json:"checked_at"`
	Capabilities      []string     `json:"capabilities"`
	Permissions       DeviceAccess `json:"permissions"`
}

// deviceReference returns a namespaced stable identifier for a trusted driver and its persistent id.
func deviceReference(driver, id string) string { return driver + ":" + id }

// normalizedState maps legacy driver states to the common vocabulary; unrecognized values are unknown.
func normalizedState(value string) string {
	switch value {
	case "up", "online":
		return "online"
	case "down", "offline":
		return "offline"
	case "pending", "unknown", "stale", "degraded", "disabled":
		return value
	default:
		return "unknown"
	}
}

// observationTime returns a UTC timestamp for a real observation, or nil for a missing timestamp.
func observationTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

// observationState marks missing, future (>10s) or old (>90s) diagnostic observations stale at now.
func observationState(value string, checked *time.Time, now time.Time) string {
	if checked == nil || now.Sub(*checked) > 90*time.Second || checked.After(now.Add(10*time.Second)) {
		return "stale"
	}
	return normalizedState(value)
}

// inventoryHost resolves an explicit computer reference; only absent references may use a legacy IP.
// Invalid or deleted explicit IDs fail closed rather than binding to a replacement at the same address.
func inventoryHost(app core.App, reference, legacyIP string) (*core.Record, error) {
	if reference != "" {
		if !strings.HasPrefix(reference, "computer:") {
			return nil, fmt.Errorf("invalid computer reference")
		}
		return app.FindRecordById("devices", strings.TrimPrefix(reference, "computer:"))
	}
	return app.FindFirstRecordByData("devices", "ip", legacyIP)
}

// deviceAccess reports read/control grants for host and authenticated e; nil hosts never grant access.
func deviceAccess(e *core.RequestEvent, host *core.Record) DeviceAccess {
	if host == nil || e.Auth == nil {
		return DeviceAccess{}
	}
	if e.HasSuperuserAuth() {
		return DeviceAccess{true, true}
	}
	result := DeviceAccess{}
	for field, target := range map[string]*bool{"read": &result.Read, "power": &result.Control} {
		_, err := e.App.FindFirstRecordByFilter("permissions", "user.id = {:user} && "+field+".id ?= {:host}", dbx.Params{"user": e.Auth.Id, "host": host.Id})
		*target = err == nil
	}
	return result
}

// collectorHost resolves the configured Pi computer ID, with an IP fallback for pre-upgrade installations.
func collectorHost(app core.App) (*core.Record, error) {
	return inventoryHost(app, os.Getenv("UPSNAP_COLLECTOR_DEVICE_ID"), "192.168.50.152")
}

// computerEnvelope adapts a PocketBase record without exposing command strings, credentials or driver-owned data.
func computerEnvelope(e *core.RequestEvent, host *core.Record) DeviceEnvelope {
	caps := []string{"resources", "terminal"}
	if !host.GetBool("reboot_only") {
		if host.GetString("mac") != "" || host.GetString("wake_cmd") != "" {
			caps = append(caps, "wake")
		}
		if host.GetString("shutdown_cmd") != "" {
			caps = append(caps, "suspend")
		}
	}
	if host.GetBool("reboot_only") || host.GetString("shutdown_cmd") != "" {
		caps = append(caps, "reboot")
	}
	if host.GetBool("boot_managed") {
		caps = append(caps, "boot")
	}
	return DeviceEnvelope{ID: deviceReference("computer", host.Id), Driver: "computer", DriverID: host.Id, Category: "computers", Name: host.GetString("name"), Address: host.GetString("ip"), State: normalizedState(host.GetString("status")), Capabilities: caps, Permissions: deviceAccess(e, host)}
}

// routerEnvelope adapts trusted router metadata plus optional latest diagnostics; it never probes or changes hardware.
func routerEnvelope(def RouterDefinition, reading *RouterReading, collector string, access DeviceAccess, now time.Time) DeviceEnvelope {
	caps := []string{"resources", "terminal"}
	if len(def.Bands) > 0 {
		caps = append(caps, "wifi")
	}
	if def.LEDControl {
		caps = append(caps, "leds")
	}
	if def.LCD {
		caps = append(caps, "lcd")
	}
	value := DeviceEnvelope{ID: deviceReference("router", def.ID), Driver: "router", DriverID: def.ID, Category: "networking", Name: def.Name, Address: def.IP, CollectorDeviceID: collector, State: "unknown", Capabilities: caps, Permissions: access}
	if reading != nil {
		if reading.CheckedAt > 0 {
			value.CheckedAt = observationTime(time.Unix(reading.CheckedAt, 0))
		}
		value.State = observationState(reading.Status, value.CheckedAt, now)
	}
	return value
}

// powerEnvelope adapts read-only plug diagnostics, retaining identity even when stale or the relay is off.
func powerEnvelope(plug PowerDevice, collector string, access DeviceAccess, now time.Time) DeviceEnvelope {
	checked := observationTime(plug.CheckedAt)
	return DeviceEnvelope{ID: deviceReference("appliance", plug.ID), Driver: "appliance", DriverID: plug.ID, Category: "home_appliances", Name: plug.Name, Address: plug.IP, CollectorDeviceID: collector, State: observationState(plug.Status, checked, now), CheckedAt: checked, Capabilities: []string{"power_metrics", "power_history"}, Permissions: DeviceAccess{Read: access.Read}}
}

// inventoryAppliances merges optional public definitions with observations at now (UTC).
// Definitions in UPSNAP_APPLIANCES_FILE (default /app/ssh/appliances.json) own identity
// and permission inheritance; snapshot-only legacy installs remain supported when absent.
// A snapshot failure returns configured devices plus an error, without inventing readings.
func inventoryAppliances(now time.Time) ([]PowerDevice, error) {
	var snapshot PowerSnapshot
	var snapshotErr error
	if filename := os.Getenv("UPSNAP_SERVICES_FILE"); filename != "" {
		snapshot, snapshotErr = readPowerSnapshot(filepath.Join(filepath.Dir(filename), "power.json"), now)
	}
	filename := os.Getenv("UPSNAP_APPLIANCES_FILE")
	if filename == "" {
		filename = "/app/ssh/appliances.json"
	}
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		return snapshot.Devices, snapshotErr
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var defs []struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		IP                string `json:"ip"`
		CollectorDeviceID string `json:"collector_device_id"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	if decoder.Decode(&defs) != nil || len(defs) > 32 {
		return nil, fmt.Errorf("invalid appliance definitions")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("invalid appliance definitions")
	}
	values := make([]PowerDevice, 0, len(defs))
	seen := map[string]bool{}
	for _, def := range defs {
		if !routerIDPattern.MatchString(def.ID) || seen[def.ID] || def.Name == "" || net.ParseIP(def.IP) == nil || !strings.HasPrefix(def.CollectorDeviceID, "computer:") {
			return nil, fmt.Errorf("invalid appliance definition")
		}
		seen[def.ID] = true
		plug := PowerDevice{ID: def.ID, Name: def.Name, IP: def.IP, CollectorDeviceID: def.CollectorDeviceID}
		for _, reading := range snapshot.Devices {
			if reading.ID == def.ID {
				plug = reading
				break
			}
		}
		plug.Name, plug.IP, plug.CollectorDeviceID = def.Name, def.IP, def.CollectorDeviceID
		values = append(values, plug)
	}
	return values, snapshotErr
}

// deviceInventory returns authorized common records and per-driver errors without triggering SSH, WoL or device mutations.
// One unavailable optional driver does not hide computer records or leak ungranted metadata.
func deviceInventory(e *core.RequestEvent, now time.Time) ([]DeviceEnvelope, []string, error) {
	records, err := e.App.FindAllRecords("devices")
	if err != nil {
		return nil, nil, err
	}
	values := []DeviceEnvelope{}
	failures := []string{}
	for _, record := range records {
		value := computerEnvelope(e, record)
		if value.Permissions.Read {
			values = append(values, value)
		}
	}
	collector, collectorErr := collectorHost(e.App)
	if collectorErr == nil {
		access := deviceAccess(e, collector)
		if access.Read {
			collectorID := deviceReference("computer", collector.Id)
			defs, err := routerDefinitions()
			if err != nil {
				failures = append(failures, "router")
			} else {
				cached := cachedRouterReadings()
				for _, def := range defs {
					var reading *RouterReading
					for i := range cached {
						if cached[i].ID == def.ID {
							reading = &cached[i]
							break
						}
					}
					values = append(values, routerEnvelope(def, reading, collectorID, access, now))
				}
			}
		}
	}
	plugs, err := inventoryAppliances(now)
	if err != nil {
		failures = append(failures, "appliance")
	}
	for _, plug := range plugs {
		host, err := inventoryHost(e.App, plug.CollectorDeviceID, plug.HostIP)
		if err != nil {
			continue
		}
		permission := deviceAccess(e, host)
		if permission.Read {
			common := powerEnvelope(plug, deviceReference("computer", host.Id), permission, now)
			if plug.CheckedAt.IsZero() {
				common.State = "unknown"
			}
			values = append(values, common)
		}
	}
	return values, failures, nil
}

// HandlerDeviceInventory serves the versioned, read-filtered common inventory to authenticated e.
func HandlerDeviceInventory(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	now := time.Now().UTC()
	devices, failures, err := deviceInventory(e, now)
	if err != nil {
		return apis.NewApiError(503, "Device inventory unavailable.", nil)
	}
	return e.JSON(200, map[string]any{"schema_version": 1, "checked_at": now, "devices": devices, "unavailable_drivers": failures})
}

// collectorReference returns host's stable computer ID, or an empty reference for missing collector metadata.
func collectorReference(host *core.Record) string {
	if host == nil {
		return ""
	}
	return deviceReference("computer", host.Id)
}

// cachedRouterReadings returns the last immutable diagnostic snapshot without waiting for in-flight SSH/actions.
// Before the first observation it returns nil, allowing unrelated inventory/service reads to complete promptly.
func cachedRouterReadings() []RouterReading {
	if value := routerCache.Snapshot.Load(); value != nil {
		return *value
	}
	return nil
}
