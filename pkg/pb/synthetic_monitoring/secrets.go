package synthetic_monitoring

import (
	"bytes"
	"regexp"
	"slices"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// Does this check need a secret before it can run? The API and the agent both
// need that answer, so it is worked out here, once, in the package they share.

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

// ReferencesSecrets reports whether these settings point at a secret. See
// Check.ReferencesSecrets.
func (s CheckSettings) ReferencesSecrets() bool {
	checkType, found := s.checkType()
	if !found {
		// Nothing is set, which is what a check being deleted looks like.
		return false
	}

	switch checkType {
	case CheckTypeHttp:
		return s.Http.referencesSecrets()

	case CheckTypeScripted:
		return scriptReferencesSecrets(s.Scripted.Script)

	case CheckTypeBrowser:
		return scriptReferencesSecrets(s.Browser.Script)

	case CheckTypeDns, CheckTypeGrpc, CheckTypePing, CheckTypeTcp, CheckTypeTraceroute:
		// None of these has a field that takes a secret.
		return false

	case CheckTypeMultiHttp:
		// Multihttp has credential fields, but nothing resolves secrets in
		// them yet.
		return false

	default:
		// A new check type, added before anyone decided whether it can carry a
		// secret. Yes is the safe answer; the type belongs in a case above.
		return true
	}
}

// referencesSecrets reports whether any of the five fields that support secrets
// holds a reference: the bearer token, the basic auth password and the three
// TLS fields.
func (h *HttpSettings) referencesSecrets() bool {
	if h == nil {
		return false
	}

	if secretRefRegex.MatchString(h.BearerToken) {
		return true
	}

	if h.BasicAuth != nil && secretRefRegex.MatchString(h.BasicAuth.Password) {
		return true
	}

	if h.TlsConfig != nil && slices.ContainsFunc(
		[][]byte{h.TlsConfig.CACert, h.TlsConfig.ClientCert, h.TlsConfig.ClientKey},
		secretRefRegex.Match,
	) {
		return true
	}

	return false
}

// scriptReferencesSecrets reports whether the script names the secrets module.
//
// The script is parsed rather than pattern-matched, because a module can be
// loaded in more ways than one:
//
//	import secrets from 'k6/secrets'
//	import * as s from 'k6/secrets'
//	const s = await import('k6/secrets')
//	let anything = require('k6/secrets')
//
// Only the first two are import statements, so matching the shape of an import
// misses the rest. Parsing finds all four, and ignores a mention inside a
// comment without being told to. It also reads a line like /['"]/ correctly,
// where a quote sits inside a pattern - a simpler scan mistakes that quote for
// the start of a string and misreads everything after it.
//
// What nothing can see is a name built while the script runs, as in
// require('k6' + '/secrets').
func scriptReferencesSecrets(script []byte) bool {
	// The name has to appear somewhere for the module to be loaded, so a script
	// that never mentions it needs no parsing. That is nearly every script.
	if !bytes.Contains(script, []byte(k6SecretsModule)) {
		return false
	}

	ast, err := js.Parse(parse.NewInputBytes(script), js.Options{})
	if err != nil {
		// The script mentions the module and cannot be read. Being unable to
		// read it is no reason to answer no.
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
