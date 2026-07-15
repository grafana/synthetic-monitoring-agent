package backfill_test

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/pkg/backfill"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

// TestNewGeneratorForCheckHTTPByteMatchesNewGeneratorFromSM proves that the new
// type-dispatching constructor + CollectTyped path produces byte-identical
// output to the existing NewGeneratorFromSM + Collect path for an HTTP check.
func TestNewGeneratorForCheckHTTPByteMatchesNewGeneratorFromSM(t *testing.T) {
	ctx := context.Background()
	check := testCheck().Check // sm.Check
	probe := testProbe()
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	legacyGen, err := backfill.NewGeneratorFromSM(ctx, check, probe)
	require.NoError(t, err)
	legacyTS, legacyStreams, err := legacyGen.Collect(ctx, at, testSample(at))
	require.NoError(t, err)

	typedGen, err := backfill.NewGeneratorForCheck(ctx, check, probe)
	require.NoError(t, err)
	typedTS, typedStreams, err := typedGen.CollectTyped(ctx, at, backfill.NewTypedHTTPSample(testSample(at)))
	require.NoError(t, err)

	require.Equal(t, seriesSignatures(legacyTS), seriesSignatures(typedTS))
	require.Equal(t, streamSignatures(legacyStreams), streamSignatures(typedStreams))
}

func streamSignatures(streams backfill.Streams) []string {
	sigs := make([]string, 0, len(streams))
	for _, stream := range streams {
		for _, entry := range stream.Entries {
			sigs = append(sigs, entry.Timestamp.String()+"|"+entry.Line)
		}
	}
	return sigs
}

// TestNewGeneratorForCheckUnsupportedType proves that check types without a
// registered constructor fail with an error naming the supported types.
func TestNewGeneratorForCheckUnsupportedType(t *testing.T) {
	ctx := context.Background()
	probe := testProbe()

	for _, ct := range []sm.CheckType{sm.CheckTypeScripted, sm.CheckTypeMultiHttp, sm.CheckTypeBrowser} {
		check := testCheck().Check
		check.Settings = sm.CheckSettings{}
		switch ct {
		case sm.CheckTypeScripted:
			check.Settings.Scripted = &sm.ScriptedSettings{}
		case sm.CheckTypeMultiHttp:
			check.Settings.Multihttp = &sm.MultiHttpSettings{}
		case sm.CheckTypeBrowser:
			check.Settings.Browser = &sm.BrowserSettings{}
		}

		_, err := backfill.NewGeneratorForCheck(ctx, check, probe)
		require.Errorf(t, err, "expected error for check type %s", ct)
		require.Contains(t, err.Error(), "http", "error should list supported types for %s", ct)
	}
}

// TestNewGeneratorForCheckNoSettings proves that a bare sm.Check (no
// Settings sub-message set) errors before dispatch instead of panicking:
// sm.Check.Type() panics ("unhandled check type") when Settings is entirely
// zero-valued, so NewGeneratorForCheck must detect that case up front.
func TestNewGeneratorForCheckNoSettings(t *testing.T) {
	ctx := context.Background()
	check := testCheck().Check
	check.Settings = sm.CheckSettings{}
	probe := testProbe()

	require.NotPanics(t, func() {
		_, err := backfill.NewGeneratorForCheck(ctx, check, probe)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no settings")
	})
}
