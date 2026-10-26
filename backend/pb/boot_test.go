package pb

import (
	"errors"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bootRequestFixture creates an isolated app/device and HTTP event for t; returns the borrowed test app and event.
// The host map locks firmware writes and PATH contains no executables, so tests cannot invoke real SSH or power commands.
func bootRequestFixture(t *testing.T, action, body string) (core.App, *core.RequestEvent) {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	collection := core.NewBaseCollection("devices")
	collection.Fields.Add(&core.TextField{Name: "ip"})
	if err := app.Save(collection); err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(collection)
	d.Set("ip", "192.168.50.148")
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "hosts.json")
	text := `{"192.168.50.148":{"writes_enabled":false,"profiles":{"ubuntu":{"alias":"test-ubuntu","username":"linux"},"windows":{"alias":"test-windows","username":"win"}}}}`
	if err := os.WriteFile(cfg, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPSNAP_BOOT_HOSTS_FILE", cfg)
	t.Setenv("PATH", t.TempDir())
	request := httptest.NewRequest("POST", "/api/upsnap/boot/"+d.Id+"/"+action, strings.NewReader(body))
	request.SetPathValue("id", d.Id)
	request.SetPathValue("action", action)
	return app, &core.RequestEvent{App: app, Event: router.Event{Request: request, Response: httptest.NewRecorder()}}
}

// requireBootError asserts err is an API error with HTTP status; t owns failure reporting and nothing is returned.
func requireBootError(t *testing.T, err error, status int) {
	t.Helper()
	var api *router.ApiError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("wanted HTTP %d, got %v", status, err)
	}
}

// TestBootRestartAndPrepareAreLocked proves legacy and new restart paths plus prepare fail closed before SSH.
func TestBootRestartAndPrepareAreLocked(t *testing.T) {
	_, e := bootRequestFixture(t, "prepare", `{"target":"windows"}`)
	requireBootError(t, RequireBootRestartLock().Func(e), http.StatusLocked)
	requireBootError(t, HandlerBootTarget(e), http.StatusLocked)
	_, e = bootRequestFixture(t, "restart", `{"target":"windows"}`)
	requireBootError(t, HandlerBootTarget(e), http.StatusLocked)
}

// TestBootBodyBounds rejects trailing JSON and oversized input without any OS helper execution.
func TestBootBodyBounds(t *testing.T) {
	_, e := bootRequestFixture(t, "validate", `{"target":"windows"} {}`)
	requireBootError(t, HandlerBootTarget(e), http.StatusBadRequest)
	e.Request.Body = http.MaxBytesReader(e.Response, httptest.NewRequest("POST", "/", strings.NewReader(`{"target":"`+strings.Repeat("x", 300)+`"}`)).Body, 1000)
	requireBootError(t, HandlerBootTarget(e), http.StatusBadRequest)
}

// TestBootRoutesRequireAuthentication verifies the exact read/power middleware bindings reject anonymous requests.
func TestBootRoutesRequireAuthentication(t *testing.T) {
	_, e := bootRequestFixture(t, "validate", `{"target":"windows"}`)
	requireBootError(t, RequireDeviceReadPermission().Func(e), http.StatusUnauthorized)
	requireBootError(t, RequireUpSnapPermission().Func(e), http.StatusUnauthorized)
}

// TestBootPowerPermissionRejectsReadOnlyUser confirms signed-in users do not automatically gain boot control.
func TestBootPowerPermissionRejectsReadOnlyUser(t *testing.T) {
	_, e := bootRequestFixture(t, "validate", `{"target":"windows"}`)
	e.Auth = core.NewRecord(core.NewAuthCollection("users"))
	e.Auth.Id = "read-only-user"
	requireBootError(t, RequireUpSnapPermission().Func(e), http.StatusForbidden)
}
