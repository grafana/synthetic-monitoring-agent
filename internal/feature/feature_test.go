package feature

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidCollection(t *testing.T) {
	var c Collection
	err := c.Set("flag")
	require.Error(t, err)
}

func TestCollection(t *testing.T) {
	testcases := map[string]struct {
		input              string
		shouldError        bool
		expectedString     string
		expectedCollection Collection
	}{
		"empty": {
			input:              "",
			shouldError:        false,
			expectedString:     "",
			expectedCollection: Collection{},
		},
		"single": {
			input:              "flag",
			shouldError:        false,
			expectedString:     "flag",
			expectedCollection: Collection{"flag": struct{}{}},
		},
		"multiple": {
			input:              "flag1,flag2",
			shouldError:        false,
			expectedString:     "flag1,flag2",
			expectedCollection: Collection{"flag1": struct{}{}, "flag2": struct{}{}},
		},
		"blanks": {
			input:              " flag1 , flag2 ",
			shouldError:        false,
			expectedString:     "flag1,flag2",
			expectedCollection: Collection{"flag1": struct{}{}, "flag2": struct{}{}},
		},
		"empty element": {
			input:              "flag1,,flag2",
			shouldError:        false,
			expectedString:     "flag1,flag2",
			expectedCollection: Collection{"flag1": struct{}{}, "flag2": struct{}{}},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			c := NewCollection()
			err := c.Set(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expectedCollection, c)
			require.Equal(t, tc.expectedString, c.String())

			for flag := range tc.expectedCollection {
				require.True(t, c.IsSet(flag))
			}
		})
	}
}

func TestCollectionFromFlag(t *testing.T) {
	testcases := map[string]struct {
		input            []string
		expectedFeatures []string
	}{
		"single flag single value": {
			input:            []string{"--flag", "foo"},
			expectedFeatures: []string{"foo"},
		},
		"single flag multiple values": {
			input:            []string{"--flag", "foo,bar"},
			expectedFeatures: []string{"foo", "bar"},
		},
		"multiple flags": {
			input:            []string{"--flag", "foo", "--flag", "bar"},
			expectedFeatures: []string{"foo", "bar"},
		},
		"multiple flags multiple values": {
			input:            []string{"--flag", "foo,bar", "--flag", "baz"},
			expectedFeatures: []string{"foo", "bar", "baz"},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			c := NewCollection()
			require.NotNil(t, c)

			fs := flag.NewFlagSet("test flag set", flag.ContinueOnError)
			require.NotNil(t, fs)

			fs.Var(&c, "flag", "test flag")

			err := fs.Parse(tc.input)
			require.NoError(t, err)

			for _, feature := range tc.expectedFeatures {
				require.True(t, c.IsSet(feature))
			}
		})
	}
}
