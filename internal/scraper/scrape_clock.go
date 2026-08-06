package scraper

import (
	"time"

	kitlog "github.com/go-kit/log"
)

// scrapeClock decides how a single scrape's telemetry is timestamped. The
// scheduled scraper and unscheduled collector select different policies at
// their entry points, while the scrape pipeline remains caller-agnostic.
type scrapeClock interface {
	// wrapperLogTimestamp is the kitlog Valuer for the wrapper-log `ts` field.
	// extractLogs turns that field into each Loki entry's timestamp, so it
	// governs where logs without their own `time` field land.
	wrapperLogTimestamp() kitlog.Valuer
	// endLogFields returns any extra fields appended to the terminal
	// (succeeded/failed) wrapper log, e.g. an event-end `time` that overrides
	// `ts` for that line.
	endLogFields(duration float64) []any
	// fallbackDuration computes the probe duration when the prober reports none.
	fallbackDuration(wallStart time.Time) time.Duration
}

// wallClock is the scheduled scraper's policy: real-time log timestamps and a
// duration fallback measured from the scrape's nominal time.
type wallClock struct {
	scheduledAt time.Time
}

func scheduledScrapeClock(scheduledAt time.Time) scrapeClock {
	return wallClock{scheduledAt: scheduledAt}
}

func (wallClock) wrapperLogTimestamp() kitlog.Valuer { return kitlog.DefaultTimestampUTC }

func (wallClock) endLogFields(float64) []any { return nil }

// fallbackDuration deliberately ignores wallStart: measuring from the scrape's
// nominal time is the scheduled path's pre-existing behavior.
func (w wallClock) fallbackDuration(time.Time) time.Duration {
	return time.Since(w.scheduledAt)
}

// logicalClock places a scrape's telemetry at a caller-supplied event time,
// which may be arbitrarily far in the past: the wrapper `ts` is pinned to
// eventTime and the terminal log is shifted to eventTime+duration, while the
// duration fallback still measures real elapsed time.
type logicalClock struct {
	eventTime time.Time
}

func logicalScrapeClock(eventTime time.Time) scrapeClock {
	return logicalClock{eventTime: eventTime}
}

func (l logicalClock) wrapperLogTimestamp() kitlog.Valuer {
	return kitlog.TimestampFormat(l.eventTime.UTC, time.RFC3339Nano)
}

func (l logicalClock) endLogFields(duration float64) []any {
	eventEnd := l.eventTime.Add(time.Duration(duration * float64(time.Second)))
	return []any{"time", eventEnd.UTC().Format(time.RFC3339Nano)}
}

func (logicalClock) fallbackDuration(wallStart time.Time) time.Duration {
	return time.Since(wallStart)
}
