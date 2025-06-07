package loki

import (
	"testing"

	logproto "github.com/grafana/loki/pkg/push"
	"github.com/stretchr/testify/require"
)

func TestSplitStreamsIntoChunks(t *testing.T) {
	tests := []struct {
		name     string
		streams  []logproto.Stream
		maxBytes int
		want     [][]logproto.Stream
	}{
		{
			name:     "empty streams",
			streams:  []logproto.Stream{},
			maxBytes: 1000,
			want:     nil,
		},
		{
			name: "single stream fits",
			streams: []logproto.Stream{
				{
					Labels:  `{job="test"}`,
					Entries: []logproto.Entry{{Line: "test1"}},
				},
			},
			maxBytes: 1000,
			want: [][]logproto.Stream{
				{
					{
						Labels:  `{job="test"}`,
						Entries: []logproto.Entry{{Line: "test1"}},
					},
				},
			},
		},
		{
			name: "multiple streams split correctly",
			streams: []logproto.Stream{
				{
					Labels:  `{job="test1"}`,
					Entries: []logproto.Entry{{Line: "test1"}},
				},
				{
					Labels:  `{job="test2"}`,
					Entries: []logproto.Entry{{Line: "test2"}},
				},
			},
			maxBytes: 50, // Small enough to force splitting
			want: [][]logproto.Stream{
				{
					{
						Labels:  `{job="test1"}`,
						Entries: []logproto.Entry{{Line: "test1"}},
					},
				},
				{
					{
						Labels:  `{job="test2"}`,
						Entries: []logproto.Entry{{Line: "test2"}},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitStreamsIntoChunks(tt.streams, tt.maxBytes)
			require.Equal(t, tt.want, got)
		})
	}
}
