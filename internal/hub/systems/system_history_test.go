//go:build testing

package systems

import (
	"context"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestArchiveDoesNotUpdateInventoryOrNativeHistory(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	sys.ctx, sys.cancel = context.WithCancel(context.Background())
	defer sys.cancel()
	sys.swapStatus(up)
	data := &system.CombinedData{Stats: system.Stats{Cpu: 42}, Containers: []*container.Stats{{Name: "worker", Cpu: 123, Mem: 32}}}
	require.NoError(t, sys.archiveSample(data, 15))
	for _, name := range []string{"system_stats", "container_stats"} {
		total, err := app.CountRecords(name, dbx.HashExp{"type": "raw"})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		total, err = app.CountRecords(name, dbx.HashExp{"type": "1m"})
		require.NoError(t, err)
		require.Zero(t, total)
	}
	total, err := app.CountRecords("containers")
	require.NoError(t, err)
	require.Zero(t, total)
	sys.swapStatus(paused)
	require.NoError(t, sys.archiveSample(data, 15))
	total, err = app.CountRecords("system_stats", dbx.HashExp{"type": "raw"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	sys.swapStatus(up)
	sys.cancel()
	require.NoError(t, sys.archiveSample(data, 15))
	total, err = app.CountRecords("system_stats", dbx.HashExp{"type": "raw"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
}
