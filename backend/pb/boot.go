package pb

import (
	"encoding/json"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/seriousm4x/upsnap/networking"
	"io"
	"net/http"
)

// HandlerBootStatus reads capabilities for the authorized device in e; returns no-store JSON or an API error.
// The read-permission middleware supplies authorization. No firmware or power operation is invoked.
func HandlerBootStatus(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	d, err := e.App.FindRecordById("devices", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("Device not found.", nil)
	}
	status, err := networking.ReadBootStatus(e.Request.Context(), d.GetString("ip"))
	if err != nil {
		status = &networking.BootStatus{OS: "unknown", Message: "OS identity or boot information is unavailable.", Targets: map[string]networking.BootTarget{}}
	}
	job, jobErr := currentBootJob(e.App, d.GetString("ip"), status)
	if jobErr != nil {
		return apis.NewApiError(http.StatusServiceUnavailable, "Boot progress storage is unavailable.", nil)
	}
	return e.JSON(http.StatusOK, struct {
		*networking.BootStatus
		Job *BootJob `json:"job,omitempty"`
	}{status, job})
}

// HandlerBootTarget validates a bounded target body from e; action=prepare requires the independent write gates.
// Power-permission middleware is required. Restart additionally requires independent gates and a persisted host job.
func HandlerBootTarget(e *core.RequestEvent) error {
	action := e.Request.PathValue("action")
	if action != "validate" && action != "prepare" && action != "restart" {
		return apis.NewApiError(http.StatusLocked, "Restart execution is locked pending explicit authorization.", nil)
	}
	d, err := e.App.FindRecordById("devices", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("Device not found.", nil)
	}
	var body struct {
		Target string `json:"target"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(e.Response, e.Request.Body, 256))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&body); err != nil {
		return apis.NewBadRequestError("Expected one boot target.", nil)
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return apis.NewBadRequestError("Invalid trailing body.", nil)
	}
	if action == "prepare" || action == "restart" {
		host, cfgErr := networking.BootConfiguration(d.GetString("ip"))
		if cfgErr != nil || host == nil || !host.WritesEnabled || (action == "restart" && !host.RestartEnabled) {
			return apis.NewApiError(http.StatusLocked, "Boot selection is locked pending explicit authorization.", nil)
		}
	}
	if action == "restart" {
		if e.Auth == nil {
			return apis.NewUnauthorizedError("Login required.", nil)
		}
		job, err := startBootJob(e.Request.Context(), e.App, d.GetString("ip"), body.Target, e.Auth.Id)
		if err != nil {
			return apis.NewApiError(http.StatusConflict, err.Error(), nil)
		}
		return e.JSON(http.StatusAccepted, job)
	}
	status, err := networking.CheckBootTarget(e.Request.Context(), d.GetString("ip"), body.Target, action == "prepare")
	if err != nil {
		return apis.NewApiError(http.StatusConflict, err.Error(), nil)
	}
	return e.JSON(http.StatusOK, status)
}

// RequireBootRestartLock returns middleware that blocks all existing reboot routes for configured dual-boot hosts.
// No arguments; malformed configuration fails closed. The returned handler never issues a power operation.
func RequireBootRestartLock() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{Func: func(e *core.RequestEvent) error {
		d, err := e.App.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return apis.NewNotFoundError("Device not found.", nil)
		}
		host, err := networking.BootConfiguration(d.GetString("ip"))
		if err != nil || host != nil {
			return apis.NewApiError(http.StatusLocked, "Dual-boot restart is locked pending explicit authorization.", nil)
		}
		return e.Next()
	}}
}
