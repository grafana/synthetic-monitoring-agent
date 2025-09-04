package interpolation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/rs/zerolog"
)

// VariableRegex matches ${variable_name} patterns
var VariableRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_-]*)\}`)

// SecretRegex matches ${secrets.secret_name} patterns
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
	variableEnabled  bool
}

// NewResolver creates a new interpolation resolver
func NewResolver(variableProvider VariableProvider, secretProvider SecretProvider, tenantID model.GlobalID, logger zerolog.Logger, secretEnabled bool) *Resolver {
	return &Resolver{
		variableProvider: variableProvider,
		secretProvider:   secretProvider,
		tenantID:         tenantID,
		logger:           logger,
		secretEnabled:    secretEnabled,
		variableEnabled:  variableProvider != nil,
	}
}

// Resolve performs string interpolation, replacing both variables and secrets in a single pass
func (r *Resolver) Resolve(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "", nil
	}

	// If secrets are disabled, just process variables in the entire string
	if !r.secretEnabled {
		return r.processVariables(value), nil
	}

	// Step 1: Find all secret matches with their positions
	type secretMatch struct {
		start, end  int
		name        string
		placeholder string
	}

	var secretMatches []secretMatch
	matches := SecretRegex.FindAllStringSubmatchIndex(value, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		secretName := value[match[2]:match[3]]
		placeholder := value[match[0]:match[1]]

		// Validate secret name follows Kubernetes DNS subdomain naming convention
		if !isValidSecretName(secretName) {
			return "", fmt.Errorf("invalid secret name '%s': must follow Kubernetes DNS subdomain naming convention", secretName)
		}

		secretMatches = append(secretMatches, secretMatch{
			start:       match[0],
			end:         match[1],
			name:        secretName,
			placeholder: placeholder,
		})
	}

	// Step 2: Split string into parts and process each part
	var result strings.Builder
	lastPos := 0

	for _, secretMatch := range secretMatches {
		// Process the part before this secret (non-secret part)
		nonSecretPart := value[lastPos:secretMatch.start]
		if nonSecretPart != "" {
			processedPart := r.processVariables(nonSecretPart)
			result.WriteString(processedPart)
		}

		// Process the secret part
		r.logger.Debug().Str("secretName", secretMatch.name).Int64("tenantId", int64(r.tenantID)).Msg("resolving secret from GSM")

		secretValue, err := r.secretProvider.GetSecretValue(ctx, r.tenantID, secretMatch.name)
		if err != nil {
			return "", fmt.Errorf("failed to get secret '%s' from GSM: %w", secretMatch.name, err)
		}

		result.WriteString(secretValue)
		lastPos = secretMatch.end
	}

	// Process the remaining part after the last secret (non-secret part)
	remainingPart := value[lastPos:]
	if remainingPart != "" {
		processedPart := r.processVariables(remainingPart)
		result.WriteString(processedPart)
	}

	return result.String(), nil
}

// processVariables resolves ${variable_name} patterns in a string
func (r *Resolver) processVariables(value string) string {
	if !r.variableEnabled {
		return value
	}

	result := value
	variableMatches := VariableRegex.FindAllStringSubmatch(result, -1)
	for _, match := range variableMatches {
		if len(match) < 2 {
			continue
		}

		varName := match[1]
		placeholder := match[0] // ${variable_name}

		varValue, err := r.variableProvider.GetVariable(varName)
		if err != nil {
			// If variable is not found, leave the placeholder as-is
			// This allows for backward compatibility and flexible configuration
			continue
		}

		// Replace the placeholder with the actual variable value
		result = strings.ReplaceAll(result, placeholder, varValue)
	}

	return result
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

// ToJavaScriptWithSecrets converts a string with both variable and secret interpolation to JavaScript code
// This is used by multihttp to generate JavaScript that references both variables and secrets
func ToJavaScriptWithSecrets(value string) string {
	if len(value) == 0 {
		return `''`
	}

	var s strings.Builder
	buf := []byte(value)

	// First handle secret variables
	p := handleSecretVariables(&s, buf, 0)

	// Then handle regular variables in the remaining text
	handleRegularVariables(&s, buf[p:])

	return s.String()
}

// handleSecretVariables processes secret variables in the buffer and returns the position after the last secret
func handleSecretVariables(s *strings.Builder, buf []byte, startPos int) int {
	locs := SecretRegex.FindAllSubmatchIndex(buf, -1)
	p := startPos

	for _, loc := range locs {
		if len(loc) < 4 {
			panic("unexpected result while building JavaScript")
		}

		writePlusIfNeeded(s)
		writeTextBeforeMatch(s, buf, p, loc[0])

		// Generate async secret lookup
		s.WriteString(`await secrets.get('`)
		s.Write(buf[loc[2]:loc[3]])
		s.WriteString(`')`)

		p = loc[1]
	}

	// Write any remaining text after secrets
	if len(buf[p:]) > 0 {
		writePlusIfNeeded(s)
		writeQuotedText(s, buf[p:])
	}

	return p
}

// handleRegularVariables processes regular variables in the remaining text
func handleRegularVariables(s *strings.Builder, remainingText []byte) {
	if len(remainingText) == 0 {
		return
	}

	regularLocs := VariableRegex.FindAllSubmatchIndex(remainingText, -1)

	if len(regularLocs) > 0 {
		processRegularVariableMatches(s, remainingText, regularLocs)
	} else {
		// No regular variables, just append the remaining text
		writePlusIfNeeded(s)
		writeQuotedText(s, remainingText)
	}
}

// processRegularVariableMatches processes the matches found by VariableRegex
func processRegularVariableMatches(s *strings.Builder, remainingText []byte, regularLocs [][]int) {
	writePlusIfNeeded(s)

	p2 := 0
	for _, loc := range regularLocs {
		if len(loc) < 4 {
			panic("unexpected result while building JavaScript")
		}

		writePlusIfNeeded(s)
		writeTextBeforeMatch(s, remainingText, p2, loc[0])

		s.WriteString(`vars['`)
		s.Write(remainingText[loc[2]:loc[3]])
		s.WriteString(`']`)

		p2 = loc[1]
	}

	// Write any remaining text after the last variable
	if len(remainingText[p2:]) > 0 {
		writePlusIfNeeded(s)
		writeQuotedText(s, remainingText[p2:])
	}
}

// writePlusIfNeeded writes a plus sign if the builder already has content
func writePlusIfNeeded(s *strings.Builder) {
	if s.Len() > 0 {
		s.WriteRune('+')
	}
}

// writeTextBeforeMatch writes the text before a regex match, quoted and escaped
func writeTextBeforeMatch(s *strings.Builder, buf []byte, start, matchStart int) {
	if pre := buf[start:matchStart]; len(pre) > 0 {
		writeQuotedText(s, pre)
		s.WriteRune('+')
	}
}

// writeQuotedText writes text as a quoted JavaScript string with proper escaping
func writeQuotedText(s *strings.Builder, text []byte) {
	s.WriteRune('\'')
	escapeJavaScript(s, text)
	s.WriteRune('\'')
}

// escapeJavaScript escapes a byte slice for use in JavaScript strings
func escapeJavaScript(s *strings.Builder, buf []byte) {
	for _, b := range buf {
		switch b {
		case '\'':
			s.WriteString(`\'`)
		case '"':
			s.WriteString(`\"`)
		case '\\':
			s.WriteString(`\\`)
		case '\n':
			s.WriteString(`\n`)
		case '\r':
			s.WriteString(`\r`)
		case '\t':
			s.WriteString(`\t`)
		case '=':
			s.WriteString(`\u003D`)
		case '>':
			s.WriteString(`\u003E`)
		case '<':
			s.WriteString(`\u003C`)
		case '&':
			s.WriteString(`\u0026`)
		default:
			if b < 32 {
				// Escape control characters using Unicode escape
				fmt.Fprintf(s, `\u%04X`, b)
			} else {
				s.WriteByte(b)
			}
		}
	}
}
