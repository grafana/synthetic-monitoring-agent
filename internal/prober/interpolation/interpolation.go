package interpolation

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/secrets"
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

// ErrInvalidSecretName is the reason Resolve gives for a reference whose name is not a valid
// secret name.
var ErrInvalidSecretName = errors.New("invalid secret name: use up to 253 lowercase letters, digits, '-' and '.', starting and ending with a letter or digit")

// SecretProvider defines the interface for resolving secrets
type SecretProvider interface {
	GetSecretValue(ctx context.Context, tenantID model.GlobalID, secretKey string) (string, error)
}

// Resolver handles string interpolation for secrets. It is not safe for concurrent use.
type Resolver struct {
	secretProvider SecretProvider
	tenantID       model.GlobalID
	logger         zerolog.Logger
	// refs maps each secret value Resolve returned to its reference, for Redactor.
	refs map[string]string
}

// NewResolver creates a new interpolation resolver
func NewResolver(secretProvider SecretProvider, tenantID model.GlobalID, logger zerolog.Logger) *Resolver {
	return &Resolver{
		secretProvider: secretProvider,
		tenantID:       tenantID,
		logger:         logger,
		refs:           make(map[string]string),
	}
}

// Resolve replaces every ${secrets.name} reference in value with that secret's value. Everything
// else is copied through byte for byte.
func (r *Resolver) Resolve(ctx context.Context, value string) (string, error) {
	// No "${secrets." in the value, so skip all the secret work.
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
			return "", &secrets.Error{Reason: ErrInvalidSecretName, Err: fmt.Errorf("name %q", secretName)}
		}

		secretMatches = append(secretMatches, secretMatch{
			start: match[0],
			end:   match[1],
			name:  secretName,
		})
	}

	// The value is just "${secrets.x}" with nothing around it, so skip the builder.
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
		secretErr := secrets.AsError(err)
		secretErr.Secret = name

		return "", secretErr
	}

	// An empty value is left out, because a strings.Replacer matches "" at every position.
	if secretValue != "" {
		r.refs[secretValue] = SecretRefPrefix + name + "}"
	}

	return secretValue, nil
}

// Redactor returns a function that replaces each secret value r has resolved with its
// ${secrets.name} reference, or nil when r has resolved none. Values that r resolves after the
// call are not replaced. The function is safe for concurrent use.
func (r *Resolver) Redactor() func(string) string {
	if len(r.refs) == 0 {
		return nil
	}

	// Longest first, so that a value that holds another value is replaced as a whole.
	values := slices.SortedFunc(maps.Keys(r.refs), func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), cmp.Compare(a, b))
	})

	oldnew := make([]string, 0, 2*len(values))
	for _, value := range values {
		oldnew = append(oldnew, value, r.refs[value])
	}

	return strings.NewReplacer(oldnew...).Replace
}

// isValidSecretName validates that a secret name follows Kubernetes DNS subdomain naming convention.
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
func isValidSecretName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}

	return secretNameRegex.MatchString(name)
}
