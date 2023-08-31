package prom_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/pkg/prom"
)

type FailNTimesPrometheusClient struct {
	CountedRetries float64
	FailuresLeft   int64
}

func (f *FailNTimesPrometheusClient) StoreBytes(ctx context.Context, req []byte) error {
	return f.StoreStream(ctx, bytes.NewReader(req))
}

func (f *FailNTimesPrometheusClient) StoreStream(ctx context.Context, req io.Reader) error {
	if f.FailuresLeft == 0 {
		return nil
	}
	f.FailuresLeft--
	return prom.NewRecoverableError(errors.New("client failed"))
}

func (f *FailNTimesPrometheusClient) CountRetries(retries float64) {
	f.CountedRetries = retries
}

type FakePrometheusClient struct {
	storeFunc        func(ctx context.Context, req []byte) error
	countRetriesFunc func(float642 float64)
}

func (f *FakePrometheusClient) Store(ctx context.Context, req []byte) error {
	return f.storeFunc(ctx, req)
}

func (f *FakePrometheusClient) CountRetries(retries float64) {
	f.countRetriesFunc(retries)
}

func TestSendBytesWithBackoffRetriesCounter(t *testing.T) {
	type args struct {
		ctx         context.Context
		timesToFail int64
		req         []byte
	}
	tests := []struct {
		name                 string
		args                 args
		wantErr              bool
		expectedRetriesCount float64
		isSlow               bool
	}{
		{
			name: "should count 0 retries when successful at first",
			args: args{
				ctx:         context.Background(),
				timesToFail: 0,
				req:         []byte{},
			},
			expectedRetriesCount: 0,
		},
		{
			name: "should count 2 retries when failed 2 times",
			args: args{
				ctx:         context.Background(),
				timesToFail: 2,
				req:         []byte{},
			},
			expectedRetriesCount: 2,
		},
		{
			name: "should count 10 retries when retries exceeded",
			args: args{
				ctx:         context.Background(),
				timesToFail: 10,
				req:         []byte{},
			},
			wantErr:              true,
			expectedRetriesCount: 10,
			isSlow:               true,
		},
	}
	for _, tt := range tests {
		client := &FailNTimesPrometheusClient{
			CountedRetries: -1,
			FailuresLeft:   tt.args.timesToFail,
		}
		t.Run(tt.name, func(t *testing.T) {
			if testing.Short() && tt.isSlow {
				t.Skip("skipping in short mode")
			}

			if err := prom.SendBytesWithBackoff(tt.args.ctx, client, tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("SendBytesWithBackoff() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if client.CountedRetries != tt.expectedRetriesCount {
				t.Errorf("Not expected retries count registered. Expected: %f, Got: %f", tt.expectedRetriesCount, client.CountedRetries)
			}
		})
	}
}
