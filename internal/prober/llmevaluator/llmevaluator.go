// Package llmevaluator implements the LLM Evaluator prober, which sends a
// prompt to an OpenAI-compatible LLM endpoint and evaluates the response
// against natural-language criteria using a judge proxy (sm-judge-proxy).
package llmevaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	smjudge "github.com/grafana/sm-judge-proxy/client"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	proberName = "llmevaluator"

	// maxResponseChars is the maximum number of rune characters we pass to the judge.
	maxResponseChars = 5000

	// maxLogResponseChars is the max number of rune characters logged for the LLM response.
	maxLogResponseChars = 1000

	// maxLogReasoningChars is the max number of rune characters logged per criterion reasoning.
	maxLogReasoningChars = 150

	// targetMaxTokens is the max_tokens sent to the target LLM.
	targetMaxTokens = 2048

	// targetResponseLimitBytes is the maximum bytes read from the target LLM response body (1 MB).
	targetResponseLimitBytes = 1 << 20
)

// judge request/response types are imported from github.com/grafana/sm-judge-proxy/client,
// generated from the canonical openapi.yaml in that repo.

// targetChatRequest is the OpenAI-compatible chat completions request body.
type targetChatRequest struct {
	Model       string          `json:"model"`
	Messages    []targetMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

// targetMessage is one message in the OpenAI chat format.
type targetMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// targetChatResponse is the minimal OpenAI-compatible response we need to parse.
type targetChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Prober implements the prober.Prober interface for LLM Evaluator checks.
type Prober struct {
	logger      zerolog.Logger
	check       model.Check
	settings    *sm.LLMEvaluatorSettings
	secretStore secrets.SecretProvider
	httpClient  *http.Client                 // shared transport for target LLM calls
	judgeClient *smjudge.ClientWithResponses // generated client for sm-judge-proxy
}

// NewProber creates a new LLMEvaluator Prober.
//
// Returns an error if settings are nil, criteria are empty, or if the endpoint
// contains a non-root path (which would cause /v1/v1/... doubling).
func NewProber(
	check model.Check,
	secretStore secrets.SecretProvider,
	judgeURI string,
	logger zerolog.Logger,
) (*Prober, error) {
	if check.Settings.LlmEvaluator == nil {
		return nil, fmt.Errorf("llm evaluator settings are nil")
	}

	// F1: Require at least one criterion.
	if len(check.Settings.LlmEvaluator.Criteria) == 0 {
		return nil, fmt.Errorf("llm evaluator requires at least one criterion")
	}

	// F4: Reject endpoints that already include a path component — callers
	// must supply a base URL only (e.g. "https://api.openai.com") because
	// callTargetLLM appends "/v1/chat/completions" itself.
	u, err := url.Parse(check.Settings.LlmEvaluator.Endpoint)
	if err != nil || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("llm evaluator endpoint must be a base URL with no path (got %q)", check.Settings.LlmEvaluator.Endpoint)
	}

	// judgeURI is required; an empty value produces a broken client that only
	// fails at the first probe fire, not at construction time.
	if judgeURI == "" {
		return nil, fmt.Errorf("judgeURI is required for LLM Evaluator checks")
	}

	// Use the check's configured timeout for the target LLM transport.
	// Rely on the context deadline (set by the scraper from check.Timeout) as
	// the budget for both calls combined; no client-level timeout needed.
	// Separate clients prevent the target and judge from sharing a timeout domain.
	targetHTTPClient := &http.Client{}
	judgeHTTPClient := &http.Client{}

	judgeClient, err := smjudge.NewClientWithResponses(
		judgeURI,
		smjudge.WithHTTPClient(judgeHTTPClient),
	)
	if err != nil {
		return nil, fmt.Errorf("creating judge client: %w", err)
	}

	return &Prober{
		logger:      logger.With().Str("prober", proberName).Logger(),
		check:       check,
		settings:    check.Settings.LlmEvaluator,
		secretStore: secretStore,
		httpClient:  targetHTTPClient,
		judgeClient: judgeClient,
	}, nil
}

// Name returns the prober name.
func (p *Prober) Name() string {
	return proberName
}

// Probe executes one LLM Evaluator check and emits metrics to registry.
//
// Returns (success bool, duration float64). Duration is always 0 — the
// scraper uses wall time because the prober doesn't produce
// probe_duration_seconds itself (that is handled by the scraper layer).
func (p *Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, l logger.Logger) (bool, float64) {
	basicOnly := p.check.BasicMetricsOnly

	// Step 1: Fetch API key from secret store.
	apiKey, err := p.secretStore.GetSecretValue(ctx, p.check.GlobalTenantID(), p.settings.ApiKeyRef)
	if err != nil {
		p.logger.Error().Err(err).
			Str("apiKeyRef", p.settings.ApiKeyRef).
			Str("reason", "target_secret_missing").
			Msg("failed to fetch target LLM API key")
		// F3: l.Log is Loki-bound (tenant-visible). Send only fixed fields, never raw err.
		_ = l.Log("msg", "probe failed", "reason", "target_secret_missing")
		return false, 0
	}

	// Step 2: Call target LLM.
	targetStart := time.Now()
	llmResponse, err := p.callTargetLLM(ctx, apiKey)
	targetDuration := time.Since(targetStart)

	if err != nil {
		reason := classifyTargetError(err)
		p.logger.Error().Err(err).
			Str("reason", reason).
			Str("endpoint", p.settings.Endpoint).
			Msg("target LLM call failed")
		// F3: reason only to Loki — no raw error details.
		_ = l.Log("msg", "probe failed", "reason", reason)
		return false, 0
	}

	// Step 3: Call judge proxy.
	judgeStart := time.Now()
	judgeResp, err := p.callJudge(ctx, llmResponse)
	judgeDuration := time.Since(judgeStart)

	if err != nil {
		reason := classifyJudgeError(err)
		p.logger.Error().Err(err).
			Str("reason", reason).
			Msg("judge proxy call failed")
		// F3: reason only to Loki — no raw error details.
		_ = l.Log("msg", "probe failed", "reason", reason)
		return false, 0
	}

	// Step 4: Emit metrics.
	p.emitMetrics(registry, basicOnly, judgeResp, targetDuration, judgeDuration)

	// Step 5: Log to Loki.
	p.emitLogs(l, llmResponse, judgeResp)

	// probe_success: 1 only if all criteria passed.
	// Score is computed by the proxy as passed_count/total_count (integer arithmetic),
	// so 1.0 is exact when all criteria pass — no floating-point tolerance needed.
	success := judgeResp.Score >= 1.0
	return success, 0
}

// callTargetLLM sends the configured prompt to the target LLM and returns the truncated response text.
func (p *Prober) callTargetLLM(ctx context.Context, apiKey string) (string, error) {
	messages := make([]targetMessage, 0, 2)
	if p.settings.SystemPrompt != "" {
		messages = append(messages, targetMessage{Role: "system", Content: p.settings.SystemPrompt})
	}
	messages = append(messages, targetMessage{Role: "user", Content: p.settings.Prompt})

	reqBody := targetChatRequest{
		Model:       p.settings.Model,
		Messages:    messages,
		Temperature: 0,
		MaxTokens:   targetMaxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling target LLM request: %w", err)
	}

	targetURL := strings.TrimRight(p.settings.Endpoint, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating target LLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", &targetHTTPError{cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", &targetRateLimitError{statusCode: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		return "", &targetClientError{statusCode: resp.StatusCode}
	}

	// F8: Bound target LLM response body reads at 1 MB.
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, targetResponseLimitBytes))
	if err != nil {
		return "", fmt.Errorf("reading target LLM response body: %w", err)
	}

	var chatResp targetChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("decoding target LLM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("target LLM returned no choices")
	}

	// F5: Rune-safe truncation prevents splitting a multi-byte UTF-8 sequence.
	return truncateString(chatResp.Choices[0].Message.Content, maxResponseChars), nil
}

// callJudge sends the LLM response and criteria to the judge proxy and returns the evaluation.
// Uses the generated ClientWithResponses from github.com/grafana/sm-judge-proxy/client.
func (p *Prober) callJudge(ctx context.Context, llmResponse string) (*smjudge.EvaluateLLMResponse, error) {
	req := smjudge.EvaluateRequest{
		LlmResponse: llmResponse,
		Criteria:    p.settings.Criteria,
		Metadata: smjudge.EvaluateMetadata{
			// TenantId is the local (region-scoped) ID, intentionally consistent
			// with how k6runner.CheckInfoFromSM sends the same field. The judge
			// proxy uses metadata only for logging context, not routing.
			TenantId: p.check.TenantId,
			RegionId: int64(p.check.RegionId),
			CheckId:  p.check.Id,
		},
	}

	resp, err := p.judgeClient.EvaluateWithResponse(ctx, req)
	if err != nil {
		// Connection refused or other network error → judge_unavailable.
		return nil, &judgeUnavailableError{cause: err}
	}

	if resp.JSON200 != nil {
		if len(resp.JSON200.Results) == 0 {
			// A 200 with no results is a judge bug — don't falsely report success.
			return nil, fmt.Errorf("judge proxy returned 200 with empty results")
		}

		return resp.JSON200, nil
	}

	// Non-200 response: use typed error bodies where available (our own service, safe to surface).
	if resp.JSON400 != nil {
		return nil, fmt.Errorf("judge proxy validation error: %s", resp.JSON400.Message)
	}

	if resp.JSON500 != nil {
		return nil, fmt.Errorf("judge proxy error: %s", resp.JSON500.Message)
	}

	return nil, fmt.Errorf("judge proxy returned HTTP %d", resp.StatusCode())
}

// emitMetrics registers and sets all probe_llm_* metrics on the registry.
// probe_success, probe_duration_seconds, and sm_check_info are owned by the
// scraper layer and MUST NOT be emitted here.
func (p *Prober) emitMetrics(
	registry *prometheus.Registry,
	basicOnly bool,
	judgeResp *smjudge.EvaluateLLMResponse,
	targetDuration time.Duration,
	judgeDuration time.Duration,
) {
	evalScore := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_llm_evaluation_score",
		Help: "Overall evaluation score from 0 to 1",
	})
	registry.MustRegister(evalScore)
	evalScore.Set(judgeResp.Score)

	if basicOnly {
		return
	}

	// Per-criterion pass/fail.
	criteriaPassedVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_llm_criteria_passed",
		Help: "Whether the criterion was passed",
	}, []string{"criterion_index"})
	registry.MustRegister(criteriaPassedVec)

	for _, result := range judgeResp.Results {
		label := strconv.Itoa(result.CriterionIndex)
		val := 0.0
		if result.Passed {
			val = 1.0
		}
		criteriaPassedVec.WithLabelValues(label).Set(val)
	}

	// Target LLM call latency.
	targetDurGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_llm_target_duration_seconds",
		Help: "Time to receive a response from the target LLM",
	})
	registry.MustRegister(targetDurGauge)
	targetDurGauge.Set(targetDuration.Seconds())

	// Judge call latency.
	judgeDurGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_llm_judge_seconds",
		Help: "Time to receive a response from the judge proxy",
	})
	registry.MustRegister(judgeDurGauge)
	judgeDurGauge.Set(judgeDuration.Seconds())

	// Judge token usage.
	judgeTokensVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_llm_judge_tokens_total",
		Help: "Number of tokens used by the judge LLM",
	}, []string{"role"})
	registry.MustRegister(judgeTokensVec)
	judgeTokensVec.WithLabelValues("input").Set(float64(judgeResp.JudgeInputTokens))
	judgeTokensVec.WithLabelValues("output").Set(float64(judgeResp.JudgeOutputTokens))
}

// emitLogs writes one structured log entry per check execution via the kit logger.
func (p *Prober) emitLogs(l logger.Logger, llmResponse string, judgeResp *smjudge.EvaluateLLMResponse) {
	passed := 0
	for _, r := range judgeResp.Results {
		if r.Passed {
			passed++
		}
	}

	keyvals := []any{
		"msg", "llm evaluator probe result",
		"score", judgeResp.Score,
		"criteria_passed", passed,
		"criteria_total", len(judgeResp.Results),
		// F5: Rune-safe truncation.
		"llm_response", truncateString(llmResponse, maxLogResponseChars),
	}

	for _, r := range judgeResp.Results {
		// F5: Rune-safe truncation.
		keyvals = append(keyvals,
			fmt.Sprintf("criterion_%d_passed", r.CriterionIndex), r.Passed,
			fmt.Sprintf("criterion_%d_reasoning", r.CriterionIndex), truncateString(r.Reasoning, maxLogReasoningChars),
		)
	}

	_ = l.Log(keyvals...)
}

// truncateString returns s truncated to at most maxChars Unicode code points.
// This is safe for multi-byte UTF-8 strings; byte-slicing with [:n] is not.
func truncateString(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}

// --- Error types for classifying failure reasons ---

// targetHTTPError wraps connection/timeout errors to the target LLM.
type targetHTTPError struct {
	cause error
}

func (e *targetHTTPError) Error() string { return e.cause.Error() }
func (e *targetHTTPError) Unwrap() error { return e.cause }

// targetRateLimitError is returned when the target LLM returns HTTP 429.
type targetRateLimitError struct {
	statusCode int
}

func (e *targetRateLimitError) Error() string {
	return fmt.Sprintf("target LLM rate limited (HTTP %d)", e.statusCode)
}

// targetClientError is returned for other 4xx responses.
type targetClientError struct {
	statusCode int
}

func (e *targetClientError) Error() string {
	return fmt.Sprintf("target LLM client error (HTTP %d)", e.statusCode)
}

// judgeUnavailableError is returned when the judge proxy is unreachable.
type judgeUnavailableError struct {
	cause error
}

func (e *judgeUnavailableError) Error() string { return e.cause.Error() }
func (e *judgeUnavailableError) Unwrap() error { return e.cause }

// classifyTargetError maps a target LLM error to a reason label string.
func classifyTargetError(err error) string {
	var rateLimit *targetRateLimitError
	if errors.As(err, &rateLimit) {
		return "target_rate_limited"
	}

	var clientErr *targetClientError
	if errors.As(err, &clientErr) {
		return "target_client_error"
	}

	// Connection error, timeout, DNS failure etc.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "target_timeout"
	}

	return "target_error"
}

// classifyJudgeError maps a judge proxy error to a reason label string.
func classifyJudgeError(err error) string {
	var unavail *judgeUnavailableError
	if errors.As(err, &unavail) {
		return "judge_unavailable"
	}
	return "judge_error"
}
