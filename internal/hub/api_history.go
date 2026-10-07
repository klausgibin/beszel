package hub

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/henrygd/beszel/internal/history"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"
	_ "time/tzdata"
)

type historyPoint struct {
	Time          string   `json:"time"`
	Count         int      `json:"count"`
	Min           *float64 `json:"min"`
	Max           *float64 `json:"max"`
	Mean          *float64 `json:"mean"`
	Coverage      float64  `json:"coverage"`
	sum, duration float64
}
type historyHistogram struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Count int     `json:"count"`
}
type historySummary struct {
	Count                 int                `json:"count"`
	Min                   *float64           `json:"min"`
	Max                   *float64           `json:"max"`
	Mean                  *float64           `json:"mean"`
	P50                   *float64           `json:"p50"`
	P95                   *float64           `json:"p95"`
	P99                   *float64           `json:"p99"`
	Coverage              float64            `json:"coverage"`
	AboveThresholdPercent float64            `json:"aboveThresholdPercent"`
	Histogram             []historyHistogram `json:"histogram"`
}
type historyDaily struct {
	Date string `json:"date"`
	historySummary
}
type historyValue struct{ value, duration float64 }

func summarizeHistory(values []historyValue, duration, threshold float64) historySummary {
	result := historySummary{Histogram: []historyHistogram{}}
	if len(values) == 0 {
		return result
	}
	sorted := make([]float64, len(values))
	sum, covered, above := 0.0, 0.0, 0.0
	for i, v := range values {
		sorted[i] = v.value
		sum += v.value
		covered += v.duration
		if v.value > threshold {
			above += v.duration
		}
	}
	sort.Float64s(sorted)
	n := len(sorted)
	result.Count = n
	ptr := func(v float64) *float64 { return &v }
	result.Min = ptr(sorted[0])
	result.Max = ptr(sorted[n-1])
	result.Mean = ptr(sum / float64(n))
	percentile := func(p float64) *float64 {
		index := p * float64(n-1)
		a := int(index)
		b := min(a+1, n-1)
		return ptr(sorted[a] + (sorted[b]-sorted[a])*(index-float64(a)))
	}
	result.P50 = percentile(.5)
	result.P95 = percentile(.95)
	result.P99 = percentile(.99)
	if duration > 0 {
		result.Coverage = math.Min(100, covered/duration*100)
	}
	if covered > 0 {
		result.AboveThresholdPercent = above / covered * 100
	}
	upper := math.Max(100, math.Ceil(sorted[n-1]/10)*10)
	width := upper / 10
	for i := range 10 {
		result.Histogram = append(result.Histogram, historyHistogram{Min: float64(i) * width, Max: float64(i+1) * width})
	}
	for _, v := range sorted {
		bin := min(9, max(0, int(v/width)))
		result.Histogram[bin].Count++
	}
	return result
}

func (h *Hub) historyConfig(e *core.RequestEvent) error {
	c, err := history.Load()
	if err != nil {
		return err
	}
	return e.JSON(http.StatusOK, c)
}
func (h *Hub) getHistory(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	id := q.Get("system")
	sys := &systems.System{Id: id}
	if !sys.HasUser(e.App, e.Auth) {
		return e.ForbiddenError("System access denied", nil)
	}
	c, err := history.Load()
	if err != nil {
		return err
	}
	start, err := time.Parse(time.RFC3339, q.Get("start"))
	if err != nil {
		return e.BadRequestError("Invalid start timestamp", err)
	}
	end, err := time.Parse(time.RFC3339, q.Get("end"))
	if err != nil {
		return e.BadRequestError("Invalid end timestamp", err)
	}
	now := time.Now()
	if !start.Before(end) || end.Sub(start) > time.Duration(c.RetentionDays)*24*time.Hour || start.Before(now.Add(-time.Duration(c.RetentionDays)*24*time.Hour-time.Minute)) || end.After(now.Add(time.Minute)) {
		return e.BadRequestError("Range must be within retained history", nil)
	}
	points := 1000
	if raw := q.Get("points"); raw != "" {
		points, err = strconv.Atoi(raw)
		if err != nil || points < 1 || points > 2000 {
			return e.BadRequestError("points must be between 1 and 2000", nil)
		}
	}
	threshold := 80.0
	if raw := q.Get("threshold"); raw != "" {
		threshold, err = strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
			return e.BadRequestError("Invalid threshold", nil)
		}
	}
	tz := q.Get("timezone")
	if tz == "" {
		tz = "UTC"
	}
	location, err := time.LoadLocation(tz)
	if err != nil {
		return e.BadRequestError("Invalid timezone", err)
	}
	metric := q.Get("metric")
	if metric == "" {
		metric = "cpu"
	}
	if metric != "cpu" && metric != "memory" && metric != "disk" {
		return e.BadRequestError("Invalid metric", nil)
	}
	container := q.Get("container")
	if container != "" && metric == "disk" {
		return e.BadRequestError("Container disk usage is unavailable", nil)
	}
	table := "system_stats"
	if container != "" {
		table = "container_stats"
	}
	// Context cancellation terminates a disconnected client's database scan.
	rows, err := e.App.DB().NewQuery("SELECT created, stats, intervalSeconds FROM " + table + " WHERE system={:system} AND type='raw' AND created>={:start} AND created<{:end} ORDER BY created").Bind(dbx.Params{"system": id, "start": start.UTC().Format("2006-01-02 15:04:05.000Z"), "end": end.UTC().Format("2006-01-02 15:04:05.000Z")}).WithContext(e.Request.Context()).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	export := e.Request.URL.Path == "/api/beszel/history/export"
	var writer *csv.Writer
	if export {
		e.Response.Header().Set("Content-Type", "text/csv")
		e.Response.Header().Set("Content-Disposition", "attachment; filename=beszel-history.csv")
		writer = csv.NewWriter(e.Response)
		if err = writer.Write([]string{"timestamp", "value", "interval_seconds"}); err != nil {
			return err
		}
	}
	bucketSeconds := math.Max(float64(c.IntervalSeconds), math.Ceil(end.Sub(start).Seconds()/float64(points)))
	bucketCount := int(math.Ceil(end.Sub(start).Seconds() / bucketSeconds))
	buckets := make([]historyPoint, bucketCount)
	for i := range buckets {
		buckets[i].Time = start.Add(time.Duration(float64(i)*bucketSeconds) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	values := make([]historyValue, 0)
	dailySummaries := map[string]historySummary{}
	dailyDurations := map[string][2]float64{}
	var currentDate string
	var dayValues []historyValue
	finishDay := func() {
		if currentDate == "" {
			return
		}
		dayStart, _ := time.ParseInLocation("2006-01-02", currentDate, location)
		dayEnd := dayStart.AddDate(0, 0, 1)
		if dayStart.Before(start) {
			dayStart = start
		}
		if dayEnd.After(end) {
			dayEnd = end
		}
		dailySummaries[currentDate] = summarizeHistory(dayValues, dayEnd.Sub(dayStart).Seconds(), threshold)
	}
	containerNames := map[string]bool{}
	var previous time.Time
	for rows.Next() {
		var created, payload string
		var interval float64
		if err = rows.Scan(&created, &payload, &interval); err != nil {
			return err
		}
		stamp, err := time.Parse("2006-01-02 15:04:05.000Z", created)
		if err != nil {
			return fmt.Errorf("invalid stored history timestamp: %w", err)
		}
		if stamp.Before(start) {
			continue
		}
		if interval <= 0 {
			interval = float64(c.IntervalSeconds)
		}
		duration := interval
		if !previous.IsZero() {
			duration = math.Min(interval, stamp.Sub(previous).Seconds())
		}
		duration = math.Max(0, math.Min(duration, stamp.Sub(start).Seconds()))
		previous = stamp
		var value float64
		found := true
		if container != "" {
			var entries []struct {
				Name   string  `json:"n"`
				CPU    float64 `json:"c"`
				Memory float64 `json:"m"`
			}
			if err = json.Unmarshal([]byte(payload), &entries); err != nil {
				return err
			}
			found = false
			for _, entry := range entries {
				containerNames[entry.Name] = true
				if entry.Name == container {
					found = true
					value = entry.CPU
					if metric == "memory" {
						value = entry.Memory
					}
				}
			}
		} else {
			var stats struct {
				CPU    float64 `json:"cpu"`
				Memory float64 `json:"mp"`
				Disk   float64 `json:"dp"`
			}
			if err = json.Unmarshal([]byte(payload), &stats); err != nil {
				return err
			}
			value = stats.CPU
			if metric == "memory" {
				value = stats.Memory
			} else if metric == "disk" {
				value = stats.Disk
			}
		}
		if !found {
			continue
		}
		if export {
			if err = writer.Write([]string{stamp.UTC().Format(time.RFC3339Nano), strconv.FormatFloat(value, 'f', -1, 64), strconv.FormatFloat(interval, 'f', -1, 64)}); err != nil {
				return err
			}
			continue
		}
		segment := stamp.Add(-time.Duration(duration * float64(time.Second)))
		for segment.Before(stamp) {
			local := segment.In(location)
			dayBoundary := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location)
			segmentEnd := stamp
			if dayBoundary.Before(segmentEnd) {
				segmentEnd = dayBoundary
			}
			seconds := segmentEnd.Sub(segment).Seconds()
			if seconds <= 0 {
				break
			}
			key := local.Format("2006-01-02")
			totals := dailyDurations[key]
			totals[0] += seconds
			if value > threshold {
				totals[1] += seconds
			}
			dailyDurations[key] = totals
			segment = segmentEnd
		}
		sample := historyValue{value, duration}
		values = append(values, sample)
		date := stamp.In(location).Format("2006-01-02")
		if date != currentDate {
			finishDay()
			currentDate = date
			dayValues = dayValues[:0]
		}
		dayValues = append(dayValues, sample)
		index := min(bucketCount-1, int(stamp.Sub(start).Seconds()/bucketSeconds))
		b := &buckets[index]
		b.Count++
		b.sum += value
		segmentStart := stamp.Add(-time.Duration(duration * float64(time.Second)))
		for segmentStart.Before(stamp) {
			bin := min(bucketCount-1, max(0, int(segmentStart.Sub(start).Seconds()/bucketSeconds)))
			boundary := start.Add(time.Duration(float64(bin+1) * bucketSeconds * float64(time.Second)))
			segmentEnd := stamp
			if boundary.Before(segmentEnd) {
				segmentEnd = boundary
			}
			if !segmentEnd.After(segmentStart) {
				break
			}
			buckets[bin].duration += segmentEnd.Sub(segmentStart).Seconds()
			segmentStart = segmentEnd
		}
		if b.Min == nil || value < *b.Min {
			v := value
			b.Min = &v
		}
		if b.Max == nil || value > *b.Max {
			v := value
			b.Max = &v
		}
	}
	finishDay()
	if err = rows.Err(); err != nil {
		return err
	}
	if export {
		writer.Flush()
		return writer.Error()
	}
	for i := range buckets {
		b := &buckets[i]
		if b.Count > 0 {
			mean := b.sum / float64(b.Count)
			b.Mean = &mean
		}
		b.Coverage = math.Min(100, b.duration/math.Min(bucketSeconds, end.Sub(start.Add(time.Duration(float64(i)*bucketSeconds)*time.Second)).Seconds())*100)
	}
	rows.Close()
	// Discover historical containers even when the live catalog no longer contains them.
	if container == "" {
		nameRows, err := e.App.DB().NewQuery("SELECT DISTINCT json_extract(value,'$.n') AS name FROM container_stats, json_each(container_stats.stats) WHERE container_stats.system={:system} AND container_stats.type='raw' AND container_stats.created>={:start} AND container_stats.created<{:end}").Bind(dbx.Params{"system": id, "start": start.UTC().Format("2006-01-02 15:04:05.000Z"), "end": end.UTC().Format("2006-01-02 15:04:05.000Z")}).WithContext(e.Request.Context()).Rows()
		if err != nil {
			return err
		}
		defer nameRows.Close()
		for nameRows.Next() {
			var name string
			if err = nameRows.Scan(&name); err != nil {
				return err
			}
			containerNames[name] = true
		}
		if err = nameRows.Err(); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(containerNames))
	for name := range containerNames {
		names = append(names, name)
	}
	sort.Strings(names)
	days := []historyDaily{}
	localStart := start.In(location)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	for day.Before(end) {
		next := day.AddDate(0, 0, 1)
		rangeStart := day
		if rangeStart.Before(start) {
			rangeStart = start
		}
		rangeEnd := next
		if rangeEnd.After(end) {
			rangeEnd = end
		}
		date := day.Format("2006-01-02")
		summary, ok := dailySummaries[date]
		if !ok {
			summary = summarizeHistory(nil, rangeEnd.Sub(rangeStart).Seconds(), threshold)
		}
		totals := dailyDurations[date]
		summary.Coverage = math.Min(100, totals[0]/rangeEnd.Sub(rangeStart).Seconds()*100)
		if totals[0] > 0 {
			summary.AboveThresholdPercent = totals[1] / totals[0] * 100
		} else {
			summary.AboveThresholdPercent = 0
		}
		days = append(days, historyDaily{date, summary})
		day = next
	}
	return e.JSON(http.StatusOK, map[string]any{"intervalSeconds": c.IntervalSeconds, "retentionDays": c.RetentionDays, "bucketSeconds": bucketSeconds, "containers": names, "points": buckets, "summary": summarizeHistory(values, end.Sub(start).Seconds(), threshold), "daily": days})
}
