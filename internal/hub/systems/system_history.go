package systems

import (
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/history"
	"github.com/pocketbase/pocketbase/core"
	"time"
)

// archiveSample persists metrics only, leaving inventory, status and alerts on the native cadence.
func (sys *System) archiveSample(data *system.CombinedData, seconds int) error {
	if sys.ctx.Err() != nil || sys.GetStatus() != up {
		return nil
	}
	sys.recordsMu.Lock()
	defer sys.recordsMu.Unlock()
	return sys.manager.hub.RunInTransaction(func(app core.App) error {
		for _, item := range []struct {
			name  string
			value any
		}{{"system_stats", data.Stats}, {"container_stats", data.Containers}} {
			c, err := app.FindCachedCollectionByNameOrId(item.name)
			if err != nil {
				return err
			}
			r := core.NewRecord(c)
			r.Set("system", sys.Id)
			r.Set("type", "raw")
			r.Set("stats", item.value)
			r.Set("intervalSeconds", seconds)
			if err = app.SaveNoValidate(r); err != nil {
				return err
			}
		}
		return nil
	})
}

func (sys *System) collectArchive(seconds int) error {
	if sys.GetStatus() != up || sys.ctx.Err() != nil {
		sys.archiveWarmed = false
		return nil
	}
	data, err := sys.fetchDataFromAgent(common.DataRequestOptions{CacheTimeMs: uint16(seconds * 1000)})
	if err != nil {
		sys.archiveWarmed = false
		return err
	}
	if !sys.archiveWarmed {
		sys.archiveWarmed = true
		return nil
	}
	migrateDeprecatedFields(data, false)
	return sys.archiveSample(data, seconds)
}

func historyInterval() time.Duration {
	c, err := history.Load()
	if err != nil {
		return 15 * time.Second
	}
	return time.Duration(c.IntervalSeconds) * time.Second
}
