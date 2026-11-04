package fleet

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	srvtypes "github.com/open-telemetry/opamp-go/server/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
)

func configMap(bodies map[string]string) *protobufs.AgentConfigMap {
	m := &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigObject{}}
	for name, body := range bodies {
		m.ConfigMap[name] = &protobufs.AgentConfigObject{Body: []byte(body), ContentType: "text/yaml"}
	}

	return m
}

func TestParseRemoteConfig(t *testing.T) {
	testcases := map[string]struct {
		bodies    map[string]string
		expected  remoteSettings
		wantFound bool
		wantErr   bool
	}{
		"empty":           {bodies: map[string]string{"": ""}},
		"unrelated":       {bodies: map[string]string{"a": "receivers:\n  otlp: {}\n"}},
		"top level":       {bodies: map[string]string{"a": "synthetic_monitoring:\n  log_level: debug\n"}, expected: remoteSettings{LogLevel: "debug"}, wantFound: true},
		"under extension": {bodies: map[string]string{"a": "extensions:\n  synthetic_monitoring:\n    log_level: warn\n"}, expected: remoteSettings{LogLevel: "warn"}, wantFound: true},
		"named extension": {bodies: map[string]string{"a": "extensions:\n  synthetic_monitoring/probe:\n    log_level: error\n"}, expected: remoteSettings{LogLevel: "error"}, wantFound: true},
		"same file twice": {bodies: map[string]string{"a": "synthetic_monitoring:\n  log_level: debug\nextensions:\n  synthetic_monitoring/x:\n    log_level: info\n"}, wantErr: true},
		"k6 http debug":   {bodies: map[string]string{"a": "synthetic_monitoring:\n  k6_http_debug: full\n"}, expected: remoteSettings{K6HTTPDebug: "full"}, wantFound: true},
		"unknown setting": {bodies: map[string]string{"a": "synthetic_monitoring:\n  api_token: nope\n"}, wantErr: true},
		"not a string":    {bodies: map[string]string{"a": "synthetic_monitoring:\n  log_level: [debug]\n"}, wantErr: true},
		"invalid yaml":    {bodies: map[string]string{"a": "synthetic_monitoring: [\n"}, wantErr: true},
		"duplicate": {bodies: map[string]string{
			"a": "synthetic_monitoring:\n  log_level: debug\n",
			"b": "synthetic_monitoring:\n  log_level: info\n",
		}, wantErr: true},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, found, err := parseRemoteConfig(configMap(tc.bodies))
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantFound, found)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	for _, s := range []string{"debug", "INFO", "warn", "error"} {
		_, err := parseLogLevel(s)
		require.NoError(t, err, s)
	}

	for _, s := range []string{"", "trace", "fatal", "disabled", "loud"} {
		_, err := parseLogLevel(s)
		require.Error(t, err, s)
	}
}

// TestAgentAppliesRemoteConfig checks that the agent applies the log level
// sent by the server, reports the status and effective config, and keeps
// talking to the server afterwards.
func TestAgentAppliesRemoteConfig(t *testing.T) {
	prevLevel := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prevLevel) })

	remoteConfig := &protobufs.AgentRemoteConfig{
		Config:     configMap(map[string]string{"": "synthetic_monitoring:\n  log_level: debug\n"}),
		ConfigHash: []byte("hash-1"),
	}

	var sent atomic.Bool
	msgs := make(chan *protobufs.AgentToServer, 100)

	srv := server.New(nil)
	err := srv.Start(server.StartSettings{
		ListenEndpoint: "127.0.0.1:0",
		Settings: server.Settings{
			Callbacks: srvtypes.Callbacks{
				OnConnecting: func(*http.Request) srvtypes.ConnectionResponse {
					return srvtypes.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: srvtypes.ConnectionCallbacks{
							OnMessage: func(_ context.Context, _ srvtypes.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
								msgs <- msg

								resp := &protobufs.ServerToAgent{InstanceUid: msg.InstanceUid}
								if !sent.Swap(true) {
									resp.RemoteConfig = remoteConfig
								}

								return resp
							},
						},
					}
				},
			},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })

	stateDir := t.TempDir()

	agent, err := New(Opts{
		Logger:     zerolog.New(zerolog.NewTestWriter(t)),
		URL:        "http://" + srv.Addr().String(),
		InstanceID: "1",
		Token:      "test-token",
		StateDir:   stateDir,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- agent.Run(ctx) }()

	// The status and effective config may arrive in the same message or in
	// separate ones.
	var (
		status    *protobufs.RemoteConfigStatus
		effective *protobufs.EffectiveConfig
		described bool
	)

	waitFor(t, msgs, func(m *protobufs.AgentToServer) bool {
		if m.RemoteConfigStatus.GetStatus() != protobufs.RemoteConfigStatuses_RemoteConfigStatuses_UNSET {
			status = m.RemoteConfigStatus
		}

		if m.EffectiveConfig != nil {
			effective = m.EffectiveConfig
		}

		if m.AgentDescription != nil && attributes(m.AgentDescription)["sm.agent.log_level"] == "debug" {
			described = true
		}

		return status != nil && effective != nil && described
	})

	require.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, status.Status)
	require.Equal(t, remoteConfig.ConfigHash, status.LastRemoteConfigHash)
	require.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
	require.Contains(t, string(effective.ConfigMap.ConfigMap[effectiveConfigName].Body), "log_level: debug")

	require.FileExists(t, stateDir+"/"+remoteConfigDumpFile)

	// The client must still be able to send after handling the config.
	agent.SetConnected(true)
	waitFor(t, msgs, func(m *protobufs.AgentToServer) bool { return m.Health != nil && m.Health.Healthy })

	cancel()
	require.NoError(t, <-done)
}

func TestApplyRemoteConfigK6HTTPDebug(t *testing.T) {
	prevLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(prevLevel) })

	cfg := func(body string) *protobufs.AgentConfigMap {
		return configMap(map[string]string{"": body})
	}

	t.Run("applies both settings", func(t *testing.T) {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

		debug := new(k6runner.HTTPDebug)
		a := &Agent{logger: zerolog.Nop(), defaultLevel: zerolog.InfoLevel, k6HTTPDebug: debug}

		require.NoError(t, a.applyRemoteConfig(cfg("synthetic_monitoring:\n  log_level: debug\n  k6_http_debug: headers\n")))
		require.Equal(t, k6runner.HTTPDebugHeaders, debug.Get())
		require.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
		require.Equal(t, "headers", attributes(a.description())["sm.agent.k6_http_debug"])

		// Removing the setting turns it off again.
		require.NoError(t, a.applyRemoteConfig(cfg("")))
		require.Equal(t, k6runner.HTTPDebugOff, debug.Get())
		require.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
	})

	t.Run("invalid value changes nothing", func(t *testing.T) {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

		debug := new(k6runner.HTTPDebug)
		a := &Agent{logger: zerolog.Nop(), defaultLevel: zerolog.InfoLevel, k6HTTPDebug: debug}

		require.Error(t, a.applyRemoteConfig(cfg("synthetic_monitoring:\n  log_level: debug\n  k6_http_debug: loud\n")))
		require.Equal(t, k6runner.HTTPDebugOff, debug.Get())
		require.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
	})

	t.Run("rejected without k6", func(t *testing.T) {
		a := &Agent{logger: zerolog.Nop(), defaultLevel: zerolog.InfoLevel}

		require.Error(t, a.applyRemoteConfig(cfg("synthetic_monitoring:\n  k6_http_debug: full\n")))
		require.NoError(t, a.applyRemoteConfig(cfg("synthetic_monitoring:\n  k6_http_debug: \"off\"\n")))
		require.NotContains(t, attributes(a.description()), "sm.agent.k6_http_debug")
	})
}
