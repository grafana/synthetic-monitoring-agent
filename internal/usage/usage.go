package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/feature"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// Reporter represents a way of communicating reports to different backend systems.
type Reporter interface {
	ReportProbe(ctx context.Context, probe sm.Probe, features feature.Collection) error
}

// report represents a specific usage event that will be sent to https://stats.grafana.com to be processed and stored.
// Each attribute represents a column in a BigQuery table that can be easily searched.
// Adding new attributes to report will not automatically update the table, and instead needs to be handled in https://github.com/grafana/usage-stats
type report struct {
	CreatedAt    string `json:"createdAt"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Report       string `json:"report"`
	Version      string `json:"version"`
	Public       bool   `json:"public"`
	Features     string `json:"features"`
	UsageStatsId string `json:"usageStatsId"`
	TenantID     int64  `json:"tenantId"`
	ProbeID      int64  `json:"probeId"`
}

// HTTPReporter represents
type HTTPReporter struct {
	endpoint string
	client   *http.Client
}

const (
	// UsageReportApplication aligns with the usage-stats service endpoint defined
	// in github.com/grafana/usage-stats for synthetic monitoring agents
	UsageStatsApplication = "synthetic-monitoring-agent-usage-report"
	// Base Endpoint for usage stats
	ProdStatsEndpoint = "https://stats.grafana.org"
)

func NewHTTPReporter(endpoint string) *HTTPReporter {
	return &HTTPReporter{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// submitReport is responsible for sending a report to the stats endpoint via an http POST request. The primary concern is that
// the http server responds with http.StatusOK. Otherwise, there are no other expected responses.
func (hr *HTTPReporter) submitReport(ctx context.Context, report *report) error {
	jsonData, err := json.Marshal(&report)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	endpoint := fmt.Sprintf("%s/%s", hr.endpoint, UsageStatsApplication)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hr.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

// ReportProbe creates a report from the probe and then sends the report to the stats api endpoint via the report method.
func (hr *HTTPReporter) ReportProbe(ctx context.Context, probe sm.Probe, features feature.Collection) error {
	hashValue, err := hashOfProbe(probe)
	if err != nil {
		return err
	}
	r := &report{
		Report:       probe.String(),
		CreatedAt:    time.Now().Format(time.RFC3339),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Version:      probe.Version,
		UsageStatsId: hashValue,
		Features:     features.String(),
		Public:       probe.Public,
		TenantID:     probe.TenantId,
		ProbeID:      probe.Id,
	}
	return hr.submitReport(ctx, r)
}

type NoOPReporter struct{}

func NewNoOPReporter() *NoOPReporter {
	return &NoOPReporter{}
}

func (r *NoOPReporter) ReportProbe(_ context.Context, _ sm.Probe, _ feature.Collection) error {
	return nil
}

// hashOfProbe returns a string representation of the probe by concatenating identifying attributes and
// generating an FNV(https://en.wikipedia.org/wiki/Fowler%E2%80%93Noll%E2%80%93Vo_hash_function) hash.
// The hash function is deterministic to ensure a probe always results in the same string.
func hashOfProbe(p sm.Probe) (string, error) {
	s := p.Name + strconv.FormatInt(p.Id, 10) + p.Region + strconv.FormatBool(p.Public) + strconv.FormatInt(p.TenantId, 10)
	h := fnv.New64a()
	_, err := h.Write([]byte(s))
	if err != nil {
		return "", err
	}
	sum := h.Sum64()
	return strconv.FormatUint(sum, 10), nil
}
