package interpolation

import (
	"fmt"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	"github.com/stretchr/testify/require"
)

// mockVariableProvider is a mock implementation of VariableProvider for testing
type mockVariableProvider struct {
	variables map[string]string
}

func (m *mockVariableProvider) GetVariable(name string) (string, error) {
	if value, exists := m.variables[name]; exists {
		return value, nil
	}
	return "", fmt.Errorf("variable '%s' not found", name)
}

func TestResolver_Resolve(t *testing.T) {
	ctx, logger, tenantID := testhelper.CommonTestSetup()

	// Mock providers
	variableProvider := &mockVariableProvider{
		variables: map[string]string{
			"username":             "admin",
			"domain":               "example.com",
			"random":               "123",
			"variable-with-secret": "${secrets.api-token}",
		},
	}

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
		"variable interpolation only": {
			input:          "${username}",
			secretEnabled:  true,
			expectedOutput: "admin",
			expectError:    false,
		},
		"mixed secret and variable": {
			input:          "Bearer ${secrets.api-token} for ${username}@${domain}",
			secretEnabled:  true,
			expectedOutput: "Bearer secret-token-123 for admin@example.com",
			expectError:    false,
		},
		"multiple secrets": {
			input:          "${secrets.api-token}:${secrets.db-password}",
			secretEnabled:  true,
			expectedOutput: "secret-token-123:secret-password",
			expectError:    false,
		},
		"secrets disabled": {
			input:          "${secrets.api-token}",
			secretEnabled:  false,
			expectedOutput: "${secrets.api-token}",
			expectError:    false,
		},
		"variables still work when secrets disabled": {
			input:          "${username}",
			secretEnabled:  false,
			expectedOutput: "admin",
			expectError:    false,
		},
		"empty secret name": {
			input:          "${secrets.}",
			secretEnabled:  true,
			expectedOutput: "",
			expectError:    true,
		},
		"invalid secret name": {
			input:          "${secrets.invalid-name}",
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
		"missing variable": {
			input:          "${missing-variable}",
			secretEnabled:  true,
			expectedOutput: "${missing-variable}",
			expectError:    false,
		},
		"missing variable when secrets disabled": {
			input:          "${missing-variable}",
			secretEnabled:  false,
			expectedOutput: "${missing-variable}",
			expectError:    false,
		},
		"variable with no provider": {
			input:          "${some-variable}",
			secretEnabled:  true,
			expectedOutput: "${some-variable}",
			expectError:    false,
		},
		"secret containing variables": {
			input:          "https://api.example.com/auth?${secrets.auth-config}",
			secretEnabled:  true,
			expectedOutput: "https://api.example.com/auth?username=${username}&token=${api-token}",
			expectError:    false,
		},
		"secret with variable-like pattern": {
			input:          "Password: ${secrets.random-password}",
			secretEnabled:  true,
			expectedOutput: "Password: my-password-${random}",
			expectError:    false,
		},
		"variable with secret": {
			input:          "${variable-with-secret}",
			secretEnabled:  true,
			expectedOutput: "${secrets.api-token}",
			expectError:    false,
		},
		"variables disabled": {
			input:          "Hello ${username} with token ${secrets.api-token}",
			secretEnabled:  true,
			expectedOutput: "Hello ${username} with token secret-token-123",
			expectError:    false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var resolver *Resolver
			if name == "variables disabled" {
				// Create resolver with no variable provider to test variables disabled
				resolver = NewResolver(nil, secretProvider, tenantID, logger, tc.secretEnabled)
			} else {
				resolver = NewResolver(variableProvider, secretProvider, tenantID, logger, tc.secretEnabled)
			}

			actual, err := resolver.Resolve(ctx, tc.input)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, actual)
			}
		})
	}

	// Test with no variable provider
	t.Run("no variable provider", func(t *testing.T) {
		resolver := NewResolver(nil, secretProvider, tenantID, logger, true)
		actual, err := resolver.Resolve(ctx, "${some-variable}")
		require.NoError(t, err)
		require.Equal(t, "${some-variable}", actual)
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

func TestToJavaScript(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected string
	}{
		"empty string": {
			input:    "",
			expected: "''",
		},
		"plaintext only": {
			input:    "hello world",
			expected: "'hello world'",
		},
		"variable only": {
			input:    "${username}",
			expected: "vars['username']",
		},
		"mixed text and variable": {
			input:    "Hello ${username}!",
			expected: "'Hello '+vars['username']+'!'",
		},
		"multiple variables": {
			input:    "${username}@${domain}",
			expected: "vars['username']+'@'+vars['domain']",
		},
		"complex mixed": {
			input:    "Bearer ${token} for ${username}@${domain}",
			expected: "'Bearer '+vars['token']+' for '+vars['username']+'@'+vars['domain']",
		},
		"with quotes": {
			input:    "Hello \"${username}\"",
			expected: "'Hello \\\"'+vars['username']+'\\\"'",
		},
		"with backslashes": {
			input:    "Path: \\${username}\\",
			expected: "'Path: \\\\'+vars['username']+'\\\\'",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := ToJavaScript(tc.input)
			require.Equal(t, tc.expected, actual)
		})
	}
}
