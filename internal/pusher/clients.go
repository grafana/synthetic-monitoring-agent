package pusher

import (
	"fmt"
	"net/url"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
	"github.com/grafana/synthetic-monitoring-agent/internal/version"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

func ClientFromRemoteInfo(remote *sm.RemoteInfo) (*prom.ClientConfig, error) {
	// TODO(mem): this is hacky.
	//
	// it's trying to deal with the fact that the URL shown to users
	// is not the push URL but the base for the API endpoints
	u, err := url.Parse(remote.Url + "/push")
	if err != nil {
		// XXX(mem): do we really return an error here?
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	clientCfg := prom.ClientConfig{
		URL:       u,
		Timeout:   5 * time.Second,
		UserAgent: version.UserAgent(),
	}

	if remote.Username != "" {
		clientCfg.HTTPClientConfig.BasicAuth = &prom.BasicAuth{
			Username: remote.Username,
			Password: remote.Password,
		}
	}

	if clientCfg.Headers == nil {
		clientCfg.Headers = make(map[string]string)
	}

	clientCfg.Headers["X-Prometheus-Remote-Write-Version"] = "0.1.0"
	return &clientCfg, nil
}
