package migrations

import (
	"fmt"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"system_stats", "container_stats"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			field := c.Fields.GetByName("type").(*core.SelectField)
			field.Values = append(field.Values, "raw")
			c.Fields.Add(&core.NumberField{Name: "intervalSeconds", Min: ptrHistory(0), Max: ptrHistory(60)})
			c.Indexes = append(c.Indexes, fmt.Sprintf("CREATE INDEX idx_%s_history_cleanup ON %s (type, created)", name, name))
			if err = app.Save(c); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"system_stats", "container_stats"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if _, err = app.DB().NewQuery("DELETE FROM " + name + " WHERE type='raw'").Execute(); err != nil {
				return err
			}
			f := c.Fields.GetByName("type").(*core.SelectField)
			f.Values = f.Values[:len(f.Values)-1]
			c.Fields.RemoveByName("intervalSeconds")
			c.RemoveIndex("idx_" + name + "_history_cleanup")
			if err = app.Save(c); err != nil {
				return err
			}
		}
		return nil
	})
}
func ptrHistory(v float64) *float64 { return &v }
