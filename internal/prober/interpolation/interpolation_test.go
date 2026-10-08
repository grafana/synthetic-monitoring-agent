package interpolation

import (
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/stretchr/testify/require"
)

func TestResolver_Resolve(t *testing.T) {
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
		secretEnabled  bool
		expectedOutput string
		expectError    bool
	}{
		"empty string": {
			input:          "",
			secretEnabled:  true,
			expectedOutput: "",
			expectError:    false,
		},
		"plaintext only": {
			input:          "hello world",
			secretEnabled:  true,
			expectedOutput: "hello world",
			expectError:    false,
		},
		"secret interpolation only": {
			input:          "${secrets.api-token}",
			secretEnabled:  true,
			expectedOutput: "secret-token-123",
			expectError:    false,
		},
		"multiple secrets": {
			input:          "${secrets.api-token}:${secrets.db-password}",
			secretEnabled:  true,
			expectedOutput: "secret-token-123:secret-password",
			expectError:    false,
		},
		"empty secret resolves to empty string": {
			input:          "token=${secrets.empty-secret}",
			secretEnabled:  true,
			expectedOutput: "token=",
			expectError:    false,
		},
		"secrets disabled": {
			input:          "${secrets.api-token}",
			secretEnabled:  false,
			expectedOutput: "${secrets.api-token}",
			expectError:    false,
		},
		"empty secret name": {
			input:          "${secrets.}",
			secretEnabled:  true,
			expectedOutput: "",
			expectError:    true,
		},
		"secret name that fails validation": {
			input:          "${secrets.Invalid_Name}",
			secretEnabled:  true,
			expectedOutput: "",
			expectError:    true,
		},
		"missing secret": {
			input:          "${secrets.missing-secret}",
			secretEnabled:  true,
			expectedOutput: "",
			expectError:    true,
		},

		// This package resolves ${secrets.name} and nothing else. A bare ${name} belongs to
		// multihttp variable expansion, which runs elsewhere, so it has to survive untouched.
		"bare variable is not expanded": {
			input:          "${username}",
			secretEnabled:  true,
			expectedOutput: "${username}",
			expectError:    false,
		},
		"bare hyphenated variable is not expanded": {
			input:          "${some-variable}",
			secretEnabled:  true,
			expectedOutput: "${some-variable}",
			expectError:    false,
		},
		"bare variable is not expanded when secrets disabled": {
			input:          "${username}",
			secretEnabled:  false,
			expectedOutput: "${username}",
			expectError:    false,
		},
		"secret alongside bare variables": {
			input:          "Bearer ${secrets.api-token} for ${username}@${domain}",
			secretEnabled:  true,
			expectedOutput: "Bearer secret-token-123 for ${username}@${domain}",
			expectError:    false,
		},

		// A resolved secret is never rescanned, so a reference inside a secret's value stays
		// literal. Both cases below would change meaning if expansion came back.
		"secret value containing variables": {
			input:          "https://api.example.com/auth?${secrets.auth-config}",
			secretEnabled:  true,
			expectedOutput: "https://api.example.com/auth?username=${username}&token=${api-token}",
			expectError:    false,
		},
		"secret value containing a variable-like pattern": {
			input:          "Password: ${secrets.random-password}",
			secretEnabled:  true,
			expectedOutput: "Password: my-password-${random}",
			expectError:    false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			resolver := NewResolver(secretProvider, tenantID, logger, tc.secretEnabled)

			actual, err := resolver.Resolve(ctx, tc.input)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, actual)
			}
		})
	}
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
