package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStruct is a test struct for serialization testing
type TestStruct struct {
	ID   int
	Name string
	Tags []string
}

func TestEncodeDecode(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		original := "hello world"
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded string
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("int", func(t *testing.T) {
		original := 42
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded int
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("struct", func(t *testing.T) {
		original := TestStruct{
			ID:   123,
			Name: "test",
			Tags: []string{"a", "b", "c"},
		}
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded TestStruct
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("slice", func(t *testing.T) {
		original := []string{"a", "b", "c"}
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded []string
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("map", func(t *testing.T) {
		original := map[string]int{"a": 1, "b": 2}
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded map[string]int
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("pointer to struct", func(t *testing.T) {
		original := &TestStruct{
			ID:   456,
			Name: "pointer test",
			Tags: []string{"x", "y"},
		}
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded *TestStruct
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})
}

func TestEncodeErrors(t *testing.T) {
	testcases := map[string]struct {
		value any
	}{
		"channel": {
			value: make(chan int),
		},
		"function": {
			value: func() {},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			_, err := encode(tc.value)
			require.Error(t, err)
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	t.Run("invalid data", func(t *testing.T) {
		var dest string
		err := decode([]byte("invalid gob data"), &dest)
		require.Error(t, err)
	})

	t.Run("type mismatch", func(t *testing.T) {
		// Encode a string
		encoded, err := encode("hello")
		require.NoError(t, err)

		// Try to decode as int
		var dest int
		err = decode(encoded, &dest)
		require.Error(t, err)
	})

	t.Run("non-pointer destination", func(t *testing.T) {
		encoded, err := encode("hello")
		require.NoError(t, err)

		// This should fail because dest is not a pointer
		var dest string
		// Note: We need to pass dest directly (not &dest) to test this
		// But gob.Decode requires a pointer, so this will fail
		err = decode(encoded, dest)
		require.Error(t, err)
	})
}

func TestEncodeDecodeNil(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		var original []string
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded []string
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		require.Nil(t, decoded)
	})

	t.Run("nil map", func(t *testing.T) {
		var original map[string]int
		encoded, err := encode(original)
		require.NoError(t, err)
		require.NotEmpty(t, encoded)

		var decoded map[string]int
		err = decode(encoded, &decoded)
		require.NoError(t, err)
		// Note: gob decodes nil maps as empty maps, this is expected behavior
		require.Empty(t, decoded)
	})
}

func TestEncodeDecodeComplexStruct(t *testing.T) {
	type NestedStruct struct {
		Data map[string][]int
		Ptr  *TestStruct
	}

	original := NestedStruct{
		Data: map[string][]int{
			"a": {1, 2, 3},
			"b": {4, 5, 6},
		},
		Ptr: &TestStruct{
			ID:   999,
			Name: "nested",
			Tags: []string{"nested", "test"},
		},
	}

	encoded, err := encode(original)
	require.NoError(t, err)

	var decoded NestedStruct
	err = decode(encoded, &decoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}
