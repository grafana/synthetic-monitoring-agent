package tls

import (
	"context"
	"fmt"
	"os"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	promconfig "github.com/prometheus/common/config"
	"github.com/rs/zerolog"
)

func SMtoProm(ctx context.Context, logger zerolog.Logger, tlsConfig *sm.TLSConfig) (promconfig.TLSConfig, error) {
	c := promconfig.TLSConfig{
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ServerName:         tlsConfig.ServerName,
	}

	if len(tlsConfig.CACert) > 0 {
		fn, err := newDataProvider(ctx, logger, "ca_cert", tlsConfig.CACert)
		if err != nil {
			return promconfig.TLSConfig{}, err
		}
		c.CAFile = fn
	}

	if len(tlsConfig.ClientCert) > 0 {
		fn, err := newDataProvider(ctx, logger, "client_cert", tlsConfig.ClientCert)
		if err != nil {
			return promconfig.TLSConfig{}, err
		}
		c.CertFile = fn
	}

	if len(tlsConfig.ClientKey) > 0 {
		fn, err := newDataProvider(ctx, logger, "client_key", tlsConfig.ClientKey)
		if err != nil {
			return promconfig.TLSConfig{}, err
		}
		c.KeyFile = fn
	}

	return c, nil
}

// newDataProvider creates a filesystem object that provides the
// specified data as often as needed. It returns the name under which
// the data can be accessed.
//
// It does NOT try to make guarantees about partial reads. If the reader
// goes away before reaching the end of the data, the next time the
// reader shows up, the writer might continue from the previous
// prosition.
func newDataProvider(ctx context.Context, logger zerolog.Logger, basename string, data []byte) (string, error) {
	fh, err := os.CreateTemp("", basename+".")
	if err != nil {
		logger.Error().Err(err).Str("basename", basename).Msg("creating temporary file")
		return "", fmt.Errorf("creating temporary file: %w", err)
	}
	defer func() {
		if err := fh.Close(); err != nil {
			// close errors should never happen, but if they
			// do, the most we can do is log them to be able
			// to debug the issue.
			logger.Error().Err(err).Str("filename", fh.Name()).Msg("closing temporary file")
		}
	}()

	fn := fh.Name()

	if n, err := fh.Write(data); err != nil {
		logger.Error().Err(err).Str("filename", fn).Int("bytes", n).Int("data", len(data)).Msg("writing temporary file")
		return "", fmt.Errorf("writing temporary file for %s: %w", basename, err)
	}

	// play nice and make sure this file gets deleted once the
	// context is cancelled, which could be when the program is
	// shutting down or when the scraper stops.
	go func() {
		<-ctx.Done()
		if err := os.Remove(fn); err != nil {
			logger.Error().Err(err).Str("filename", fn).Msg("removing temporary file")
		}
	}()

	return fn, nil
}
