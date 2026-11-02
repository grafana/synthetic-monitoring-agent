package fleet

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	srvtypes "github.com/open-telemetry/opamp-go/server/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func TestOpampEndpoint(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected string
		wantErr  bool
	}{
		"bare":           {input: "https://fm.example.com", expected: "https://fm.example.com/v1/opamp"},
		"trailing slash": {input: "https://fm.example.com/", expected: "https://fm.example.com/v1/opamp"},
		"already set":    {input: "https://fm.example.com/v1/opamp", expected: "https://fm.example.com/v1/opamp"},
		"prefix path":    {input: "http://localhost:8080/fm", expected: "http://localhost:8080/fm/v1/opamp"},
		"bad scheme":     {input: "ws://fm.example.com", wantErr: true},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual, err := opampEndpoint(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestNewValidation(t *testing.T) {
	_, err := New(Opts{InstanceID: "1", Token: "t"})
	require.ErrorIs(t, err, errMissingURL)

	_, err = New(Opts{URL: "https://fm.example.com", Token: "t"})
	require.ErrorIs(t, err, errMissingInstanceID)

	_, err = New(Opts{URL: "https://fm.example.com", InstanceID: "1"})
	require.ErrorIs(t, err, errMissingToken)
}

func TestLoadOrCreateUIDPersists(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateUID(dir)
	require.NoError(t, err)

	second, err := loadOrCreateUID(dir)
	require.NoError(t, err)
	require.Equal(t, first, second)

	ephemeral, err := loadOrCreateUID("")
	require.NoError(t, err)
	require.NotEqual(t, first, ephemeral)
}

func TestDescriptionRespectsAttributeLimit(t *testing.T) {
	a := &Agent{probe: &sm.Probe{Id: 42, Name: "home", Region: "AMER"}}
	for i := range 50 {
		a.probe.Labels = append(a.probe.Labels, sm.Label{Name: "l" + strconv.Itoa(i), Value: "v"})
	}

	d := a.description()
	require.LessOrEqual(t, len(d.IdentifyingAttributes)+len(d.NonIdentifyingAttributes), maxLocalAttr)

	attrs := attributes(d)
	require.Equal(t, serviceName, attrs["service.name"])
	require.Equal(t, "home", attrs["sm.probe.name"])
	require.Equal(t, "42", attrs["sm.probe.id"])
}

// TestAgentReportsToServer runs the agent against an in-process OpAMP server
// and checks that it authenticates, describes itself, and reports health
// changes and probe details.
func TestAgentReportsToServer(t *testing.T) {
	const (
		instanceID = "123456"
		token      = "test-token"
	)

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(instanceID+":"+token))
	msgs := make(chan *protobufs.AgentToServer, 100)

	srv := server.New(nil)
	err := srv.Start(server.StartSettings{
		ListenEndpoint: "127.0.0.1:0",
		Settings: server.Settings{
			Callbacks: srvtypes.Callbacks{
				OnConnecting: func(r *http.Request) srvtypes.ConnectionResponse {
					if r.Header.Get("Authorization") != wantAuth {
						return srvtypes.ConnectionResponse{Accept: false, HTTPStatusCode: http.StatusUnauthorized}
					}

					return srvtypes.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: srvtypes.ConnectionCallbacks{
							OnMessage: func(_ context.Context, _ srvtypes.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
								msgs <- msg
								return &protobufs.ServerToAgent{InstanceUid: msg.InstanceUid}
							},
						},
					}
				},
			},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })

	agent, err := New(Opts{
		Logger:     zerolog.New(zerolog.NewTestWriter(t)),
		URL:        "http://" + srv.Addr().String(),
		InstanceID: instanceID,
		Token:      token,
		StateDir:   t.TempDir(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- agent.Run(ctx) }()

	// The first message carries the description and initial (unhealthy) state.
	first := waitFor(t, msgs, func(m *protobufs.AgentToServer) bool { return m.AgentDescription != nil })
	require.Equal(t, agent.uid[:], first.InstanceUid)
	require.Equal(t, serviceName, attributes(first.AgentDescription)["service.name"])
	require.NotNil(t, first.Health)
	require.False(t, first.Health.Healthy)

	agent.SetConnected(true)
	waitFor(t, msgs, func(m *protobufs.AgentToServer) bool { return m.Health != nil && m.Health.Healthy })

	agent.SetProbe(&sm.Probe{Id: 7, Name: "quickpizza-probe", Region: "AMER"})
	withProbe := waitFor(t, msgs, func(m *protobufs.AgentToServer) bool {
		return m.AgentDescription != nil && attributes(m.AgentDescription)["sm.probe.name"] == "quickpizza-probe"
	})
	require.Equal(t, "AMER", attributes(withProbe.AgentDescription)["sm.probe.region"])

	agent.SetConnected(false)
	waitFor(t, msgs, func(m *protobufs.AgentToServer) bool { return m.Health != nil && !m.Health.Healthy })

	cancel()
	require.NoError(t, <-done)
}

func waitFor(t *testing.T, msgs <-chan *protobufs.AgentToServer, match func(*protobufs.AgentToServer) bool) *protobufs.AgentToServer {
	t.Helper()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case m := <-msgs:
			if match(m) {
				return m
			}
		case <-timeout:
			t.Fatal("timed out waiting for matching message")
			return nil
		}
	}
}

func attributes(d *protobufs.AgentDescription) map[string]string {
	out := make(map[string]string)
	for _, kv := range append(d.IdentifyingAttributes, d.NonIdentifyingAttributes...) {
		out[kv.Key] = kv.Value.GetStringValue()
	}

	return out
}
