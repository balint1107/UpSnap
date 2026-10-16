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

func HandlerHostStats(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	device, err := e.App.FindRecordById("devices", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("Device not found.", err)
	}
	if device.GetString("status") != "online" {
		return e.JSON(http.StatusOK, map[string]bool{"available": false})
	}
	stats, err := networking.ReadHostStats(e.Request.Context(), device)
	if err != nil {
		logging.Logger(e.App).Warn("Resource collection failed", "device", device.GetString("name"), "error", err)
		return apis.NewApiError(http.StatusServiceUnavailable, "Resource readings are unavailable.", nil)
	}
	return e.JSON(http.StatusOK, stats)
}
