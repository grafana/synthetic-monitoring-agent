package usage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestHTTPUsageReporter_Report(t *testing.T) {
	// Create a test http server that intercepts requests to https://stats.grafana.net
	t.Parallel()

	var gotReport *report

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotReport); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Clear these values out as they're dynamic attributes that are not important to validate the result of
		gotReport.UsageStatsId = ""
		gotReport.CreatedAt = ""
		gotReport.Report = ""
		w.WriteHeader(http.StatusOK)
	}))

	defer ts.Close()

	tests := map[string]struct {
		endpoint       string
		features       []string
		probe          sm.Probe
		wantErr        bool
		expectedReport *report
	}{
		"Send over zero features": {
			endpoint: ts.URL,
			features: []string{},
			expectedReport: &report{
				Public:   false,
				ProbeID:  1,
				TenantID: 1,
				Arch:     runtime.GOARCH,
				OS:       runtime.GOOS,
			},
			probe: sm.Probe{
				Id:       1,
				TenantId: 1,
				Public:   false,
			},
		},
		"Send over a single features": {
			endpoint: ts.URL,
			expectedReport: &report{
				Public:   false,
				ProbeID:  1,
				TenantID: 1,
				Arch:     runtime.GOARCH,
				OS:       runtime.GOOS,
				Features: "k6",
			},
			probe: sm.Probe{
				Id:       1,
				TenantId: 1,
				Public:   false,
			},
			features: []string{feature.K6},
		},
		"Send over a full list of features": {
			endpoint: ts.URL,
			expectedReport: &report{
				Public:   false,
				ProbeID:  1,
				TenantID: 1,
				Arch:     runtime.GOARCH,
				OS:       runtime.GOOS,
				Features: "experimental-dns-prober,k6,traceroute",
			},
			probe: sm.Probe{
				Id:       1,
				TenantId: 1,
				Public:   false,
			},
			features: []string{feature.K6, feature.Traceroute, feature.ExperimentalDnsProber},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			features := feature.NewCollection()
			for _, f := range tt.features {
				_ = features.Set(f)
			}
			r := NewHTTPReporter(tt.endpoint)
			if err := r.ReportProbe(t.Context(), tt.probe, features); (err != nil) != tt.wantErr {
				t.Errorf("Report() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.expectedReport, gotReport)
		})
	}
}

func TestNoOPUsageReporter_Report(t *testing.T) {
	r := NewNoOPReporter()
	if err := r.ReportProbe(t.Context(), sm.Probe{}, nil); err != nil {
		t.Errorf("Report() error = %v", err)
	}
}

func Test_hashOfProbe(t *testing.T) {
	tests := map[string]struct {
		p       sm.Probe
		want    string
		wantErr bool
	}{
		"Nil probe should return an error": {
			p:       sm.Probe{},
			want:    "4547005171780583226",
			wantErr: false,
		},
		"Probe with an ID should generate a consistent hash": {
			p: sm.Probe{
				Id: 1,
			},
			want:    "10535204341849580461",
			wantErr: false,
		},
		"Probe with everything set should return a consistent hash": {
			p: sm.Probe{
				Id:     1,
				Region: "test",
				Name:   "Some Name",
				Public: true,
			},
			want:    "1162427638690750921",
			wantErr: false,
		},
		"Probe with everything set but slightly different values should return a different hash value": {
			p: sm.Probe{
				Id:       10,
				Region:   "test",
				Name:     "Some Name",
				Public:   true,
				TenantId: 1,
			},
			want:    "9777170254191072896",
			wantErr: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := hashOfProbe(tt.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("hashOfProbe() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equalf(t, tt.want, got, "want=%v, got=%v", tt.want, got)
		})
	}
}
