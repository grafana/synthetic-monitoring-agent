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

	"github.com/go-kit/log/level"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/logger"
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

// ErrInvalidSecretName is the reason Resolver gives for a reference whose name is not a valid
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
	// refs maps each secret value r has resolved to its reference, for Redactor.
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

// ResolveField replaces each ${secrets.name} in value with that secret's value, and leaves the
// rest as is. An error names field, the setting that value comes from.
func (r *Resolver) ResolveField(ctx context.Context, field, value string) (string, error) {
	return r.ResolveFunc(ctx, field, value, nil)
}

// ResolveFunc is ResolveField with transform applied to each secret value, not to the text around
// it. Redactor still matches the value as stored.
func (r *Resolver) ResolveFunc(ctx context.Context, field, value string, transform func(string) (string, error)) (string, error) {
	resolved, err := r.resolve(ctx, value, transform)
	if err != nil {
		secretErr := secrets.AsError(err)
		secretErr.Field = field

		return "", secretErr
	}

	return resolved, nil
}

func (r *Resolver) resolve(ctx context.Context, value string, transform func(string) (string, error)) (string, error) {
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
		return r.resolveSecret(ctx, secretMatches[0].name, transform)
	}

	// Step 2: Write out the gaps between the secrets verbatim, resolving each secret in turn
	var result strings.Builder

	lastPos := 0

	for _, secretMatch := range secretMatches {
		result.WriteString(value[lastPos:secretMatch.start])

		secretValue, err := r.resolveSecret(ctx, secretMatch.name, transform)
		if err != nil {
			return "", err
		}

		result.WriteString(secretValue)

		lastPos = secretMatch.end
	}

	result.WriteString(value[lastPos:])

	return result.String(), nil
}

// LogSecretError logs err to the check logs l if it is a secrets.Error. Only LogFields are logged,
// because the wrapped error can hold the secret store URL.
func LogSecretError(l logger.Logger, err error) {
	secretErr, ok := errors.AsType[*secrets.Error](err)
	if !ok {
		return
	}

	msg := "Could not " + secretErr.Verb() + " a secret referenced by this check"

	_ = level.Error(l).Log(append([]any{"msg", msg}, secretErr.LogFields()...)...)
}

func (r *Resolver) resolveSecret(ctx context.Context, name string, transform func(string) (string, error)) (string, error) {
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

	if transform == nil {
		return secretValue, nil
	}

	secretValue, err = transform(secretValue)
	if err != nil {
		secretErr := secrets.AsError(err)
		secretErr.Secret = name

		return "", secretErr
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
