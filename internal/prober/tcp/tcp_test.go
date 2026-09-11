package tcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/prometheus/blackbox_exporter/config"
	bbeprober "github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := Prober.Name(Prober{})
	require.Equal(t, name, "tcp")
}

func TestNewProber(t *testing.T) {
	testcases := map[string]struct {
		input       model.Check
		expected    Prober
		ExpectError bool
	}{
		"default": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Tcp: &sm.TcpSettings{},
					},
				},
			},
			expected: Prober{
				config: config.Module{
					Prober:  "tcp",
					Timeout: 0,
					TCP: config.TCPProbe{
						IPProtocol:         "ip6",
						IPProtocolFallback: true,
						QueryResponse:      []config.QueryResponse{},
					},
				},
			},
			ExpectError: false,
		},
		"no-settings": {
			input: model.Check{
				Check: sm.Check{
					Target: "www.grafana.com",
					Settings: sm.CheckSettings{
						Tcp: nil,
					},
				},
			},
			expected:    Prober{},
			ExpectError: true,
		},
	}

	ctx := testCtx(context.Background(), t)

	for name, testcase := range testcases {
		logger := zerolog.New(io.Discard)

		t.Run(name, func(t *testing.T) {
			actual, err := NewProber(ctx, testcase.input, logger)
			require.Equal(t, &testcase.expected, &actual)

			if testcase.ExpectError {
				require.Error(t, err, "unsupported check")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSettingsToModule(t *testing.T) {
	testcases := map[string]struct {
		input    sm.TcpSettings
		expected config.Module
	}{
		"default": {
			input: sm.TcpSettings{},
			expected: config.Module{
				Prober:  "tcp",
				Timeout: 0,
				TCP: config.TCPProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					QueryResponse:      []config.QueryResponse{},
				},
			},
		},
		"query-response": {
			input: sm.TcpSettings{
				QueryResponse: []sm.TCPQueryResponse{
					{Send: []byte("EHLO sm"), Expect: []byte("^250")},
					{Send: []byte("STARTTLS"), Expect: []byte("^220"), StartTLS: true},
					{Send: []byte("PING")},
					{Expect: []byte("PONG")},
				},
			},
			expected: config.Module{
				Prober:  "tcp",
				Timeout: 0,
				TCP: config.TCPProbe{
					IPProtocol:         "ip6",
					IPProtocolFallback: true,
					QueryResponse: []config.QueryResponse{
						{Send: "EHLO sm", Expect: config.MustNewRegexp("^250")},
						{Send: "STARTTLS", Expect: config.MustNewRegexp("^220"), StartTLS: true},
						{Send: "PING"},
						{Expect: config.MustNewRegexp("PONG")},
					},
				},
			},
		},
		"partial-settings": {
			input: sm.TcpSettings{
				SourceIpAddress: "0.0.0.0",
				Tls:             true,
				IpVersion:       1,
			},
			expected: config.Module{
				Prober:  "tcp",
				Timeout: 0,
				TCP: config.TCPProbe{
					IPProtocol:         "ip4",
					IPProtocolFallback: false,
					QueryResponse:      []config.QueryResponse{},
					TLS:                true,
					SourceIPAddress:    "0.0.0.0",
				},
			},
		},
	}

	ctx := testCtx(context.Background(), t)

	for name, testcase := range testcases {
		logger := zerolog.New(io.Discard)

		t.Run(name, func(t *testing.T) {
			actual, err := settingsToModule(ctx, &testcase.input, logger)
			require.NoError(t, err)
			require.Equal(t, &testcase.expected, &actual)
		})
	}
}

func testCtx(ctx context.Context, t *testing.T) context.Context {
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithDeadline(ctx, deadline)
		t.Cleanup(cancel)

		return ctx
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func TestProbeQueryResponse(t *testing.T) {
	testcases := map[string]struct {
		banner    string
		settings  sm.TcpSettings
		expectGot []string
	}{
		"client-speaks-first": {
			settings: sm.TcpSettings{
				QueryResponse: []sm.TCPQueryResponse{
					{Send: []byte("PING")},
					{Expect: []byte("^PONG")},
				},
			},
			expectGot: []string{"PING"},
		},
		"banner-then-send": {
			banner: "220 ready",
			settings: sm.TcpSettings{
				QueryResponse: []sm.TCPQueryResponse{
					{Expect: []byte("^220")},
					{Send: []byte("PING")},
					{Expect: []byte("^PONG")},
				},
			},
			expectGot: []string{"PING"},
		},
		"consecutive-sends": {
			settings: sm.TcpSettings{
				QueryResponse: []sm.TCPQueryResponse{
					{Send: []byte("HELO")},
					{Send: []byte("PING")},
					{Expect: []byte("^PONG")},
				},
			},
			expectGot: []string{"HELO", "PING"},
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			// Bounded so that a step waiting for input it should not wait for
			// fails here instead of hanging until the package timeout.
			ctx, cancel := context.WithTimeout(testCtx(context.Background(), t), 10*time.Second)
			defer cancel()

			module, err := settingsToModule(ctx, &testcase.settings, zerolog.New(io.Discard))
			require.NoError(t, err)

			addr, got := runEchoServer(t, testcase.banner)

			registry := prometheus.NewRegistry()
			success := bbeprober.ProbeTCP(ctx, addr, module, registry, slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.True(t, success)
			require.Equal(t, testcase.expectGot, got())
		})
	}
}

// runEchoServer accepts a single connection, optionally greets the client with
// banner, and answers every line it reads with "PONG". It returns the address to
// dial and a function that waits for the connection to close before reporting
// what the client sent.
func runEchoServer(t *testing.T, banner string) (string, func() []string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	var (
		sent []string
		done = make(chan struct{})
	)

	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		if banner != "" {
			_, _ = fmt.Fprintf(conn, "%s\n", banner)
		}

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			sent = append(sent, scanner.Text())

			if _, err := fmt.Fprintf(conn, "PONG\n"); err != nil {
				return
			}
		}
	}()

	return listener.Addr().String(), func() []string {
		t.Helper()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the server to finish")
		}

		return sent
	}
}
