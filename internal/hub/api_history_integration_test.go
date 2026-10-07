//go:build testing

package hub_test

import (
	"encoding/json"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGranularHistoryAPI(t *testing.T) {
	app, handler := firstUserTestMux(t)
	defer app.Cleanup()
	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "history@example.com")
	user.SetPassword("historypassword")
	user.Set("role", "admin")
	require.NoError(t, app.Save(user))
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(collection)
	system.Set("name", "History fixture")
	system.Set("host", "localhost")
	system.Set("port", "45876")
	system.Set("users", []string{user.Id})
	system.Set("status", "paused")
	require.NoError(t, app.SaveNoValidate(system))
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)
	end := start.Add(time.Minute)
	stats, err := app.FindCollectionByNameOrId("system_stats")
	require.NoError(t, err)
	for i, v := range []float64{10, 90} {
		r := core.NewRecord(stats)
		r.Set("system", system.Id)
		r.Set("type", "raw")
		r.Set("intervalSeconds", 15)
		r.Set("created", start.Add(time.Duration(i+1)*15*time.Second))
		r.Set("stats", map[string]any{"cpu": v, "mp": 50, "dp": 30})
		require.NoError(t, app.Save(r))
		r.SetRaw("created", start.Add(time.Duration(i+1)*15*time.Second).Format("2006-01-02 15:04:05.000Z"))
		require.NoError(t, app.SaveNoValidate(r))
	}
	containers, err := app.FindCollectionByNameOrId("container_stats")
	require.NoError(t, err)
	r := core.NewRecord(containers)
	r.Set("system", system.Id)
	r.Set("type", "raw")
	r.Set("intervalSeconds", 15)
	r.Set("created", start.Add(15*time.Second))
	r.Set("stats", []map[string]any{{"n": "gone-container", "c": 250, "m": 32}})
	require.NoError(t, app.Save(r))
	r.SetRaw("created", start.Add(15*time.Second).Format("2006-01-02 15:04:05.000Z"))
	require.NoError(t, app.SaveNoValidate(r))
	params := url.Values{"system": {system.Id}, "start": {start.Format(time.RFC3339)}, "end": {end.Format(time.RFC3339)}, "points": {"4"}, "timezone": {"America/Sao_Paulo"}}
	get := func(path, auth string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	response := get("/api/beszel/history?"+params.Encode(), token)
	require.Equal(t, 200, response.Code, response.Body.String())
	var result struct {
		Points []struct {
			Time  string `json:"time"`
			Count int
			Mean  *float64
		}
		Summary struct {
			Count    int
			Mean     float64
			Coverage float64
		}
		Containers []string
		Daily      []struct{ Date string }
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, 2, result.Summary.Count)
	require.Equal(t, 50.0, result.Summary.Mean)
	require.Equal(t, 50.0, result.Summary.Coverage)
	require.Nil(t, result.Points[0].Mean)
	require.Contains(t, result.Containers, "gone-container")
	require.Len(t, result.Daily, 1)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &wire))
	first := wire["points"].([]any)[0].(map[string]any)
	require.NotEmpty(t, first["time"])
	require.Equal(t, 100.0, first["coverage"])
	other := core.NewRecord(users)
	other.Set("email", "other@example.com")
	other.SetPassword("historypassword")
	other.Set("role", "user")
	require.NoError(t, app.Save(other))
	otherToken, err := other.NewAuthToken()
	require.NoError(t, err)
	require.Equal(t, 403, get("/api/beszel/history?"+params.Encode(), otherToken).Code)
	other.Set("role", "readonly")
	require.NoError(t, app.Save(other))
	system.Set("users", []string{user.Id, other.Id})
	require.NoError(t, app.SaveNoValidate(system))
	otherToken, err = other.NewAuthToken()
	require.NoError(t, err)
	require.Equal(t, 200, get("/api/beszel/history?"+params.Encode(), otherToken).Code)
	csv := get("/api/beszel/history/export?"+params.Encode(), token)
	require.Equal(t, 200, csv.Code)
	require.Contains(t, csv.Body.String(), "timestamp,value,interval_seconds")
	require.Len(t, strings.Split(strings.TrimSpace(csv.Body.String()), "\n"), 3)
	params.Set("container", "gone-container")
	response = get("/api/beszel/history?"+params.Encode(), token)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"max":250`)
	params.Set("timezone", "invalid/zone")
	require.Equal(t, 400, get("/api/beszel/history?"+params.Encode(), token).Code)
	require.Equal(t, 401, get("/api/beszel/history?"+params.Encode(), "").Code)
	params.Set("system", "nonexistent")
	require.Equal(t, 403, get("/api/beszel/history?"+params.Encode(), token).Code)
}
