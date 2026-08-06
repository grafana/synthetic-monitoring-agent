package scraper

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScheduledScrapeClockUsesWallTime(t *testing.T) {
	scheduledAt := time.Now().Add(-time.Hour)
	wallStart := time.Now().Add(-time.Minute)
	clock := scheduledScrapeClock(scheduledAt)

	wrapperTimestamp := fmt.Sprint(clock.wrapperLogTimestamp()())
	parsedTimestamp, err := time.Parse(time.RFC3339Nano, wrapperTimestamp)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().UTC(), parsedTimestamp, time.Second)
	require.Empty(t, clock.endLogFields(0.25))
	require.Greater(t, clock.fallbackDuration(wallStart), 59*time.Minute)
}

func TestLogicalScrapeClockUsesEventTime(t *testing.T) {
	eventTime := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
	wallStart := time.Now().Add(-time.Minute)
	clock := logicalScrapeClock(eventTime)

	require.Equal(t, eventTime.Format(time.RFC3339Nano), fmt.Sprint(clock.wrapperLogTimestamp()()))
	require.Equal(t, []any{
		"time", eventTime.Add(250 * time.Millisecond).Format(time.RFC3339Nano),
	}, clock.endLogFields(0.25))
	require.Less(t, clock.fallbackDuration(wallStart), 2*time.Minute)
}
