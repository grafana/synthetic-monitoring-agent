package agentic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestProbe(t *testing.T) {
	testcases := map[string]struct {
		stream   string
		expected bool
	}{
		"pass": {
			stream:   `{"type":"text-start"}|{"type":"text-delta","delta":"VERDICT: FAIL"}|{"type":"text-end"}|{"type":"tool-input-available","toolName":"browser","input":{"context":"click"}}|{"type":"text-start"}|{"type":"text-delta","delta":"All good.\nVERDICT:"}|{"type":"text-delta","delta":" PASS"}|{"type":"text-end"}|{"type":"finish"}|[DONE]`,
			expected: true,
		},
		"fail":       {stream: `{"type":"text-start"}|{"type":"text-delta","delta":"VERDICT: FAIL: no button"}|{"type":"finish"}`},
		"no verdict": {stream: `{"type":"text-start"}|{"type":"text-delta","delta":"done"}|{"type":"finish"}`},
		"error":      {stream: `{"type":"text-start"}|{"type":"text-delta","delta":"VERDICT: PASS"}|{"type":"error","errorText":"boom"}`},
		"truncated":  {stream: `{"type":"text-start"}|{"type":"text-delta","delta":"VERDICT: PASS"}`},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, ev := range strings.Split(tc.stream, "|") {
					fmt.Fprintf(w, "data: %s\n\n", ev)
				}
			}))
			t.Cleanup(srv.Close)

			check := model.Check{Check: sm.Check{Settings: sm.CheckSettings{Scripted: &sm.ScriptedSettings{
				Script: []byte("/* sm-agentic\nOpen the page.\n*/"),
			}}}}

			t.Setenv("SM_AGENTIC_ENDPOINT", srv.URL)
			endpoint := Endpoint(check)
			require.Equal(t, srv.URL, endpoint)

			p := NewProber(check, endpoint, zerolog.New(zerolog.NewTestWriter(t)))
			require.Contains(t, p.prompt, "Open the page.")
			require.NotContains(t, p.prompt, "sm-agentic")

			success, _ := p.Probe(context.Background(), "", nil, log.NewNopLogger(), "")
			require.Equal(t, tc.expected, success)
		})
	}

	// Regular scripts are left alone.
	t.Setenv("SM_AGENTIC_ENDPOINT", "http://x")
	require.Empty(t, Endpoint(model.Check{Check: sm.Check{Settings: sm.CheckSettings{Scripted: &sm.ScriptedSettings{Script: []byte("import http from 'k6/http'")}}}}))
}
