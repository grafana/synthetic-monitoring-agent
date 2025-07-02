package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// Report represents a specific usage event that will be sent to https://stats.grafana.com to be processed and stored.
// Each attribute represents a column in a BigQuery table that can be easily searched.
// Adding new attributes to Report will not automatically update the table, and instead needs to be handled in https://github.com/grafana/usage-stats
type Report struct {
	CreatedAt    string `json:"createdAt"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Report       string `json:"report"`
	Version      string `json:"version"`
	Public       bool   `json:"public"`
	Features     string `json:"features"`
	UsageStatsId string `json:"usageStatsId"`
	TenantID     int64  `json:"tenantId"`
}

// UsageReporter represents
type UsageReporter struct {
	endpoint string
	features feature.Collection
}

const (
	// UsageReportApplication aligns with the usage-stats service endpoint defined
	// in github.com/grafana/usage-stats for synthetic monitoring agents
	UsageStatsApplication = "synthetic-monitoring-agent-usage-report"
	// Base Endpoint for usage stats
	DefaultUsageStatsEndpoint = "https://stats.grafana.com"
)

func NewUsageReporter(endpoint string, features feature.Collection) *UsageReporter {
	if endpoint == "" {
		endpoint = DefaultUsageStatsEndpoint
	}
	return &UsageReporter{
		endpoint: endpoint,
		features: features,
	}
}

// submitReport is responsible for sending a report to the stats endpoint via an http POST request. The primary concern is that
// the http server responds with http.StatusOK. Otherwise, there are no other expected responses.
func (r *UsageReporter) submitReport(ctx context.Context, report *Report) error {
	jsonData, err := json.Marshal(&report)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	endpoint := fmt.Sprintf("%s/%s", r.endpoint, UsageStatsApplication)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

// ReportProbe creates a Report from the probe and then sends the report to the stats api endpoint via the report method.
func (r *UsageReporter) ReportProbe(ctx context.Context, probe sm.Probe) error {
	report := &Report{
		Report:       probe.String(),
		CreatedAt:    time.Now().Format(time.RFC3339),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Version:      probe.Version,
		UsageStatsId: uuid.NewString(),
		Features:     r.features.String(),
		Public:       probe.Public,
		TenantID:     probe.TenantId,
	}
	return r.submitReport(ctx, report)
}
