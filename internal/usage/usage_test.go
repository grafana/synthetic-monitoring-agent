package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestUsageReporter_Report(t *testing.T) {

	// Create a test http server that intercepts requests to https://stats.grafana.net
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tests := map[string]struct {
		endpoint string
		features []string
		ctx      context.Context
		wantErr  bool
	}{
		"Send over zero features": {
			endpoint: ts.URL,
			features: []string{},
			ctx:      context.Background(),
		},
		"Send over a full list of features": {
			endpoint: ts.URL,
			ctx:      context.Background(),
			features: []string{feature.K6, feature.Traceroute, feature.ExperimentalDnsProber},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			features := feature.NewCollection()
			for _, f := range tt.features {
				_ = features.Set(f)
			}
			r := NewUsageReporter(tt.endpoint, features)
			if err := r.ReportProbe(tt.ctx, sm.Probe{}); (err != nil) != tt.wantErr {
				t.Errorf("Report() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
