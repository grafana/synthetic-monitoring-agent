// Package agentic is an experimental prober that runs a scripted check whose
// script is a natural-language prompt, by delegating it to an external
// browser-agent API. A check is agentic when its script starts with
// "/* sm-agentic" and SM_AGENTIC_ENDPOINT is set; otherwise it runs as a normal
// scripted check.
package agentic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

const (
	proberName = "agentic"
	marker     = "/* sm-agentic"

	// AdhocTimeout replaces the (<=180s) check timeout for ad-hoc runs.
	AdhocTimeout = 10 * time.Minute

	verdictInstructions = "\n\nWhen you are done, end your final answer with exactly one line: " +
		"`VERDICT: PASS` if every step succeeded, or `VERDICT: FAIL: <reason>` otherwise."
)

var verdictRe = regexp.MustCompile(`VERDICT:\s*(PASS|FAIL)`)

// Endpoint returns the browser-agent URL if check must run as an agentic check, or "" otherwise.
func Endpoint(check model.Check) string {
	if check.Settings.Scripted == nil || !strings.HasPrefix(strings.TrimSpace(string(check.Settings.Scripted.Script)), marker) {
		return ""
	}

	return os.Getenv("SM_AGENTIC_ENDPOINT")
}

type Prober struct {
	endpoint string
	prompt   string
	logger   zerolog.Logger
}

func NewProber(check model.Check, endpoint string, logger zerolog.Logger) Prober {
	prompt := strings.TrimSpace(string(check.Settings.Scripted.Script))
	prompt = strings.TrimPrefix(prompt, marker)
	prompt = strings.TrimSpace(strings.TrimSuffix(prompt, "*/"))

	return Prober{endpoint: endpoint, prompt: prompt + verdictInstructions, logger: logger}
}

func (p Prober) Name() string {
	return proberName
}

func (p Prober) Probe(ctx context.Context, target string, registry *prometheus.Registry, logger logger.Logger, executionID string) (bool, float64) {
	start := time.Now()

	answer, err := p.run(ctx, logger)
	if err != nil {
		p.logger.Error().Err(err).Msg("running agentic probe")
		_ = logger.Log("level", "error", "msg", "agentic check failed", "error", err.Error())

		return false, time.Since(start).Seconds()
	}

	m := verdictRe.FindAllStringSubmatch(answer, -1)
	success := len(m) > 0 && m[len(m)-1][1] == "PASS"

	_ = logger.Log("level", "info", "msg", "agentic verdict", "success", success, "answer", answer)

	return success, time.Since(start).Seconds()
}

// event is the subset of the AI SDK UI message stream we care about.
type event struct {
	Type      string `json:"type"`
	Delta     string `json:"delta"`
	ToolName  string `json:"toolName"`
	ErrorText string `json:"errorText"`
	Input     struct {
		Context string `json:"context"`
	} `json:"input"`
}

// run sends the prompt and returns the agent's final text block. Tool outputs
// are never logged: they carry screenshots and browser session credentials.
func (p Prober) run(ctx context.Context, logger logger.Logger) (string, error) {
	body, err := json.Marshal(map[string]string{"prompt": p.prompt})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("browser agent returned " + resp.Status)
	}

	return parseStream(resp.Body, logger)
}

func parseStream(r io.Reader, logger logger.Logger) (string, error) {
	var (
		text     strings.Builder
		finished bool
	)

	// bufio.Reader instead of Scanner: screenshot events are single lines of several MB.
	br := bufio.NewReader(r)

	for {
		line, err := br.ReadBytes('\n')

		if data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data: ")); ok {
			var ev event
			if json.Unmarshal(data, &ev) == nil {
				switch ev.Type {
				case "text-start":
					text.Reset()
				case "text-delta":
					text.WriteString(ev.Delta)
				case "text-end":
					_ = logger.Log("level", "info", "msg", "agent", "text", text.String())
				case "tool-input-available":
					_ = logger.Log("level", "info", "msg", "agent tool call", "tool", ev.ToolName, "context", ev.Input.Context)
				case "error":
					return "", errors.New("browser agent error: " + ev.ErrorText)
				case "finish":
					finished = true
				}
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", err
		}
	}

	if !finished {
		return "", errors.New("browser agent stream ended without finishing")
	}

	return text.String(), nil
}
