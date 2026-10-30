package pb

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// ssoSettings binds trusted Authelia usernames to existing record IDs; it never provisions users.
type ssoSettings struct {
	Secret   string            `json:"secret"`
	Users    map[string]string `json:"users"`
	AdminIDs []string          `json:"admin_ids"`
}

// readSSOSettings loads the optional private binding file; a missing or malformed file fails closed.
func readSSOSettings() (ssoSettings, error) {
	var settings ssoSettings
	data, err := os.ReadFile(os.Getenv("UPSNAP_SSO_FILE"))
	if err != nil {
		return settings, err
	}
	err = json.Unmarshal(data, &settings)
	return settings, err
}

// trustedSSOUser accepts only an exact constant-time shared secret and an explicitly bound username.
// Returns an existing record ID, or an empty string on any authentication failure.
func trustedSSOUser(settings ssoSettings, secret, username string) string {
	if len(settings.Secret) < 32 || subtle.ConstantTimeCompare([]byte(settings.Secret), []byte(secret)) != 1 {
		return ""
	}
	return settings.Users[username]
}

// HandlerSSO exchanges proxy-verified identity for the bound user's ordinary PocketBase token.
// It never accepts a client-supplied record ID, email match, password or auto-registration request.
func HandlerSSO(e *core.RequestEvent) error {
	settings, err := readSSOSettings()
	if err != nil {
		return e.JSON(404, map[string]bool{"enabled": false})
	}
	id := trustedSSOUser(settings, e.Request.Header.Get("X-Monitor-Secret"), e.Request.Header.Get("X-Monitor-User"))
	if id == "" {
		return e.ForbiddenError("No existing monitor account is linked to this login.", nil)
	}
	user, err := e.App.FindRecordById("users", id)
	if err != nil {
		return e.ForbiddenError("Linked monitor account is unavailable.", nil)
	}
	e.Response.Header().Set("Cache-Control", "no-store")
	return apis.RecordAuthResponse(e, user, "authelia", nil)
}

// serviceAdministrator reports an explicit service-management grant; device power grants confer no administration.
func serviceAdministrator(e *core.RequestEvent) bool {
	if e.HasSuperuserAuth() {
		return true
	}
	if e.Auth == nil || e.Auth.Collection().Name != "users" {
		return false
	}
	settings, err := readSSOSettings()
	if err != nil {
		return false
	}
	for _, id := range settings.AdminIDs {
		if id == e.Auth.Id {
			return true
		}
	}
	return false
}

// serviceRouteRequest relays a bounded registry request to the fixed local helper socket.
// method must be GET or PUT; body is caller-owned JSON, and the returned bytes contain no secrets.
func serviceRouteRequest(ctx context.Context, method string, body io.Reader) (int, []byte, error) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", os.Getenv("UPSNAP_SERVICE_ROUTES_SOCKET"))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second}
	request, err := http.NewRequestWithContext(ctx, method, "http://localhost/services", body)
	if err != nil {
		return 0, nil, err
	}
	if method == "PUT" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 32768))
	return response.StatusCode, data, err
}

// HandlerServiceRoutes authorizes registry reads/updates and forwards only bounded structured JSON to the helper.
// The helper validates route syntax, fixed upstreams, revision conflicts and rollback before applying changes.
func HandlerServiceRoutes(e *core.RequestEvent) error {
	if !serviceAdministrator(e) {
		return e.ForbiddenError("Service administration is not permitted.", nil)
	}
	e.Response.Header().Set("Cache-Control", "no-store")
	var body io.Reader
	if e.Request.Method == "PUT" {
		data, err := io.ReadAll(io.LimitReader(e.Request.Body, 2049))
		if err != nil || len(data) > 2048 || !json.Valid(data) {
			return e.BadRequestError("Invalid settings request.", nil)
		}
		body = strings.NewReader(string(data))
	}
	status, data, err := serviceRouteRequest(e.Request.Context(), e.Request.Method, body)
	if err != nil {
		return apis.NewApiError(503, "Route controller unavailable.", nil)
	}
	return e.Blob(status, "application/json", data)
}
