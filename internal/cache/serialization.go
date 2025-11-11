package cache

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

// encode serializes a Go value to bytes using gob encoding.
// The value must be a gob-encodable type (no unexported fields in structs,
// no channels, functions, etc.).
func encode(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	if err := enc.Encode(value); err != nil {
		return nil, fmt.Errorf("failed to encode value: %w", err)
	}

	return buf.Bytes(), nil
}

// decode deserializes bytes to a Go value using gob decoding.
// The dest parameter must be a pointer to the target type.
func decode(data []byte, dest any) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)

	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("failed to decode value: %w", err)
	}

	return nil
}
