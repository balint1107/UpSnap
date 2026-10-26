package pb

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/seriousm4x/upsnap/logging"
	"github.com/seriousm4x/upsnap/networking"
)

func RequireDeviceReadPermission() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{Func: func(e *core.RequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewUnauthorizedError("Login required.", nil)
		}
		permission, err := e.App.FindFirstRecordByFilter("permissions",
			"user.id = {:userId} && read.id ?= {:deviceId}", dbx.Params{
				"userId": e.Auth.Id, "deviceId": e.Request.PathValue("id"),
			})
		if err != nil || permission == nil {
			return apis.NewForbiddenError("Device access denied.", nil)
		}
		return e.Next()
	}}
}

// HandlerHostStats serves authorized device readings for e, or explicit unavailability on Windows.
// Returns JSON/API errors; never runs the Linux collector on a managed non-Ubuntu host.
func HandlerHostStats(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	device, err := e.App.FindRecordById("devices", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("Device not found.", err)
	}
	if device.GetString("status") != "online" {
		return e.JSON(http.StatusOK, map[string]bool{"available": false})
	}
	host, configErr := networking.BootConfiguration(device.GetString("ip"))
	if configErr != nil {
		return apis.NewApiError(http.StatusServiceUnavailable, "Boot profiles unavailable.", nil)
	}
	if host != nil {
		name, probeErr := networking.DetectBootOS(e.Request.Context(), device.GetString("ip"))
		if probeErr != nil || name != "ubuntu" {
			return e.JSON(http.StatusOK, map[string]any{"available": false, "reason": "Resources are not yet supported on this OS."})
		}
	}
	stats, err := networking.ReadHostStats(e.Request.Context(), device)
	if err != nil {
		logging.Logger(e.App).Warn("Resource collection failed", "device", device.GetString("name"), "error", err)
		return apis.NewApiError(http.StatusServiceUnavailable, "Resource readings are unavailable.", nil)
	}
	return e.JSON(http.StatusOK, stats)
}
