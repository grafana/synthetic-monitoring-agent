package v2

import (
	encoding_binary "encoding/binary"
	"io"
)

// SnappyConcatReader is an io.Reader that takes a list of snappy-compressed buffers (Streams)
// and returns a single valid snappy-compressed stream equivalent to the concatenation of those buffers
// without additional allocation.
type SnappyConcatReader struct {
	Streams [][]byte
	init    bool
}

// Read fulfills the io.Reader interface.
func (s *SnappyConcatReader) Read(out []byte) (n int, err error) {
	if !s.init {
		s.init = true
		decodedLen := s.stripHeaders()
		// Check that we have room to write the uvarint
		if fit := len(out); fit < encoding_binary.MaxVarintLen64 {
			if maxValue := uint64(1) << (7 * fit); decodedLen >= maxValue {
				return 0, io.ErrShortBuffer
			}
		}
		n = encoding_binary.PutUvarint(out, decodedLen)
		out = out[n:]
	}
	if n == 0 && len(s.Streams) == 0 {
		return 0, io.EOF
	}

	// Copy all the streams that fit
	for len(s.Streams) > 0 && len(out) >= len(s.Streams[0]) {
		chunkLen := len(s.Streams[0])
		copy(out, s.Streams[0])
		s.Streams = s.Streams[1:]
		n += chunkLen
		out = out[chunkLen:]
	}

	// Copy a partial stream if there is room
	if fit := len(out); fit > 0 && len(s.Streams) > 0 {
		// Here s.Streams[0] is always larger than what's left in out.
		// Otherwise it would've been copied by the previous loop.
		copy(out, s.Streams[0][:fit])
		s.Streams[0] = s.Streams[0][fit:]
		n += fit
	}
	return n, nil
}

func (s *SnappyConcatReader) stripHeaders() uint64 {
	var totalLength uint64
	for idx := range s.Streams {
		streamLen, skip := encoding_binary.Uvarint(s.Streams[idx])
		totalLength += streamLen
		s.Streams[idx] = s.Streams[idx][skip:]
	}
	return totalLength
}
