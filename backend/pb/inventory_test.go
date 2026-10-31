package pb

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// inventoryFixture returns isolated app/user/hosts with read access only to the first host; t owns cleanup.
func inventoryFixture(t *testing.T) (core.App, *core.Record, *core.Record, *core.Record) {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	hosts := core.NewBaseCollection("devices")
	for _, name := range []string{"name", "ip", "status", "mac", "wake_cmd", "shutdown_cmd"} {
		hosts.Fields.Add(&core.TextField{Name: name})
	}
	hosts.Fields.Add(&core.BoolField{Name: "reboot_only"})
	if err := app.Save(hosts); err != nil {
		t.Fatal(err)
	}
	values := []*core.Record{}
	for _, ip := range []string{"192.168.50.152", "192.168.50.150"} {
		h := core.NewRecord(hosts)
		h.Set("ip", ip)
		h.Set("name", ip)
		h.Set("status", "online")
		h.Set("mac", "00:11:22:33:44:55")
		h.Set("shutdown_cmd", "suspend")
		if err := app.Save(h); err != nil {
			t.Fatal(err)
		}
		values = append(values, h)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("inventory@example.invalid")
	user.SetPassword("test-password-123")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	grants := core.NewBaseCollection("permissions")
	grants.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1}, &core.RelationField{Name: "read", CollectionId: hosts.Id, MaxSelect: 10}, &core.RelationField{Name: "power", CollectionId: hosts.Id, MaxSelect: 10})
	if err := app.Save(grants); err != nil {
		t.Fatal(err)
	}
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("read", []string{values[0].Id})
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPSNAP_COLLECTOR_DEVICE_ID", deviceReference("computer", values[0].Id))
	writeRouterFixture(t, []RouterDefinition{})
	return app, user, values[0], values[1]
}

// TestInventoryStableIdentityAndCapabilities separates mutable addresses from identity, support and permissions.
func TestInventoryStableIdentityAndCapabilities(t *testing.T) {
	app, user, host, _ := inventoryFixture(t)
	e := &core.RequestEvent{App: app, Auth: user}
	before := computerEnvelope(e, host)
	host.Set("ip", "10.1.2.3")
	host.Set("name", "Renamed")
	host.Set("reboot_only", true)
	after := computerEnvelope(e, host)
	if before.ID != after.ID || before.Address == after.Address || after.Name != "Renamed" {
		t.Fatal("mutable identity")
	}
	if !before.Permissions.Read || before.Permissions.Control || !slices.Contains(before.Capabilities, "wake") || slices.Contains(after.Capabilities, "wake") || slices.Contains(after.Capabilities, "suspend") || !slices.Contains(after.Capabilities, "reboot") {
		t.Fatalf("incorrect support/grants: %+v %+v", before, after)
	}
	raw, _ := json.Marshal(after)
	var public map[string]any
	json.Unmarshal(raw, &public)
	if _, ok := public["shutdown_cmd"]; ok || public["checked_at"] != nil {
		t.Fatal("secret or invented check timestamp")
	}
	if err := app.Save(host); err != nil {
		t.Fatal(err)
	}
	if resolved, err := collectorHost(app); err != nil || resolved.Id != host.Id {
		t.Fatal("collector must survive address changes")
	}
	if _, err := inventoryHost(app, "computer:missing", host.GetString("ip")); err == nil {
		t.Fatal("explicit missing identity fell back to IP")
	}
}

// TestInventoryAdaptersNormalizeFreshnessAndReadOnlyPower keeps null/old times and powered-off relays distinct.
func TestInventoryAdaptersNormalizeFreshnessAndReadOnlyPower(t *testing.T) {
	now := time.Now()
	d := RouterDefinition{ID: "router", LEDControl: true, Bands: []RouterBand{{ID: "0"}}}
	reading := RouterReading{Status: "online", CheckedAt: now.Add(-2 * time.Minute).Unix()}
	common := routerEnvelope(d, &reading, "computer:collector", DeviceAccess{true, false}, now)
	if common.State != "stale" || common.CheckedAt.Location() != time.UTC || !slices.Contains(common.Capabilities, "wifi") || slices.Contains(common.Capabilities, "wake") {
		t.Fatalf("bad router: %+v", common)
	}
	if routerEnvelope(d, nil, "", DeviceAccess{}, now).State != "unknown" {
		t.Fatal("unsampled router has invented state")
	}
	off := false
	plug := PowerDevice{ID: "plug", Status: "up", CheckedAt: now, Reading: PlugReading{Relay: &off}}
	power := powerEnvelope(plug, "computer:collector", DeviceAccess{true, true}, now)
	if power.State != "online" || power.Permissions.Control || slices.Contains(power.Capabilities, "suspend") {
		t.Fatalf("bad plug: %+v", power)
	}
	plug.CheckedAt = now.Add(time.Minute)
	if powerEnvelope(plug, "", DeviceAccess{}, now).State != "stale" {
		t.Fatal("future reading accepted")
	}
	for input, want := range map[string]string{"up": "online", "down": "offline", "pending": "pending", "disabled": "disabled", "nonsense": "unknown"} {
		if normalizedState(input) != want {
			t.Fatal(input)
		}
	}
}

// TestServicesJoinStableIDsWithoutPermissionOrIPFallback verifies renamed/readdressed and virtual-host checks.
func TestServicesJoinStableIDsWithoutPermissionOrIPFallback(t *testing.T) {
	app, user, allowed, hidden := inventoryFixture(t)
	now := time.Now().UTC()
	folder := t.TempDir()
	t.Setenv("UPSNAP_SERVICES_FILE", filepath.Join(folder, "status.json"))
	writeRouterFixture(t, []RouterDefinition{{ID: "router", Name: "Router", IP: "192.168.50.1", Port: 22, Algorithms: []string{"ssh-ed25519"}}})
	oldIP := allowed.GetString("ip")
	allowed.Set("ip", "10.0.0.9")
	allowed.Set("name", "New name")
	if err := app.Save(allowed); err != nil {
		t.Fatal(err)
	}
	values := ServiceSnapshot{CheckedAt: now, Services: []ServiceReading{
		{ID: "stable", DeviceID: deviceReference("computer", allowed.Id), HostIP: oldIP, Status: "up", CheckedAt: now},
		{ID: "hidden", DeviceID: deviceReference("computer", hidden.Id), HostIP: allowed.GetString("ip"), Status: "up", CheckedAt: now},
		{ID: "invalid", DeviceID: "computer:missing", HostIP: allowed.GetString("ip"), Status: "up", CheckedAt: now},
		{ID: "legacy", HostIP: allowed.GetString("ip"), Status: "up", CheckedAt: now},
		{ID: "router", DeviceID: "router:router", HostIP: oldIP, Status: "up", CheckedAt: now},
	}}
	data, _ := json.Marshal(values)
	if err := os.WriteFile(filepath.Join(folder, "status.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	e := &core.RequestEvent{App: app, Auth: user, Event: router.Event{Request: httptest.NewRequest("GET", "/api/upsnap/services", nil), Response: response}}
	if err := HandlerServices(e); err != nil {
		t.Fatal(err)
	}
	var result ServiceSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 3 {
		t.Fatalf("wrong visibility: %s", response.Body.String())
	}
	for _, s := range result.Services {
		if s.State != "online" {
			t.Fatal("legacy status not normalized")
		}
		if s.ID == "stable" && (s.HostIP != "10.0.0.9" || s.HostName != "New name") {
			t.Fatal("stale IP/name relation")
		}
	}
	inventory, _, err := deviceInventory(e, now)
	if err != nil || len(inventory) != 2 {
		t.Fatalf("unexpected inventory %+v %v", inventory, err)
	}
}

// TestPowerCollectorIDSurvivesReaddressing rejects an unknown explicit collector without IP-based grant inheritance.
func TestPowerCollectorIDSurvivesReaddressing(t *testing.T) {
	app, user, host, _ := inventoryFixture(t)
	now := time.Now().UTC()
	folder := t.TempDir()
	t.Setenv("UPSNAP_SERVICES_FILE", filepath.Join(folder, "status.json"))
	old := host.GetString("ip")
	host.Set("ip", "10.0.0.8")
	if err := app.Save(host); err != nil {
		t.Fatal(err)
	}
	snapshot := PowerSnapshot{CheckedAt: now, Devices: []PowerDevice{{ID: "plug", CollectorDeviceID: deviceReference("computer", host.Id), HostIP: old, Status: "up", CheckedAt: now}, {ID: "bad", CollectorDeviceID: "computer:missing", HostIP: host.GetString("ip"), Status: "up", CheckedAt: now}}}
	data, _ := json.Marshal(snapshot)
	if err := os.WriteFile(filepath.Join(folder, "power.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	e := &core.RequestEvent{App: app, Auth: user, Event: router.Event{Request: httptest.NewRequest("GET", "/api/upsnap/power", nil), Response: response}}
	if err := HandlerPower(e); err != nil {
		t.Fatal(err)
	}
	var result PowerSnapshot
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.Devices) != 1 || result.Devices[0].Device.ID != "appliance:plug" || result.Devices[0].Device.Permissions.Control {
		t.Fatalf("bad snapshot %s", response.Body.String())
	}
}

// TestInventoryCacheDoesNotWaitForRouterSSH keeps unrelated inventory reads responsive during a slow driver probe.
func TestInventoryCacheDoesNotWaitForRouterSSH(t *testing.T) {
	previous := routerCache.Snapshot.Load()
	t.Cleanup(func() { routerCache.Snapshot.Store(previous) })
	values := []RouterReading{{ID: "cached", Status: "online"}}
	routerCache.Snapshot.Store(&values)
	routerCache.Lock()
	defer routerCache.Unlock()
	result := cachedRouterReadings()
	if len(result) != 1 || result[0].ID != "cached" {
		t.Fatal("missing independent snapshot")
	}
}

// TestConfiguredAppliancesSurviveSnapshotFailure keeps authorized definitions visible when power.json is absent/corrupt.
func TestConfiguredAppliancesSurviveSnapshotFailure(t *testing.T) {
	app, user, host, hidden := inventoryFixture(t)
	dir := t.TempDir()
	t.Setenv("UPSNAP_SERVICES_FILE", filepath.Join(dir, "status.json"))
	filename := filepath.Join(dir, "appliances.json")
	t.Setenv("UPSNAP_APPLIANCES_FILE", filename)
	defs := []map[string]string{
		{"id": "visible", "name": "Plug", "ip": "192.0.2.1", "collector_device_id": deviceReference("computer", host.Id)},
		{"id": "hidden", "name": "Private", "ip": "192.0.2.2", "collector_device_id": deviceReference("computer", hidden.Id)},
	}
	data, _ := json.Marshal(defs)
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	e := &core.RequestEvent{App: app, Auth: user}
	for _, corrupt := range []bool{false, true} {
		if corrupt {
			if err := os.WriteFile(filepath.Join(dir, "power.json"), []byte("broken"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		values, failures, err := deviceInventory(e, time.Now())
		if err != nil || !slices.Contains(failures, "appliance") {
			t.Fatalf("missing partial failure: %v %v", failures, err)
		}
		if len(values) != 2 || values[1].ID != "appliance:visible" || values[1].State != "unknown" || values[1].CheckedAt != nil || values[1].Permissions.Control {
			t.Fatalf("incorrect configured inventory: %+v", values)
		}
	}
}
