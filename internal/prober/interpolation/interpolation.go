package interpolation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/rs/zerolog"
)

// VariableRegex matches ${variable_name} patterns
var VariableRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// SecretRegex matches ${secrets.secret_name} patterns
// Secret names must start with alphanumeric or underscore and can contain alphanumeric, underscore, dot, and dash
var SecretRegex = regexp.MustCompile(`\$\{secrets\.([a-zA-Z0-9_][a-zA-Z0-9_\.\-]*)\}`)

// VariableProvider defines the interface for resolving variables
type VariableProvider interface {
	GetVariable(name string) (string, error)
}

// SecretProvider defines the interface for resolving secrets
type SecretProvider interface {
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

// Resolver handles string interpolation for both variables and secrets
type Resolver struct {
	variableProvider VariableProvider
	secretProvider   SecretProvider
	tenantID         model.GlobalID
	logger           zerolog.Logger
	secretEnabled    bool
}

// NewResolver creates a new interpolation resolver
func NewResolver(variableProvider VariableProvider, secretProvider SecretProvider, tenantID model.GlobalID, logger zerolog.Logger, secretEnabled bool) *Resolver {
	return &Resolver{
		variableProvider: variableProvider,
		secretProvider:   secretProvider,
		tenantID:         tenantID,
		logger:           logger,
		secretEnabled:    secretEnabled,
	}
}

// Resolve performs string interpolation, replacing both variables and secrets
func (r *Resolver) Resolve(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "", nil
	}

	// First resolve secrets if enabled
	if r.secretEnabled {
		resolvedValue, err := r.resolveSecrets(ctx, value)
		if err != nil {
			return "", err
		}
		value = resolvedValue
	}

	// Then resolve variables
	if r.variableProvider != nil {
		resolvedValue, err := r.resolveVariables(value)
		if err != nil {
			return "", err
		}
		value = resolvedValue
	}

	return value, nil
}

// resolveSecrets resolves ${secrets.secret_name} patterns
func (r *Resolver) resolveSecrets(ctx context.Context, value string) (string, error) {
	matches := SecretRegex.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return value, nil
	}

	result := value
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		secretName := match[1]
		placeholder := match[0] // ${secrets.secret_name}

		// Validate secret name follows Kubernetes DNS subdomain convention
		if !isValidSecretName(secretName) {
			return "", fmt.Errorf("invalid secret name '%s': must follow Kubernetes DNS subdomain naming convention", secretName)
		}

		r.logger.Debug().Str("secretName", secretName).Int64("tenantId", int64(r.tenantID)).Msg("resolving secret from GSM")

		secretValue, err := r.secretProvider.GetSecretValue(ctx, r.tenantID, secretName)
		if err != nil {
			return "", fmt.Errorf("failed to get secret '%s' from GSM: %w", secretName, err)
		}

		// Replace the placeholder with the actual secret value
		result = strings.ReplaceAll(result, placeholder, secretValue)
	}

	return result, nil
}

// resolveVariables resolves ${variable_name} patterns
func (r *Resolver) resolveVariables(value string) (string, error) {
	// If no variable provider is set, return the value as-is
	if r.variableProvider == nil {
		return value, nil
	}

	matches := VariableRegex.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return value, nil
	}

	result := value
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		varName := match[1]
		placeholder := match[0] // ${variable_name}

		varValue, err := r.variableProvider.GetVariable(varName)
		if err != nil {
			return "", fmt.Errorf("failed to get variable '%s': %w", varName, err)
		}

		// Replace the placeholder with the actual variable value
		result = strings.ReplaceAll(result, placeholder, varValue)
	}

	return result, nil
}

// isValidSecretName validates that a secret name follows Kubernetes DNS subdomain naming convention.
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
func isValidSecretName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}

	// Must consist of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character
	if !regexp.MustCompile(`^[a-z0-9]([a-z0-9\-\.]*[a-z0-9])?$`).MatchString(name) {
		return false
	}

	return true
}

// ToJavaScript converts a string with variable interpolation to JavaScript code
// This is used by multihttp to generate JavaScript that references variables
func ToJavaScript(value string) string {
	if len(value) == 0 {
		return `''`
	}

	var s strings.Builder
	buf := []byte(value)
	locs := VariableRegex.FindAllSubmatchIndex(buf, -1)

	p := 0
	for _, loc := range locs {
		if len(loc) < 4 { // put the bounds checker at ease
			panic("unexpected result while building JavaScript")
		}

		if s.Len() > 0 {
			s.WriteRune('+')
		}

		if pre := buf[p:loc[0]]; len(pre) > 0 {
			s.WriteRune('\'')
			escapeJavaScript(&s, pre)
			s.WriteRune('\'')
			s.WriteRune('+')
		}

		s.WriteString(`vars['`)
		// Because of the capture in the regular expression, the result
		// has two indices that represent the matched substring, and
		// two more indices that represent the capture group.
		s.Write(buf[loc[2]:loc[3]])
		s.WriteString(`']`)

		p = loc[1]
	}

	if len(buf[p:]) > 0 {
		if s.Len() > 0 {
			s.WriteRune('+')
		}

		s.WriteRune('\'')
		escapeJavaScript(&s, buf[p:])
		s.WriteRune('\'')
	}

	return s.String()
}

// ToBodyVariableReplacements generates JavaScript code for replacing variables in body content
// This is used by multihttp to generate JavaScript that replaces variables in request bodies
func ToBodyVariableReplacements(bodyVarName string, bodyContent []byte) []string {
	if len(bodyContent) == 0 {
		return nil
	}

	var buf strings.Builder
	parsedMatches := make(map[string]struct{})
	out := make([]string, 0)

	// Find regular variables
	regularMatches := VariableRegex.FindAllSubmatchIndex(bodyContent, -1)
	for _, match := range regularMatches {
		if len(match) < 4 {
			continue
		}
		m := string(bodyContent[match[0]:match[1]])
		if _, found := parsedMatches[m]; found {
			continue
		}

		buf.Reset()
		buf.WriteString(bodyVarName)
		buf.WriteString("=")
		buf.WriteString(bodyVarName)
		buf.WriteString(".replaceAll('")
		buf.WriteString(m)
		buf.WriteString("', vars['")
		buf.Write(bodyContent[match[2]:match[3]])
		buf.WriteString("'])")
		out = append(out, buf.String())

		parsedMatches[m] = struct{}{}
	}

	// Find secret variables
	secretMatches := SecretRegex.FindAllSubmatchIndex(bodyContent, -1)
	for _, match := range secretMatches {
		if len(match) < 4 {
			continue
		}
		m := string(bodyContent[match[0]:match[1]])
		if _, found := parsedMatches[m]; found {
			continue
		}

		buf.Reset()
		buf.WriteString(bodyVarName)
		buf.WriteString("=")
		buf.WriteString(bodyVarName)
		buf.WriteString(".replaceAll('")
		buf.WriteString(m)
		buf.WriteString("', await secrets.get('")
		buf.Write(bodyContent[match[2]:match[3]])
		buf.WriteString("'))")
		out = append(out, buf.String())

		parsedMatches[m] = struct{}{}
	}

	return out
}

// ToJavaScriptWithSecrets converts a string with both variable and secret interpolation to JavaScript code
// This is used by multihttp to generate JavaScript that references both variables and secrets
func ToJavaScriptWithSecrets(value string) string {
	if len(value) == 0 {
		return `''`
	}

	var s strings.Builder
	buf := []byte(value)

	// First handle secret variables
	locs := SecretRegex.FindAllSubmatchIndex(buf, -1)
	p := 0

	for _, loc := range locs {
		if len(loc) < 4 {
			panic("unexpected result while building JavaScript")
		}

		if s.Len() > 0 {
			s.WriteRune('+')
		}

		if pre := buf[p:loc[0]]; len(pre) > 0 {
			s.WriteRune('\'')
			escapeJavaScript(&s, pre)
			s.WriteRune('\'')
			s.WriteRune('+')
		}

		// Generate async secret lookup
		s.WriteString(`await secrets.get('`)
		s.Write(buf[loc[2]:loc[3]])
		s.WriteString(`')`)

		p = loc[1]
	}

	// Then handle regular variables in the remaining text
	remainingText := buf[p:]
	if len(remainingText) > 0 {
		regularLocs := VariableRegex.FindAllSubmatchIndex(remainingText, -1)

		if len(regularLocs) > 0 {
			if s.Len() > 0 {
				s.WriteRune('+')
			}

			p2 := 0
			for _, loc := range regularLocs {
				if len(loc) < 4 {
					panic("unexpected result while building JavaScript")
				}

				if s.Len() > 0 {
					s.WriteRune('+')
				}

				if pre := remainingText[p2:loc[0]]; len(pre) > 0 {
					s.WriteRune('\'')
					escapeJavaScript(&s, pre)
					s.WriteRune('\'')
					s.WriteRune('+')
				}

				s.WriteString(`vars['`)
				s.Write(remainingText[loc[2]:loc[3]])
				s.WriteString(`']`)

				p2 = loc[1]
			}

			if len(remainingText[p2:]) > 0 {
				if s.Len() > 0 {
					s.WriteRune('+')
				}
				s.WriteRune('\'')
				escapeJavaScript(&s, remainingText[p2:])
				s.WriteRune('\'')
			}
		} else {
			// No regular variables, just append the remaining text
			if s.Len() > 0 {
				s.WriteRune('+')
			}
			s.WriteRune('\'')
			escapeJavaScript(&s, remainingText)
			s.WriteRune('\'')
		}
	}

	return s.String()
}

// escapeJavaScript escapes a byte slice for use in JavaScript strings
func escapeJavaScript(s *strings.Builder, buf []byte) {
	template.JSEscape(s, buf)
}
