package pb

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandlerRenameDevice permits a display-name change without general device editing.
// Read access is enforced by the route; operators also need power or update access.
func HandlerRenameDevice(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if !e.HasSuperuserAuth() {
		permission, err := e.App.FindFirstRecordByFilter("permissions",
			"user.id = {:userId} && (power.id ?= {:deviceId} || update.id ?= {:deviceId})",
			dbx.Params{"userId": e.Auth.Id, "deviceId": id})
		if err != nil || permission == nil {
			return apis.NewForbiddenError("Device name editing is not permitted.", nil)
		}
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := e.BindBody(&body); err != nil {
		return apis.NewBadRequestError("Provide a device name.", nil)
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return apis.NewBadRequestError("Use 1–100 characters without control characters.", nil)
	}
	device, err := e.App.FindRecordById("devices", id)
	if err != nil {
		return apis.NewNotFoundError("Device not found.", nil)
	}
	device.Set("name", name)
	if err := e.App.Save(device); err != nil {
		return apis.NewBadRequestError("Could not save device name.", nil)
	}
	return e.JSON(http.StatusOK, device)
}
