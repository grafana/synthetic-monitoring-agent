package v2

import (
	"crypto/rand"
	"io"
	"testing"

	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"
)

func TestSnappyConcatReader(t *testing.T) {
	const bigRandSize = 4096 * 4
	var bigRand [bigRandSize]byte
	_, err := rand.Read(bigRand[:])
	require.NoError(t, err)

	for title, tc := range map[string]struct {
		input    [][]byte
		expected []byte
		readFn   func(io.Reader) ([]byte, error)
	}{
		"sample": {
			input: [][]byte{
				snap("HELLO"),
				snap(" WORL"),
				snap("D!"),
			},
			expected: []byte("HELLO WORLD!"),
		},
		"single stream": {
			input: [][]byte{
				snap("Hello World!"),
			},
			expected: []byte("Hello World!"),
		},
		"sample small buf": {
			input: [][]byte{
				snap("HELLO WORLD 1\n"),
				snap("HELLO WORLD 2!\n"),
				snap("HELLO WORLD 3 AND LAST.\n"),
			},
			readFn:   readerWithBufSize(4),
			expected: []byte("HELLO WORLD 1\nHELLO WORLD 2!\nHELLO WORLD 3 AND LAST.\n"),
		},
		"sample tiny buf": {
			input: [][]byte{
				snap("HELLO WORLD 1\n"),
				snap("HELLO WORLD 2!\n"),
				snap("HELLO WORLD 3 AND LAST.\n"),
			},
			readFn:   readerWithBufSize(1),
			expected: []byte("HELLO WORLD 1\nHELLO WORLD 2!\nHELLO WORLD 3 AND LAST.\n"),
		},
		"sample mid buf": {
			input: [][]byte{
				snap("HELLO WORLD 1\n"),
				snap("HELLO WORLD 2!\n"),
				snap("HELLO WORLD 3 AND LAST.\n"),
			},
			readFn:   readerWithBufSize(16),
			expected: []byte("HELLO WORLD 1\nHELLO WORLD 2!\nHELLO WORLD 3 AND LAST.\n"),
		},
		"very large": {
			input: [][]byte{
				snap(string(bigRand[:bigRandSize/4])),
				snap(string(bigRand[bigRandSize/4 : bigRandSize/2])),
				snap(string(bigRand[bigRandSize/2 : 3*bigRandSize/4])),
				snap(string(bigRand[3*bigRandSize/4:])),
			},
			expected: bigRand[:],
		},
		"empty": {},
	} {
		t.Run(title, func(t *testing.T) {
			r := &SnappyConcatReader{
				Streams: tc.input,
			}
			if tc.readFn == nil {
				tc.readFn = io.ReadAll
			}
			all, err := tc.readFn(r)
			require.NoError(t, err)

			result, err := snappy.Decode(nil, all)
			require.NoError(t, err)

			require.Equal(t, tc.expected, result)
		})
	}
}

func snap(data string) []byte {
	return snappy.Encode(nil, []byte(data))
}

func readerWithBufSize(n int) func(io.Reader) ([]byte, error) {
	buf := make([]byte, n)
	return func(r io.Reader) ([]byte, error) {
		var result []byte
		for {
			n, err := r.Read(buf)
			if err != nil {
				if err == io.EOF {
					return result, nil
				}
				return nil, err
			}
			result = append(result, buf[:n]...)
			if n != len(buf) {
				return result, nil
			}
		}
	}
}
