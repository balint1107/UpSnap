package pb

import (
	"encoding/json"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// writeRouterFixture sets a temporary trusted definition file for values; t owns its cleanup.
func writeRouterFixture(t *testing.T, values []RouterDefinition) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "routers.json")
	b, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPSNAP_ROUTERS_FILE", p)
}

// TestRouterActionsRejectInjectionAndUnsupportedHardware validates radio and LED capabilities before SSH.
func TestRouterActionsRejectInjectionAndUnsupportedHardware(t *testing.T) {
	r := RouterDefinition{Platform: "asus", Bands: []RouterBand{{ID: "0"}}}
	for _, v := range [][3]string{{"wifi", "off;reboot", "0"}, {"wifi", "off", "0;reboot"}, {"leds", "on", "0"}, {"reboot", "on", ""}} {
		if validateRouterAction(r, v[0], v[1], v[2]) == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
	if validateRouterAction(r, "wifi", "off", "0") != nil {
		t.Fatal("valid band rejected")
	}
	micro := RouterDefinition{Platform: "routeros", LCD: true}
	if validateRouterAction(micro, "leds", "off", "") == nil || validateRouterAction(micro, "wifi", "off", "0") == nil {
		t.Fatal("unsupported hardware accepted")
	}
	if validateRouterAction(micro, "lcd", "off", "") != nil {
		t.Fatal("LCD rejected")
	}
}

// TestRouterDefinitionsFailClosed rejects malformed destination IDs and unapproved host-key algorithms.
func TestRouterDefinitionsFailClosed(t *testing.T) {
	for _, r := range []RouterDefinition{{ID: "-oProxyCommand=id", IP: "192.168.50.1", Port: 22, Algorithms: []string{"ssh-rsa"}}, {ID: "router", IP: "example.com", Port: 22, Algorithms: []string{"ssh-rsa"}}, {ID: "router", IP: "192.168.50.1", Port: 22, Algorithms: []string{"unknown"}}} {
		writeRouterFixture(t, []RouterDefinition{r})
		if _, err := routerDefinitions(); err == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
}

// TestRouterTerminalKeepsPasswordOnlyPinnedHostPolicy exercises custom ports and the existing SSH protocol fixture.
func TestRouterTerminalKeepsPasswordOnlyPinnedHostPolicy(t *testing.T) {
	target, hosts, calls := sshPasswordFixture(t)
	ip, port, _ := net.SplitHostPort(target)
	n, _ := strconv.Atoi(port)
	writeRouterFixture(t, []RouterDefinition{{ID: "router-test", IP: ip, Port: n, Algorithms: []string{"ssh-ed25519"}}})
	c, err := passwordSSH(target, "tester", "test-password", hosts)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if calls.Load() != 1 {
		t.Fatal("password-only path not used")
	}
	writeRouterFixture(t, []RouterDefinition{{ID: "router-test", IP: ip, Port: n, Algorithms: []string{"ssh-rsa"}}})
	if c, err := passwordSSH(target, "tester", "test-password", hosts); err == nil {
		c.Close()
		t.Fatal("unpinned RSA key accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("password transmitted before host key verification")
	}
	if _, target, err := routerTerminalRecord(nil, "unknown-router"); err != nil || target != "" {
		t.Fatal("unknown router resolved")
	}
}

// TestRouterReadAndPowerPermissions verifies status grants cannot be used to mutate router state.
func TestRouterReadAndPowerPermissions(t *testing.T) {
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
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail("router-test@example.invalid")
	u.SetPassword("test-password-123")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	grants := core.NewBaseCollection("permissions")
	grants.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1}, &core.RelationField{Name: "read", CollectionId: hosts.Id, MaxSelect: 10}, &core.RelationField{Name: "power", CollectionId: hosts.Id, MaxSelect: 10})
	if err := app.Save(grants); err != nil {
		t.Fatal(err)
	}
	e := &core.RequestEvent{App: app, Auth: u, Event: router.Event{Request: httptest.NewRequest("POST", "/", nil), Response: httptest.NewRecorder()}}
	if routerPermission(e, false) || routerPermission(e, true) {
		t.Fatal("ungranted user accepted")
	}
	p := core.NewRecord(grants)
	p.Set("user", u.Id)
	p.Set("read", []string{host.Id})
	if err := app.Save(p); err != nil {
		t.Fatal(err)
	}
	if !routerPermission(e, false) || routerPermission(e, true) {
		t.Fatal("read/power separation failed")
	}
	if err := HandlerRouterAction(e); err == nil {
		t.Fatal("read-only action accepted")
	}
	p.Set("power", []string{host.Id})
	if err := app.Save(p); err != nil {
		t.Fatal(err)
	}
	if !routerPermission(e, true) {
		t.Fatal("power grant rejected")
	}
}
