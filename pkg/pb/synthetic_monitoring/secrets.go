package synthetic_monitoring

import (
	"bytes"
	"iter"
	"regexp"
	"slices"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// The API and the agent both need to know whether a check needs a secret
// before it can run, so the answer lives here, in the package they share.

// k6SecretsModule is the module a script loads to read secrets.
const k6SecretsModule = `k6/secrets`

// secretRefRegex matches ${secrets.name}, the way a check points at a secret.
//
// The name is matched loosely on purpose. ${secrets.My_Token} cannot resolve,
// but whoever wrote it wanted a secret, so it still counts as one.
var secretRefRegex = regexp.MustCompile(`\$\{secrets\.([^}]*)\}`)

// ReferencesSecrets reports whether the check points at a secret that has to be
// resolved before it can run.
//
// For protocol checks the answer is exact: a reference is a fixed piece of text
// in a known field. For script checks it is a best guess, because a script can
// build the module name while it runs.
//
// The guess errs towards yes. Saying yes when the answer was no only limits
// which probes may run the check. Saying no when the answer was yes sends the
// unresolved text to the customer's target, so the check fails to log in and
// nothing explains why.
func (c Check) ReferencesSecrets() bool {
	return c.Settings.ReferencesSecrets()
}

// ReferencesSecrets reports whether the check points at a secret. See
// Check.ReferencesSecrets.
func (c AdHocCheck) ReferencesSecrets() bool {
	return c.Settings.ReferencesSecrets()
}

// SecretFields yields each field of the check that points at a secret. See
// CheckSettings.SecretFields.
func (c Check) SecretFields() iter.Seq[SecretField] {
	return c.Settings.SecretFields()
}

// SecretFields yields each field of the check that points at a secret. See
// CheckSettings.SecretFields.
func (c AdHocCheck) SecretFields() iter.Seq[SecretField] {
	return c.Settings.SecretFields()
}

// SecretField names a settings field that can hold a secret reference. The
// values are stable: callers map them to the probe versions that resolve them.
type SecretField string

// Values name fields in the schema and never carry a list index, because
// whether a probe resolves secrets depends only on the field.
const (
	SecretFieldHttpBearerToken       SecretField = "http.bearerToken"
	SecretFieldHttpBasicAuthPassword SecretField = "http.basicAuth.password"
	SecretFieldHttpTLSCACert         SecretField = "http.tlsConfig.caCert"
	SecretFieldHttpTLSClientCert     SecretField = "http.tlsConfig.clientCert"
	SecretFieldHttpTLSClientKey      SecretField = "http.tlsConfig.clientKey"
	SecretFieldHttpHeaders           SecretField = "http.headers"
	SecretFieldScriptedScript        SecretField = "scripted.script"
	SecretFieldBrowserScript         SecretField = "browser.script"

	// SecretFieldUnknown is reported for a check type nobody has classified
	// yet. Callers should not match it to any probe.
	SecretFieldUnknown SecretField = "unknown"
)

// ReferencesSecrets reports whether these settings point at a secret. See
// Check.ReferencesSecrets.
func (s CheckSettings) ReferencesSecrets() bool {
	for range s.SecretFields() {
		return true
	}

	return false
}

// SecretFields yields each field that points at a secret, once, in a fixed
// order. Use slices.Collect to get a slice.
func (s CheckSettings) SecretFields() iter.Seq[SecretField] {
	return func(yield func(SecretField) bool) {
		checkType, found := s.checkType()
		if !found {
			// Nothing is set, which is what a check being deleted looks like.
			return
		}

		switch checkType {
		case CheckTypeHttp:
			s.Http.secretFields(yield)

		case CheckTypeScripted:
			if scriptReferencesSecrets(s.Scripted.Script) {
				yield(SecretFieldScriptedScript)
			}

		case CheckTypeBrowser:
			if scriptReferencesSecrets(s.Browser.Script) {
				yield(SecretFieldBrowserScript)
			}

		case CheckTypeDns, CheckTypeGrpc, CheckTypePing, CheckTypeTcp, CheckTypeTraceroute:
			// None of these has a field that takes a secret.

		case CheckTypeMultiHttp:
			// Multihttp has credential fields, but nothing resolves secrets in them yet.

		default:
			yield(SecretFieldUnknown)
		}
	}
}

// secretFields yields each secret field that holds a reference.
func (h *HttpSettings) secretFields(yield func(SecretField) bool) {
	if h == nil {
		return
	}

	if secretRefRegex.MatchString(h.BearerToken) && !yield(SecretFieldHttpBearerToken) {
		return
	}

	if h.BasicAuth != nil && secretRefRegex.MatchString(h.BasicAuth.Password) && !yield(SecretFieldHttpBasicAuthPassword) {
		return
	}

	if h.TlsConfig != nil {
		tlsFields := [...]struct {
			field SecretField
			value []byte
		}{
			{SecretFieldHttpTLSCACert, h.TlsConfig.CACert},
			{SecretFieldHttpTLSClientCert, h.TlsConfig.ClientCert},
			{SecretFieldHttpTLSClientKey, h.TlsConfig.ClientKey},
		}

		for _, f := range tlsFields {
			if secretRefRegex.Match(f.value) && !yield(f.field) {
				return
			}
		}
	}

	if slices.ContainsFunc(h.Headers, headerValueReferencesSecrets) {
		yield(SecretFieldHttpHeaders)
	}
}

// headerValueReferencesSecrets reports whether the value of a "Name: value"
// header points at a secret. The name is left out.
func headerValueReferencesSecrets(header string) bool {
	_, value, found := strings.Cut(header, ":")

	return found && secretRefRegex.MatchString(value)
}

// scriptReferencesSecrets reports whether the script names the secrets module.
//
// The script is parsed because a module can be loaded in several ways, and
// only the first two below are import statements:
//
//	import secrets from 'k6/secrets'
//	import * as s from 'k6/secrets'
//	const s = await import('k6/secrets')
//	let anything = require('k6/secrets')
//
// A pattern over import statements misses the other two. Parsing finds all
// four and skips a mention inside a comment with no extra rule. It also reads
// a regular expression literal such as /['"]/ correctly. A simpler scan takes
// the quote inside it for the start of a string and misreads everything after
// it.
//
// Parsing cannot see a name built while the script runs, as in
// require('k6' + '/secrets').
func scriptReferencesSecrets(script []byte) bool {
	// The name has to appear somewhere for the module to be loaded, so a script
	// that never mentions it needs no parsing. That is nearly every script.
	if !bytes.Contains(script, []byte(k6SecretsModule)) {
		return false
	}

	ast, err := js.Parse(parse.NewInputBytes(script), js.Options{})
	if err != nil {
		// The script mentions the module but does not parse, so it may still load the module.
		return true
	}

	var v secretsVisitor

	js.Walk(&v, ast)

	return v.found
}

// secretsVisitor walks a parsed script looking for the module name. It can turn
// up in three places: an import statement, a plain string, or a string written
// with backticks.
type secretsVisitor struct {
	found bool
}

func (v *secretsVisitor) Enter(n js.INode) js.IVisitor {
	if v.found {
		return nil
	}

	needle := []byte(k6SecretsModule)

	switch node := n.(type) {
	case *js.ImportStmt:
		v.found = bytes.Contains(node.Module, needle)

	case *js.LiteralExpr:
		v.found = node.TokenType == js.StringToken && bytes.Contains(node.Data, needle)

	case *js.TemplateExpr:
		v.found = bytes.Contains(node.Tail, needle) ||
			slices.ContainsFunc(node.List, func(part js.TemplatePart) bool {
				return bytes.Contains(part.Value, needle)
			})
	}

	if v.found {
		return nil
	}

	return v
}

func (v *secretsVisitor) Exit(js.INode) {}
