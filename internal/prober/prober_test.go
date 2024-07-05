package prober

import (
	"context"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestProberFactoryCoverage(t *testing.T) {
	// This test will assert that the prober factory is handling all the
	// known check types (as defined in the synthetic_monitoring package).

	pf := NewProberFactory(nil, 0)
	ctx := context.Background()
	testLogger := zerolog.New(zerolog.NewTestWriter(t))

	for _, checkType := range sm.CheckTypeValues() {
		var check model.Check
		require.NoError(t, check.FromSM(sm.GetCheckInstance(checkType)))

		_, _, err := pf.New(ctx, testLogger, check)
		require.NotErrorIs(t, err, unsupportedCheckType)
	}
}
