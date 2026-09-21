package interpolation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandVariablesToJS(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected string
	}{
		"empty string": {
			input:    "",
			expected: `''`,
		},
		"no replacements": {
			input:    "plain string",
			expected: `'plain string'`,
		},
		"one variable": {
			input:    "this is a ${var} to replace",
			expected: `'this is a '+vars['var']+' to replace'`,
		},
		"two variables": {
			input:    "this is ${v1} and ${v2}",
			expected: `'this is '+vars['v1']+' and '+vars['v2']`,
		},
		"multiple instances": {
			input:    "this is ${v1}, ${v2} and ${v1} again",
			expected: `'this is '+vars['v1']+', '+vars['v2']+' and '+vars['v1']+' again'`,
		},

		// Escaping is the reason this expander is the one that survived. A literal run goes
		// through template.JSEscape, which keeps multi-byte UTF-8 intact and neutralises the
		// characters that could otherwise end the generated string early.
		"multi-byte utf-8 is preserved": {
			input:    "h\u00e9llo ${username}",
			expected: "'h\u00e9llo '+vars['username']",
		},
		"angle brackets are escaped": {
			input:    "</script> ${u}",
			expected: `'\u003C/script\u003E '+vars['u']`,
		},
		"quotes are escaped": {
			input:    `it's a "test" ${v}`,
			expected: `'it\'s a \"test\" '+vars['v']`,
		},

		// The name pattern does not allow a hyphen, so ${my-var} is text and has to stay text.
		"hyphenated name is not a variable": {
			input:    "${my-var} and ${ok_var}",
			expected: `'${my-var} and '+vars['ok_var']`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := ExpandVariablesToJS(testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}

func TestFindVariableRefs(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected []string
	}{
		"empty string": {
			input:    "",
			expected: nil,
		},
		"no references": {
			input:    "plain string",
			expected: nil,
		},
		"references are returned as written, duplicates included": {
			input:    "${a} x ${b} y ${a}",
			expected: []string{"${a}", "${b}", "${a}"},
		},
		"hyphenated name is not a reference": {
			input:    "${bad-name} ${good_name}",
			expected: []string{"${good_name}"},
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := FindVariableRefs(testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}
