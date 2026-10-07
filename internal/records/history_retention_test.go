//go:build testing

package records_test

import (
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRawHistoryRetention(t *testing.T) {
	t.Setenv("BESZEL_HISTORY_RETENTION_DAYS", "35")
	app, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()
	user, err := tests.CreateUser(app, "history@example.com", "password123")
	require.NoError(t, err)
	system, err := tests.CreateRecord(app, "systems", map[string]any{"name": "History", "host": "localhost", "port": "45876", "status": "paused", "users": []string{user.Id}})
	require.NoError(t, err)
	now := time.Now().UTC()
	for _, name := range []string{"system_stats", "container_stats"} {
		for _, item := range []struct {
			kind string
			age  time.Duration
		}{{"raw", 36 * 24 * time.Hour}, {"raw", 34 * 24 * time.Hour}, {"480m", 29 * 24 * time.Hour}, {"1m", 2 * time.Hour}} {
			record, err := tests.CreateRecord(app, name, map[string]any{"system": system.Id, "type": item.kind, "stats": `{"cpu":1}`, "intervalSeconds": 15})
			require.NoError(t, err)
			record.SetRaw("created", now.Add(-item.age).Format("2006-01-02 15:04:05.000Z"))
			require.NoError(t, app.SaveNoValidate(record))
		}
	}
	// Exceed one cleanup batch to verify independent bounded deletions continue.
	_, err = app.DB().NewQuery(`WITH RECURSIVE samples(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM samples WHERE n<1005)
 INSERT INTO system_stats (id,system,type,stats,created,updated,intervalSeconds)
 SELECT printf('batch%010d',n),{:system},'raw','{"cpu":1}',{:created},{:created},15 FROM samples`).Bind(dbx.Params{"system": system.Id, "created": now.Add(-36 * 24 * time.Hour).Format("2006-01-02 15:04:05.000Z")}).Execute()
	require.NoError(t, err)
	records.NewRecordManager(app).DeleteOldRecords()
	for _, name := range []string{"system_stats", "container_stats"} {
		total, err := app.CountRecords(name, dbx.HashExp{"type": "raw"})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		total, err = app.CountRecords(name, dbx.HashExp{"type": "480m"})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		total, err = app.CountRecords(name, dbx.HashExp{"type": "1m"})
		require.NoError(t, err)
		require.Zero(t, total)
		var plan []struct{ Detail string }
		require.NoError(t, app.DB().NewQuery("EXPLAIN QUERY PLAN SELECT id FROM "+name+" WHERE type='raw' AND created < {:cutoff} ORDER BY created LIMIT 1000").Bind(dbx.Params{"cutoff": now.Format("2006-01-02 15:04:05.000Z")}).All(&plan))
		require.NotEmpty(t, plan)
		require.Contains(t, plan[0].Detail, "history_cleanup")
	}
}
