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

// secretNameRegex matches a Kubernetes DNS subdomain name: lowercase alphanumerics, '-' and
// '.', starting and ending with an alphanumeric.
var secretNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-\.]*[a-z0-9])?$`)

// SecretRefPrefix is the literal start of every SecretRegex match.
const SecretRefPrefix = "${secrets."

// HasSecretRef reports whether value may hold a ${secrets.name} reference.
func HasSecretRef(value string) bool {
	return strings.Contains(value, SecretRefPrefix)
}

// SecretProvider defines the interface for resolving secrets
type SecretProvider interface {
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

// Resolver handles string interpolation for secrets
type Resolver struct {
	secretProvider SecretProvider
	tenantID       model.GlobalID
	logger         zerolog.Logger
}

// NewResolver creates a new interpolation resolver
func NewResolver(secretProvider SecretProvider, tenantID model.GlobalID, logger zerolog.Logger) *Resolver {
	return &Resolver{
		secretProvider: secretProvider,
		tenantID:       tenantID,
		logger:         logger,
	}
}

// Resolve replaces every ${secrets.name} reference in value with that secret's value. Everything
// else is copied through byte for byte, including a bare ${name}, which this package does not
// expand.
func (r *Resolver) Resolve(ctx context.Context, value string) (string, error) {
	// Runs on every probe, and almost no value holds a reference.
	if !HasSecretRef(value) {
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

	// The whole value is one reference, so there is nothing to build.
	if len(secretMatches) == 1 && secretMatches[0].start == 0 && secretMatches[0].end == len(value) {
		return r.resolveSecret(ctx, secretMatches[0].name)
	}

	// Step 2: Write out the gaps between the secrets verbatim, resolving each secret in turn
	var result strings.Builder

	lastPos := 0

	for _, secretMatch := range secretMatches {
		result.WriteString(value[lastPos:secretMatch.start])

		secretValue, err := r.resolveSecret(ctx, secretMatch.name)
		if err != nil {
			return "", err
		}

		result.WriteString(secretValue)

		lastPos = secretMatch.end
	}

	result.WriteString(value[lastPos:])

	return result.String(), nil
}

func (r *Resolver) resolveSecret(ctx context.Context, name string) (string, error) {
	r.logger.Debug().Str("secretName", name).Int64("tenantId", int64(r.tenantID)).Msg("resolving secret from GSM")

	secretValue, err := r.secretProvider.GetSecretValue(ctx, r.tenantID, name)
	if err != nil {
		return "", fmt.Errorf("failed to get secret '%s' from GSM: %w", name, err)
	}

	return secretValue, nil
}

// isValidSecretName validates that a secret name follows Kubernetes DNS subdomain naming convention.
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
func isValidSecretName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}

	return secretNameRegex.MatchString(name)
}
