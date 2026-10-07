package history

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	IntervalSeconds int `json:"intervalSeconds"`
	RetentionDays   int `json:"retentionDays"`
}

// Load validates runtime configuration before collectors start.
func Load() (Config, error) {
	c := Config{15, 35}
	for _, item := range []struct {
		name     string
		target   *int
		min, max int
	}{
		{"BESZEL_HISTORY_INTERVAL_SECONDS", &c.IntervalSeconds, 5, 60},
		{"BESZEL_HISTORY_RETENTION_DAYS", &c.RetentionDays, 1, 120},
	} {
		if v, ok := os.LookupEnv(item.name); ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < item.min || n > item.max {
				return c, fmt.Errorf("%s must be an integer between %d and %d", item.name, item.min, item.max)
			}
			*item.target = n
		}
	}
	return c, nil
}
