package interpolation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/stretchr/testify/require"
)

func TestResolverResolve(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	secretProvider := testhelper.NewMockSecretProvider(map[string]string{
		"api-token":       "secret-token-123",
		"db-password":     "secret-password",
		"empty-secret":    "",
		"auth-config":     "username=${username}&token=${api-token}",
		"random-password": "my-password-${random}",
	})

	testcases := map[string]struct {
		input          string
		expectedOutput string
		expectError    bool
	}{
		"empty string": {
			input:          "",
			expectedOutput: "",
			expectError:    false,
		},
		"plaintext only": {
			input:          "hello world",
			expectedOutput: "hello world",
			expectError:    false,
		},
		"secret interpolation only": {
			input:          "${secrets.api-token}",
			expectedOutput: "secret-token-123",
			expectError:    false,
		},
		"multiple secrets": {
			input:          "${secrets.api-token}:${secrets.db-password}",
			expectedOutput: "secret-token-123:secret-password",
			expectError:    false,
		},
		"empty secret resolves to empty string": {
			input:          "token=${secrets.empty-secret}",
			expectedOutput: "token=",
			expectError:    false,
		},
		"empty secret name": {
			input:          "${secrets.}",
			expectedOutput: "",
			expectError:    true,
		},
		"secret name that fails validation": {
			input:          "${secrets.Invalid_Name}",
			expectedOutput: "",
			expectError:    true,
		},
		"missing secret": {
			input:          "${secrets.missing-secret}",
			expectedOutput: "",
			expectError:    true,
		},

		// Only ${secrets.name} is resolved. A bare ${name} is a multihttp variable and stays as is.
		"bare variable is not expanded": {
			input:          "${username}",
			expectedOutput: "${username}",
			expectError:    false,
		},
		"bare hyphenated variable is not expanded": {
			input:          "${some-variable}",
			expectedOutput: "${some-variable}",
			expectError:    false,
		},
		"secret alongside bare variables": {
			input:          "Bearer ${secrets.api-token} for ${username}@${domain}",
			expectedOutput: "Bearer secret-token-123 for ${username}@${domain}",
			expectError:    false,
		},

		// A secret value is not scanned again, so references inside it stay as text.
		"secret value containing variables": {
			input:          "https://api.example.com/auth?${secrets.auth-config}",
			expectedOutput: "https://api.example.com/auth?username=${username}&token=${api-token}",
			expectError:    false,
		},
		"secret value containing a variable-like pattern": {
			input:          "Password: ${secrets.random-password}",
			expectedOutput: "Password: my-password-${random}",
			expectError:    false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			resolver := NewResolver(secretProvider, tenantID, logger)

			actual, err := resolver.resolve(ctx, tc.input, nil)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, actual)
			}
		})
	}
}

func TestResolverResolveFunc(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	secretProvider := testhelper.NewMockSecretProvider(map[string]string{"price": "$5"})
	double := func(v string) (string, error) { return strings.ReplaceAll(v, "$", "$$"), nil }

	// transform changes the secret value only, in a bare reference and among other text, and the
	// redactor keeps the value as stored.
	for input, want := range map[string]string{
		"${secrets.price}":       "$$5",
		"$1 ${secrets.price} $2": "$1 $$5 $2",
	} {
		resolver := NewResolver(secretProvider, tenantID, logger)

		actual, err := resolver.ResolveFunc(ctx, "sent text", input, double)
		require.NoError(t, err)
		require.Equal(t, want, actual)
		require.Equal(t, "${secrets.price}", resolver.Redactor()("$5"))
	}
}

// TestResolverErrors checks that a failure comes back as one flat secrets.Error: resolveSecret adds
// the secret, ResolveFunc adds the field, and the cause is kept once, not wrapped again.
func TestResolverErrors(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	storeErr := errors.New("connection refused")
	secretProvider := testhelper.NewMockSecretProviderWithFunc(func(_ context.Context, _ model.GlobalID, name string) (string, error) {
		if name == "missing" {
			return "", storeErr
		}

		return "value", nil
	})

	reject := func(string) (string, error) {
		return "", &secrets.Error{Reason: secrets.ErrSecretValueInvalidInSentText}
	}

	testcases := map[string]struct {
		value     string
		transform func(string) (string, error)
		want      *secrets.Error
	}{
		"store error": {
			value: "${secrets.missing}",
			want:  &secrets.Error{Field: "sent text", Secret: "missing", Reason: secrets.ErrSecretFetchFailed, Err: storeErr},
		},
		"invalid name": {
			value: "${secrets.}",
			want:  &secrets.Error{Field: "sent text", Reason: ErrInvalidSecretName, Err: errors.New(`name ""`)},
		},
		"transform error": {
			value:     "AUTH ${secrets.ok}",
			transform: reject,
			want:      &secrets.Error{Field: "sent text", Secret: "ok", Reason: secrets.ErrSecretValueInvalidInSentText},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			_, err := NewResolver(secretProvider, tenantID, logger).ResolveFunc(ctx, "sent text", tc.value, tc.transform)
			require.Equal(t, tc.want, err)
		})
	}
}

func TestResolverRedactor(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	secretProvider := testhelper.NewMockSecretProvider(map[string]string{
		"short":        "token",
		"long":         "token-with-suffix",
		"empty-secret": "",
	})

	t.Run("nothing resolved", func(t *testing.T) {
		resolver := NewResolver(secretProvider, tenantID, logger)

		_, err := resolver.resolve(ctx, "plain text", nil)
		require.NoError(t, err)

		require.Nil(t, resolver.Redactor())
	})

	t.Run("an empty value is never replaced", func(t *testing.T) {
		resolver := NewResolver(secretProvider, tenantID, logger)

		_, err := resolver.resolve(ctx, "${secrets.empty-secret}", nil)
		require.NoError(t, err)

		require.Nil(t, resolver.Redactor())
	})

	t.Run("each value becomes its reference", func(t *testing.T) {
		resolver := NewResolver(secretProvider, tenantID, logger)

		for _, value := range []string{"${secrets.short}", "a ${secrets.long} b"} {
			_, err := resolver.resolve(ctx, value, nil)
			require.NoError(t, err)
		}

		redact := resolver.Redactor()
		require.NotNil(t, redact)

		// The longer value holds the shorter one, and is replaced as a whole.
		require.Equal(t,
			"host=${secrets.long} key=${secrets.short} other=text",
			redact("host=token-with-suffix key=token other=text"),
		)
	})
}

func TestIsValidSecretName(t *testing.T) {
	testcases := map[string]struct {
		name     string
		expected bool
	}{
		"valid simple name": {
			name:     "my-secret",
			expected: true,
		},
		"valid with dots": {
			name:     "my.secret.name",
			expected: true,
		},
		"valid with numbers": {
			name:     "secret123",
			expected: true,
		},
		"valid mixed case": {
			name:     "my-secret-name",
			expected: true,
		},
		"empty name": {
			name:     "",
			expected: false,
		},
		"too long": {
			name:     "a" + string(make([]byte, 253)),
			expected: false,
		},
		"starts with dash": {
			name:     "-invalid",
			expected: false,
		},
		"ends with dash": {
			name:     "invalid-",
			expected: false,
		},
		"contains uppercase": {
			name:     "Invalid-Name",
			expected: false,
		},
		"contains special chars": {
			name:     "invalid@name",
			expected: false,
		},
		"starts with dot": {
			name:     ".invalid",
			expected: false,
		},
		"ends with dot": {
			name:     "invalid.",
			expected: false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := isValidSecretName(tc.name)
			require.Equal(t, tc.expected, actual)
		})
	}
}
