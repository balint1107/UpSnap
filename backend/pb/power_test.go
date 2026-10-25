package pb

import (
	"encoding/json"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPowerSnapshotBoundsAndFreshness checks fail-closed parsing and historical retention after collection stops.
func TestPowerSnapshotBoundsAndFreshness(t *testing.T) {
	now := time.Now().UTC()
	filename := filepath.Join(t.TempDir(), "power.json")
	for _, age := range []time.Duration{30 * time.Second, 2 * time.Minute, -time.Minute} {
		value := PowerSnapshot{CheckedAt: now.Add(-age), Devices: []PowerDevice{{ID: "plug", Status: "up", CheckedAt: now.Add(-age), History: []PowerSample{{T: now.Unix(), W: 123}}}}}
		data, _ := json.Marshal(value)
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		result, err := readPowerSnapshot(filename, now)
		if err != nil {
			t.Fatal(err)
		}
		stale := age > 90*time.Second || age < -10*time.Second
		if (result.Devices[0].Status == "stale") != stale || len(result.Devices[0].History) != 1 {
			t.Fatalf("age %v: %+v", age, result)
		}
	}
	value := PowerSnapshot{CheckedAt: now, Devices: []PowerDevice{{History: make([]PowerSample, 1442)}}}
	data, _ := json.Marshal(value)
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPowerSnapshot(filename, now); err == nil {
		t.Fatal("oversized history accepted")
	}
	if err := os.WriteFile(filename, []byte(`{"checked_at":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPowerSnapshot(filename, now); err == nil {
		t.Fatal("invalid timestamp accepted")
	}
}

// TestPowerHostReadPermission checks an authenticated user cannot see a plug until granted its host's read permission.
func TestPowerHostReadPermission(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	defer app.ResetBootstrapState()
	hosts := core.NewBaseCollection("devices")
	hosts.Fields.Add(&core.TextField{Name: "ip"})
	if err := app.Save(hosts); err != nil {
		t.Fatal(err)
	}
	host := core.NewRecord(hosts)
	host.Set("ip", "192.168.50.152")
	if err := app.Save(host); err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("power-test@example.invalid")
	user.SetPassword("test-password-123")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	permissions := core.NewBaseCollection("permissions")
	permissions.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1}, &core.RelationField{Name: "read", CollectionId: hosts.Id, MaxSelect: 10})
	if err := app.Save(permissions); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	now := time.Now().UTC()
	data, _ := json.Marshal(PowerSnapshot{CheckedAt: now, Devices: []PowerDevice{{ID: "plug", HostIP: "192.168.50.152", Status: "up", CheckedAt: now}}})
	if err := os.WriteFile(filepath.Join(folder, "power.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPSNAP_SERVICES_FILE", filepath.Join(folder, "status.json"))
	for _, allowed := range []bool{false, true} {
		if allowed {
			p := core.NewRecord(permissions)
			p.Set("user", user.Id)
			p.Set("read", []string{host.Id})
			if err := app.Save(p); err != nil {
				t.Fatal(err)
			}
		}
		response := httptest.NewRecorder()
		event := &core.RequestEvent{App: app, Auth: user, Event: router.Event{Request: httptest.NewRequest("GET", "/api/upsnap/power", nil), Response: response}}
		if err := HandlerPower(event); err != nil {
			t.Fatal(err)
		}
		var result PowerSnapshot
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if (len(result.Devices) == 1) != allowed {
			t.Fatalf("read=%v returned %d devices", allowed, len(result.Devices))
		}
	}
}
