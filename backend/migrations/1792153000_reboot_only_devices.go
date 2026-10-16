package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.BoolField{Name: "reboot_only"})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("reboot_only")
		return app.Save(collection)
	})
}
