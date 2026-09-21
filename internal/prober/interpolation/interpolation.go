package interpolation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/rs/zerolog"
)

// SecretRegex matches ${secrets.secret_name} patterns
var SecretRegex = regexp.MustCompile(`\$\{secrets\.([^}]*)\}`)

// SecretProvider defines the interface for resolving secrets
type SecretProvider interface {
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

// Resolver handles string interpolation for secrets
type Resolver struct {
	secretProvider SecretProvider
	tenantID       model.GlobalID
	logger         zerolog.Logger
	secretEnabled  bool
}

// NewResolver creates a new interpolation resolver
func NewResolver(secretProvider SecretProvider, tenantID model.GlobalID, logger zerolog.Logger, secretEnabled bool) *Resolver {
	return &Resolver{
		secretProvider: secretProvider,
		tenantID:       tenantID,
		logger:         logger,
		secretEnabled:  secretEnabled,
	}
}

// Resolve replaces every ${secrets.name} reference in value with that secret's value. Everything
// else is copied through byte for byte, including a bare ${name}, which this package does not
// expand.
func (r *Resolver) Resolve(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "", nil
	}

	// If secrets are disabled, the value is not ours to touch
	if !r.secretEnabled {
		return value, nil
	}

	// Step 1: Find all secret matches with their positions
	type secretMatch struct {
		start, end int
		name       string
	}

	var secretMatches []secretMatch

	matches := SecretRegex.FindAllStringSubmatchIndex(value, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		secretName := value[match[2]:match[3]]

		// Validate secret name follows Kubernetes DNS subdomain naming convention
		if !isValidSecretName(secretName) {
			return "", fmt.Errorf("invalid secret name '%s': must follow Kubernetes DNS subdomain naming convention", secretName)
		}

		secretMatches = append(secretMatches, secretMatch{
			start: match[0],
			end:   match[1],
			name:  secretName,
		})
	}

	// Step 2: Write out the gaps between the secrets verbatim, resolving each secret in turn
	var result strings.Builder

	lastPos := 0

	for _, secretMatch := range secretMatches {
		result.WriteString(value[lastPos:secretMatch.start])

		r.logger.Debug().Str("secretName", secretMatch.name).Int64("tenantId", int64(r.tenantID)).Msg("resolving secret from GSM")

		secretValue, err := r.secretProvider.GetSecretValue(ctx, r.tenantID, secretMatch.name)
		if err != nil {
			return "", fmt.Errorf("failed to get secret '%s' from GSM: %w", secretMatch.name, err)
		}

		result.WriteString(secretValue)

		lastPos = secretMatch.end
	}

	result.WriteString(value[lastPos:])

	return result.String(), nil
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
