package pb

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

func RequireWakeSuspendAllowed() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{Func: func(e *core.RequestEvent) error {
		device, err := e.App.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return apis.NewNotFoundError("Device not found.", nil)
		}
		if device.GetBool("reboot_only") {
			return apis.NewForbiddenError("This host allows reboot only; wake and suspend are disabled.", nil)
		}
		return e.Next()
	}}
}
