package hub

import (
	"math"
	"testing"
)

func TestHistorySummary(t *testing.T) {
	s := summarizeHistory([]historyValue{{0, 15}, {20, 15}, {80, 15}, {100, 15}}, 120, 80)
	if s.Count != 4 || *s.Mean != 50 || *s.P50 != 50 || s.Coverage != 50 || s.AboveThresholdPercent != 25 {
		t.Fatalf("wrong summary: %+v", s)
	}
	if math.Abs(*s.P95-97) > 1e-9 || math.Abs(*s.P99-99.4) > 1e-9 {
		t.Fatalf("incorrect percentiles: %v %v", *s.P95, *s.P99)
	}
	n := 0
	for _, bin := range s.Histogram {
		n += bin.Count
	}
	if n != 4 {
		t.Fatalf("histogram lost samples: %d", n)
	}
	empty := summarizeHistory(nil, 120, 80)
	if empty.Mean != nil || empty.Min != nil || empty.Coverage != 0 {
		t.Fatal("missing values must stay missing")
	}
}
func TestHistorySummaryContainerCPU(t *testing.T) {
	s := summarizeHistory([]historyValue{{250, 10}, {500, 5}}, 60, 300)
	if *s.Max != 500 || math.Abs(s.AboveThresholdPercent-100.0/3) > 1e-9 {
		t.Fatalf("unexpected high CPU summary: %+v", s)
	}
}
