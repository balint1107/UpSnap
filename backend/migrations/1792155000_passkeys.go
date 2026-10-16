package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("passkeys")
		c.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, Required: true, MinSelect: 1, MaxSelect: 1}, &core.TextField{Name: "rp_id", Required: true, Max: 253}, &core.TextField{Name: "credential_id", Required: true, Max: 2048}, &core.JSONField{Name: "credential", Required: true, MaxSize: 32768}, &core.TextField{Name: "label", Required: true, Max: 80}, &core.DateField{Name: "last_used"}, &core.AutodateField{Name: "created", OnCreate: true}, &core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		c.AddIndex("idx_passkeys_rp_credential", true, "rp_id, credential_id", "")
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("passkeys")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
