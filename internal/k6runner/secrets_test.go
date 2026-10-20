package k6runner

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	kitlog "github.com/go-kit/log"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type tenantStub struct {
	err error
}

func (s tenantStub) GetTenant(context.Context, *sm.TenantInfo) (*sm.Tenant, error) {
	return nil, s.err
}

// TestSecretStoreRetrieverUnavailable asserts that the check owner receives the reason why
// configuration failed, and that none of the SM API's error text is passed on with it.
func TestSecretStoreRetrieverUnavailable(t *testing.T) {
	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	apiErr := errors.New(`rpc error: code = Unavailable desc = "transport: Error while dialing: dial tcp 10.0.0.1:4031"`)
	provider := secrets.NewSecretProvider(tenantStub{err: apiErr}, time.Minute, zerolog.Nop())

	var agentLog testhelper.LogBuffer

	retrieve := NewSecretStoreRetriever(provider, model.GlobalID(1), zerolog.New(&agentLog))

	var checkLog bytes.Buffer

	_, err := retrieve(ctx, kitlog.NewLogfmtLogger(&checkLog))
	require.Error(t, err)

	require.Contains(t, agentLog.String(), "10.0.0.1:4031")
	require.Contains(t, checkLog.String(), `reason="the secret store is unavailable"`)
	require.NotContains(t, checkLog.String(), "10.0.0.1")
	require.NotContains(t, checkLog.String(), "rpc error")
}
