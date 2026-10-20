// Package duration parses the durations carrier accepts in flags and config.
package duration

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// Hint describes the accepted format; it is part of every parse error.
const Hint = "use a number and a unit, like 30s, 10m, 24h, 7d, or 2w"

const (
	Day  = 24 * time.Hour
	Week = 7 * Day
)

// units lists longest names first so "ms" is matched before "m".
var units = []struct {
	name string
	size time.Duration
}{
	{"ns", time.Nanosecond},
	{"us", time.Microsecond},
	{"µs", time.Microsecond},
	{"ms", time.Millisecond},
	{"s", time.Second},
	{"m", time.Minute},
	{"h", time.Hour},
	{"d", Day},
	{"w", Week},
}

// Parse parses a duration such as "90s", "1.5h", "7d", "2w", or "1d12h".
// It accepts Go's units (ns, us, ms, s, m, h) plus d (24h) and w (7d), and
// "0" without a unit. Negative durations are rejected.
func Parse(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration; " + Hint)
	}
	if s == "0" {
		return 0, nil
	}
	if strings.HasPrefix(s, "-") {
		return 0, errors.New("must not be negative")
	}
	rest := strings.TrimPrefix(s, "+")
	if rest == "" {
		return 0, errors.New(Hint)
	}
	var total float64
	for rest != "" {
		n := 0
		for n < len(rest) && (rest[n] == '.' || (rest[n] >= '0' && rest[n] <= '9')) {
			n++
		}
		if n == 0 {
			return 0, errors.New(Hint)
		}
		value, err := strconv.ParseFloat(rest[:n], 64)
		if err != nil {
			return 0, errors.New(Hint)
		}
		rest = rest[n:]
		size := time.Duration(0)
		for _, u := range units {
			if strings.HasPrefix(rest, u.name) {
				size = u.size
				rest = rest[len(u.name):]
				break
			}
		}
		if size == 0 {
			return 0, errors.New(Hint)
		}
		total += value * float64(size)
	}
	if total > math.MaxInt64 {
		return 0, errors.New("duration is too long")
	}
	return time.Duration(total), nil
}
