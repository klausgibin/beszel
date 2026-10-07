package history

import "testing"

func TestConfig(t *testing.T) {
	t.Setenv("BESZEL_HISTORY_INTERVAL_SECONDS", "15")
	t.Setenv("BESZEL_HISTORY_RETENTION_DAYS", "35")
	c, err := Load()
	if err != nil || c.IntervalSeconds != 15 || c.RetentionDays != 35 {
		t.Fatalf("unexpected config: %+v %v", c, err)
	}
	for _, v := range []string{"0", "4", "61", "65535", "15s", ""} {
		t.Setenv("BESZEL_HISTORY_INTERVAL_SECONDS", v)
		if _, err := Load(); err == nil {
			t.Errorf("accepted invalid interval %q", v)
		}
	}
	t.Setenv("BESZEL_HISTORY_INTERVAL_SECONDS", "60")
	for _, v := range []string{"0", "121", "-1", "days"} {
		t.Setenv("BESZEL_HISTORY_RETENTION_DAYS", v)
		if _, err := Load(); err == nil {
			t.Errorf("accepted invalid retention %q", v)
		}
	}
}
