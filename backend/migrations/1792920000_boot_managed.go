package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init registers optional dual-boot UI metadata; no arguments or return value, firmware and power stay untouched.
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.BoolField{Name: "boot_managed"})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("boot_managed")
		return app.Save(collection)
	})
}
