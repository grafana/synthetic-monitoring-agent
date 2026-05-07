package llmevaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	smjudge "github.com/grafana/sm-judge-proxy/client"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// testLogger is a minimal kit-style logger that satisfies logger.Logger and
// discards all output — sufficient for probe test calls that don't need log inspection.
type testLogger struct{}

func (testLogger) Log(_ ...any) error { return nil }

// capturingLogger records all key/value pairs passed to Log so tests can
// inspect what was written to Loki (the tenant-visible log stream).
type capturingLogger struct {
	entries []map[string]any
}

func (c *capturingLogger) Log(keyvals ...any) error {
	m := make(map[string]any, len(keyvals)/2)
	for i := 0; i+1 < len(keyvals); i += 2 {
		key, _ := keyvals[i].(string)
		m[key] = keyvals[i+1]
	}
	c.entries = append(c.entries, m)
	return nil
}

// mockSecretProvider implements secrets.SecretProvider for tests.
type mockSecretProvider struct {
	key   string
	value string
	err   error
}

func (m mockSecretProvider) GetSecretCredentials(_ context.Context, _ model.GlobalID) (*sm.SecretStore, error) {
	return &sm.SecretStore{}, nil
}

func (m mockSecretProvider) GetSecretValue(_ context.Context, _ model.GlobalID, secretKey string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.value, nil
}

func (m mockSecretProvider) IsProtocolSecretsEnabled() bool { return true }

// buildCheck creates a model.Check with LLMEvaluatorSettings for tests.
func buildCheck(endpoint string, criteria []string, basicMetricsOnly bool) model.Check {
	var c model.Check
	c.Id = 42
	c.TenantId = 1
	c.Target = endpoint
	c.BasicMetricsOnly = basicMetricsOnly
	c.Settings.LlmEvaluator = &sm.LLMEvaluatorSettings{
		Endpoint:  endpoint,
		Model:     "gpt-4o-mini",
		ApiKeyRef: "my-api-key",
		Prompt:    "What is Grafana?",
		Criteria:  criteria,
	}
	return c
}

// fakeChatCompletions returns a fixed chat completion response.
func fakeChatCompletions(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// fakeJudge returns a fixed evaluation response.
func fakeJudge(results []smjudge.CriterionResult, score float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/evaluate" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		resp := smjudge.EvaluateLLMResponse{
			Score:             score,
			Results:           results,
			JudgeInputTokens:  120,
			JudgeOutputTokens: 80,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// gatherMetrics collects all metrics from the registry into a name→family map.
func gatherMetrics(t *testing.T, registry *prometheus.Registry) map[string]*dto.MetricFamily {
	t.Helper()
	mfs, err := registry.Gather()
	require.NoError(t, err)
	result := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		result[mf.GetName()] = mf
	}
	return result
}

// gaugeValue returns the value of a gauge metric by name (no labels).
func gaugeValue(t *testing.T, mfs map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	mf, ok := mfs[name]
	require.Truef(t, ok, "metric %q not found; available: %v", name, metricNames(mfs))
	require.Len(t, mf.GetMetric(), 1)
	return mf.GetMetric()[0].GetGauge().GetValue()
}

// gaugeValueWithLabel returns the value of a gauge metric with a specific label value.
func gaugeValueWithLabel(t *testing.T, mfs map[string]*dto.MetricFamily, name, labelName, labelValue string) float64 {
	t.Helper()
	mf, ok := mfs[name]
	require.Truef(t, ok, "metric %q not found; available: %v", name, metricNames(mfs))
	for _, m := range mf.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == labelName && lp.GetValue() == labelValue {
				return m.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("metric %q with label %s=%s not found", name, labelName, labelValue)
	return 0
}

func metricNames(mfs map[string]*dto.MetricFamily) []string {
	names := make([]string, 0, len(mfs))
	for k := range mfs {
		names = append(names, k)
	}
	return names
}

// newProber constructs a prober with test servers already set up.
func newProber(t *testing.T, check model.Check, store secrets.SecretProvider, judgeURI string) *Prober {
	t.Helper()
	logger := zerolog.New(zerolog.NewTestWriter(t))
	p, err := NewProber(check, store, judgeURI, logger)
	require.NoError(t, err)
	return p
}

// TestProber_AllCriteriaPass verifies that probe_success=1 and score=1.0 when all criteria pass.
func TestProber_AllCriteriaPass(t *testing.T) {
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	criteria := []string{"Mentions monitoring"}
	results := []smjudge.CriterionResult{{CriterionIndex: 0, Passed: true, Reasoning: "yes"}}
	judgeSrv := httptest.NewServer(fakeJudge(results, 1.0))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, criteria, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	success, duration := p.Probe(context.Background(), check.Target, registry, testLogger{})

	require.True(t, success, "expected probe_success=1 when all criteria pass")
	require.Equal(t, float64(0), duration, "prober should return 0 duration (scraper provides wall time)")

	mfs := gatherMetrics(t, registry)
	require.InDelta(t, 1.0, gaugeValue(t, mfs, "probe_llm_evaluation_score"), 0.001)
}

// TestProber_OneCriterionFails verifies probe_success=0 and score=0.5 when one of two criteria fails.
func TestProber_OneCriterionFails(t *testing.T) {
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	criteria := []string{"Mentions monitoring", "Mentions competitors"}
	results := []smjudge.CriterionResult{
		{CriterionIndex: 0, Passed: true, Reasoning: "yes"},
		{CriterionIndex: 1, Passed: false, Reasoning: "no"},
	}
	judgeSrv := httptest.NewServer(fakeJudge(results, 0.5))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, criteria, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	success, _ := p.Probe(context.Background(), check.Target, registry, testLogger{})

	require.False(t, success, "expected probe_success=0 when not all criteria pass")

	mfs := gatherMetrics(t, registry)
	require.InDelta(t, 0.5, gaugeValue(t, mfs, "probe_llm_evaluation_score"), 0.001)
}

// TestProber_CriterionMetricLabels verifies that probe_llm_criteria_passed is emitted
// with the correct criterion_index labels.
func TestProber_CriterionMetricLabels(t *testing.T) {
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	criteria := []string{"Mentions monitoring", "Mentions observability"}
	results := []smjudge.CriterionResult{
		{CriterionIndex: 0, Passed: true, Reasoning: "yes"},
		{CriterionIndex: 1, Passed: false, Reasoning: "no"},
	}
	judgeSrv := httptest.NewServer(fakeJudge(results, 0.5))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, criteria, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	_, _ = p.Probe(context.Background(), check.Target, registry, testLogger{})

	mfs := gatherMetrics(t, registry)
	require.InDelta(t, 1.0, gaugeValueWithLabel(t, mfs, "probe_llm_criteria_passed", "criterion_index", "0"), 0.001, "criterion 0 should be passed=1")
	require.InDelta(t, 0.0, gaugeValueWithLabel(t, mfs, "probe_llm_criteria_passed", "criterion_index", "1"), 0.001, "criterion 1 should be passed=0")
}

// TestProber_TargetLLM429 verifies that a 429 from the target LLM results in probe_success=0
// and the judge is never called.
func TestProber_TargetLLM429(t *testing.T) {
	judgeCallCount := 0

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}))
	defer targetSrv.Close()

	judgeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		judgeCallCount++
		http.Error(w, "unexpected judge call", http.StatusInternalServerError)
	}))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, []string{"criterion"}, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	success, _ := p.Probe(context.Background(), check.Target, registry, testLogger{})

	require.False(t, success, "expected probe_success=0 for target LLM 429")
	require.Equal(t, 0, judgeCallCount, "judge must not be called when target LLM returns 429")
}

// TestProber_SecretMissing verifies that a missing secret results in probe_success=0
// and neither the target LLM nor the judge are called.
func TestProber_SecretMissing(t *testing.T) {
	targetCallCount := 0
	judgeCallCount := 0

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCallCount++
		_, _ = w.Write([]byte("{}"))
	}))
	defer targetSrv.Close()

	judgeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		judgeCallCount++
		_, _ = w.Write([]byte("{}"))
	}))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, []string{"criterion"}, false)
	store := mockSecretProvider{err: fmt.Errorf("secret not found: my-api-key")}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	success, _ := p.Probe(context.Background(), check.Target, registry, testLogger{})

	require.False(t, success, "expected probe_success=0 when secret is missing")
	require.Equal(t, 0, targetCallCount, "target LLM must not be called when secret is missing")
	require.Equal(t, 0, judgeCallCount, "judge must not be called when secret is missing")
}

// TestProber_JudgeUnavailable verifies probe_success=0 when the judge proxy is not reachable.
func TestProber_JudgeUnavailable(t *testing.T) {
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	// Use a valid-looking but not-listening address.
	judgeURI := "http://127.0.0.1:19999"

	check := buildCheck(targetSrv.URL, []string{"criterion"}, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}

	logger := zerolog.New(zerolog.NewTestWriter(t))
	p, err := NewProber(check, store, judgeURI, logger)
	require.NoError(t, err)

	// Short context timeout so test doesn't hang waiting for TCP.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	registry := prometheus.NewRegistry()
	success, _ := p.Probe(ctx, check.Target, registry, testLogger{})

	require.False(t, success, "expected probe_success=0 when judge is unavailable")
}

// TestNewProber_EmptyCriteria verifies that NewProber returns an error when no
// criteria are provided (F1).
func TestNewProber_EmptyCriteria(t *testing.T) {
	check := buildCheck("http://example.com", []string{} /* empty */, false)
	logger := zerolog.New(zerolog.NewTestWriter(t))
	_, err := NewProber(check, mockSecretProvider{}, "http://judge", logger)
	require.Error(t, err)
	require.Contains(t, err.Error(), "criterion")
}

// TestNewProber_EmptyJudgeURI verifies that NewProber rejects an empty judgeURI
// rather than creating a client that fails silently at probe time.
func TestNewProber_EmptyJudgeURI(t *testing.T) {
	t.Parallel()
	check := buildCheck("http://example.com", []string{"criterion"}, false)
	logger := zerolog.New(zerolog.NewTestWriter(t))
	_, err := NewProber(check, mockSecretProvider{}, "" /* empty */, logger)
	require.Error(t, err)
	require.Contains(t, err.Error(), "judgeURI")
}

// TestProber_JudgeEmptyResults verifies that a 200 response with zero results is
// treated as a probe failure rather than a false success.
func TestProber_JudgeEmptyResults(t *testing.T) {
	t.Parallel()
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	// Judge returns score=1.0 but empty results — should NOT be probe_success=1.
	judgeSrv := httptest.NewServer(fakeJudge([]smjudge.CriterionResult{}, 1.0))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, []string{"criterion"}, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	success, _ := p.Probe(context.Background(), check.Target, registry, testLogger{})

	require.False(t, success, "empty results from judge must not produce probe_success=1")
}

// TestNewProber_EndpointWithPath verifies that NewProber returns an error when the
// endpoint already contains a path component, preventing /v1/v1/... doubling (F4).
func TestNewProber_EndpointWithPath(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
	}{
		{"with /v1 path", "https://api.openai.com/v1"},
		{"with full path", "https://api.openai.com/v1/chat/completions"},
		{"with subpath", "https://example.com/openai"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := buildCheck(tc.endpoint, []string{"criterion"}, false)
			logger := zerolog.New(zerolog.NewTestWriter(t))
			_, err := NewProber(check, mockSecretProvider{}, "http://judge", logger)
			require.Errorf(t, err, "expected error for endpoint %q", tc.endpoint)
			require.Contains(t, err.Error(), "base URL")
		})
	}
}

// TestProber_JudgeHTTP500 verifies that a non-200 judge response results in
// probe_success=0, that the reason is classified as "judge_error", and that the
// raw error body does NOT appear in the Loki log (verifying F2 and F3).
func TestProber_JudgeHTTP500(t *testing.T) {
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	const sensitiveBody = `{"error":"internal_secret_key=abc123"}`

	judgeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, sensitiveBody, http.StatusInternalServerError)
	}))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, []string{"criterion"}, false)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}

	logger := zerolog.New(zerolog.NewTestWriter(t))
	p, err := NewProber(check, store, judgeSrv.URL, logger)
	require.NoError(t, err)

	cl := &capturingLogger{}
	registry := prometheus.NewRegistry()
	success, _ := p.Probe(context.Background(), check.Target, registry, cl)

	// Probe must fail.
	require.False(t, success, "expected probe_success=0 for judge HTTP 500")

	// Loki log must contain reason but NOT the raw error body (F2/F3).
	require.Len(t, cl.entries, 1, "expected exactly one log entry")
	entry := cl.entries[0]
	require.Equal(t, "probe failed", entry["msg"], "log msg should be 'probe failed'")
	require.Equal(t, "judge_error", entry["reason"], "reason should be judge_error")
	// The raw error body must not appear anywhere in the Loki log entry.
	for k, v := range entry {
		s := fmt.Sprintf("%v", v)
		require.NotContains(t, s, sensitiveBody,
			"sensitive judge error body must not appear in Loki log field %q", k)
	}
}

// TestProber_BasicMetricsOnly verifies that per-criterion and latency metrics are
// NOT emitted when BasicMetricsOnly is true, but probe_llm_evaluation_score IS.
func TestProber_BasicMetricsOnly(t *testing.T) {
	targetSrv := httptest.NewServer(fakeChatCompletions("Grafana is a monitoring platform."))
	defer targetSrv.Close()

	criteria := []string{"Mentions monitoring", "Mentions observability"}
	results := []smjudge.CriterionResult{
		{CriterionIndex: 0, Passed: true, Reasoning: "yes"},
		{CriterionIndex: 1, Passed: true, Reasoning: "yes"},
	}
	judgeSrv := httptest.NewServer(fakeJudge(results, 1.0))
	defer judgeSrv.Close()

	check := buildCheck(targetSrv.URL, criteria, true /* basicMetricsOnly */)
	store := mockSecretProvider{key: "my-api-key", value: "test-key"}
	p := newProber(t, check, store, judgeSrv.URL)

	registry := prometheus.NewRegistry()
	success, _ := p.Probe(context.Background(), check.Target, registry, testLogger{})

	require.True(t, success)

	mfs := gatherMetrics(t, registry)

	// Must be present in basic mode.
	require.Contains(t, mfs, "probe_llm_evaluation_score", "evaluation score must be present in basic mode")

	// Must be absent in basic mode.
	require.NotContains(t, mfs, "probe_llm_criteria_passed", "criterion metric must be absent in basic mode")
	require.NotContains(t, mfs, "probe_llm_target_duration_seconds", "target duration must be absent in basic mode")
	require.NotContains(t, mfs, "probe_llm_judge_seconds", "judge latency must be absent in basic mode")
	require.NotContains(t, mfs, "probe_llm_judge_tokens_total", "judge tokens must be absent in basic mode")
}
