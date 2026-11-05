package multihttp

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// embed script template
//
//go:embed script.tmpl
var templateFS embed.FS

// Query params must be appended to a URL that has already been created.
// urlVarName is the variable name to reference when appending params.
func buildQueryParams(urlVarName string, req *sm.MultiHttpEntryRequest) []string {
	var buf strings.Builder

	out := make([]string, 0, len(req.QueryFields))
	for _, field := range req.QueryFields {
		buf.Reset()
		buf.WriteString(urlVarName)
		buf.WriteString(".searchParams.append(")
		buf.WriteString(interpolation.ExpandVariablesToJS(field.Name))
		buf.WriteString(", ")
		buf.WriteString(interpolation.ExpandVariablesToJS(field.Value))
		buf.WriteString(")")
		out = append(out, buf.String())
	}

	return out
}

// bodyHasPayload reports whether body has a non-empty payload. buildBody then
// emits an encoding.b64decode call, which needs the k6/encoding module.
func bodyHasPayload(body *sm.HttpRequestBody) bool {
	return body != nil && len(body.Payload) > 0
}

func buildBody(body *sm.HttpRequestBody) string {
	switch {
	case body == nil:
		return "null"

	case !bodyHasPayload(body):
		return `""`

	default:
		var buf strings.Builder

		buf.WriteString(`encoding.b64decode("`)
		buf.WriteString(base64.RawStdEncoding.EncodeToString(body.Payload))
		buf.WriteString(`", 'rawstd', "s")`)

		return buf.String()
	}
}

func interpolateBodyVariables(bodyVarName string, body *sm.HttpRequestBody) []string {
	switch {
	case !bodyHasPayload(body):
		return nil

	default:
		var buf strings.Builder

		matches := interpolation.FindVariableRefs(string(body.Payload))
		parsedMatches := make(map[string]struct{})
		out := make([]string, 0, len(matches))

		// For every instance of ${variable} in the body,
		// this block returns {bodyVarName} = {bodyVarName}.replaceAll('${variable}', vars['variable'])
		for _, m := range matches {
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
			// writing the variable name from between ${ and }
			for i := 2; i < len(m)-1; i++ {
				buf.WriteByte(m[i])
			}

			buf.WriteString("'])")
			out = append(out, buf.String())

			parsedMatches[m] = struct{}{}
		}

		return out
	}
}

func buildHeaders(headers []*sm.HttpHeader, body *sm.HttpRequestBody) string {
	var buf strings.Builder

	if len(headers) == 0 && body == nil {
		return ""
	}

	buf.WriteRune('{')

	comma := ""

	if body != nil {
		if len(body.ContentType) > 0 {
			buf.WriteString(`'Content-Type':"`)
			buf.WriteString(template.JSEscapeString(body.ContentType))
			buf.WriteRune('"')

			comma = ","
		}

		if len(body.ContentEncoding) > 0 {
			buf.WriteString(comma)

			buf.WriteString(`'Content-Encoding':"`)
			buf.WriteString(template.JSEscapeString(body.ContentEncoding))
			buf.WriteRune('"')

			comma = ","
		}
	}

	for _, header := range headers {
		buf.WriteString(comma)

		buf.WriteRune('"')
		buf.WriteString(template.JSEscapeString(header.Name))
		buf.WriteString(`":`)
		buf.WriteString(interpolation.ExpandVariablesToJS(header.Value))

		comma = ","
	}

	buf.WriteRune('}')

	return buf.String()
}

type assertionCondition sm.MultiHttpEntryAssertionConditionVariant

func (c assertionCondition) Name(w *strings.Builder, subject, value string) {
	w.WriteString(template.JSEscapeString(subject))
	w.WriteRune(' ')

	switch sm.MultiHttpEntryAssertionConditionVariant(c) {
	case sm.MultiHttpEntryAssertionConditionVariant_NOT_CONTAINS:
		w.WriteString(`does not contain`)

	case sm.MultiHttpEntryAssertionConditionVariant_CONTAINS, sm.MultiHttpEntryAssertionConditionVariant_DEFAULT_CONDITION:
		w.WriteString(`contains`)

	case sm.MultiHttpEntryAssertionConditionVariant_EQUALS:
		w.WriteString(`equals`)

	case sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH:
		w.WriteString(`starts with`)

	case sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH:
		w.WriteString(`ends with`)
	}

	w.WriteString(` \"`)
	w.WriteString(template.JSEscapeString(value))
	w.WriteString(`\"`)
}

func (c assertionCondition) Render(w *strings.Builder, subject, value string) {
	switch sm.MultiHttpEntryAssertionConditionVariant(c) {
	case sm.MultiHttpEntryAssertionConditionVariant_NOT_CONTAINS:
		w.WriteRune('!')
		fallthrough

	case sm.MultiHttpEntryAssertionConditionVariant_CONTAINS, sm.MultiHttpEntryAssertionConditionVariant_DEFAULT_CONDITION:
		w.WriteString(subject)
		w.WriteString(`.includes("`)
		w.WriteString(template.JSEscapeString(value))
		w.WriteString(`")`)

	case sm.MultiHttpEntryAssertionConditionVariant_EQUALS:
		w.WriteString(subject)
		w.WriteString(` === "`)
		w.WriteString(template.JSEscapeString(value))
		w.WriteString(`"`)

	case sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH:
		w.WriteString(subject)
		w.WriteString(`.startsWith("`)
		w.WriteString(template.JSEscapeString(value))
		w.WriteString(`")`)

	case sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH:
		w.WriteString(subject)
		w.WriteString(`.endsWith("`)
		w.WriteString(template.JSEscapeString(value))
		w.WriteString(`")`)
	}
}

// buildChecks takes a single assertion and produces the corresponding JavaScript code.
//
// This function is a mess because a single assertion represents multiple types
// of checks.
func buildChecks(urlVarName, method string, assertion *sm.MultiHttpEntryAssertion) string {
	var (
		b                   strings.Builder
		assertionDescriptor strings.Builder
	)

	b.WriteString(`currentCheck = check(response, { "`)

	switch assertion.Type {
	case sm.MultiHttpEntryAssertionType_TEXT:
		cond := assertionCondition(assertion.Condition)

		switch assertion.Subject {
		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY, sm.MultiHttpEntryAssertionSubjectVariant_DEFAULT_SUBJECT:
			cond.Name(&b, "body", assertion.Value)
			b.WriteString(`": response => `)
			cond.Render(&b, "response.body", assertion.Value)
			cond.Render(&assertionDescriptor, "response.body", assertion.Value)

		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS:
			cond.Name(&b, "header", assertion.Value)
			b.WriteString(`": response => { `)

			if len(assertion.Expression) == 0 {
				// No expression provided, match the entire value against all headers.
				b.WriteString(`const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); `)
				b.WriteString(`return !!values.find(value => `)
				cond.Render(&b, "value", assertion.Value)
				cond.Render(&assertionDescriptor, "value", assertion.Value)
				b.WriteString(`);`)
			} else {
				// Expression provided, search for a matching header.
				b.WriteString(`return assertHeader(response.headers, "`)
				b.WriteString(template.JSEscapeString(assertion.Expression))
				b.WriteString(`", `)
				b.WriteString(`v => `)
				cond.Render(&b, "value", assertion.Value)
				cond.Render(&assertionDescriptor, "value", assertion.Value)
				b.WriteString(`);`)
			}

			b.WriteString(` }`)

		case sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE:
			cond.Name(&b, "status code", assertion.Value)
			b.WriteString(`": response => `)
			cond.Render(&b, `response.status.toString()`, assertion.Value)
			cond.Render(&assertionDescriptor, `response.status.toString()`, assertion.Value)
		}

	case sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE:
		cond := assertionCondition(assertion.Condition)
		cond.Name(&b, assertion.Expression, assertion.Value)
		b.WriteString(`": response => jsonpath.query(response.json(), "`)
		b.WriteString(template.JSEscapeString(assertion.Expression))
		b.WriteString(`").some(values => `)
		cond.Render(&b, `values`, assertion.Value)
		cond.Render(&assertionDescriptor, `values`, assertion.Value)
		b.WriteString(`)`)

	case sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION:
		b.WriteString(template.JSEscapeString(assertion.Expression))
		b.WriteString(` exists": response => jsonpath.query(response.json(), "`)
		b.WriteString(template.JSEscapeString(assertion.Expression))
		assertionDescriptor.WriteString(`JsonPath expression `)
		assertionDescriptor.WriteString(assertion.Expression)
		b.WriteString(`").length > 0`)

	case sm.MultiHttpEntryAssertionType_REGEX_ASSERTION:
		switch assertion.Subject {
		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY, sm.MultiHttpEntryAssertionSubjectVariant_DEFAULT_SUBJECT:
			b.WriteString(`body matches /`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`/": response => { const expr = new RegExp("`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`"); `)
			b.WriteString(`return expr.test(response.body); }`)
			assertionDescriptor.WriteString("Body matches")
			assertionDescriptor.WriteString(assertion.Expression)

		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS:
			b.WriteString(`headers matches /`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`/": response => { const expr = new RegExp("`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`"); `)
			b.WriteString(`const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); `)
			b.WriteString(`return !!values.find(value => expr.test(value)); }`)
			assertionDescriptor.WriteString("Headers match")
			assertionDescriptor.WriteString(assertion.Expression)

		case sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE:
			b.WriteString(`status matches /`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`/": response => { const expr = new RegExp("`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`"); `)
			b.WriteString(`return expr.test(response.status.toString()); }`)
			assertionDescriptor.WriteString("Status matches")
			assertionDescriptor.WriteString(assertion.Expression)
		}
	}

	b.WriteString(` }`)
	b.WriteString(`, `)

	// Add tags to the check: url, method
	b.WriteString(`{`)
	b.WriteString(`"url": `)
	b.WriteString(urlVarName)
	b.WriteString(`.toString(), `)
	b.WriteString(`"method": "`)
	b.WriteString(method)
	b.WriteRune('"')
	b.WriteString(`}`)

	b.WriteString(`);`)

	b.WriteString("\n\t")
	b.WriteString(`if(!currentCheck) {`)
	b.WriteString("\n\t\t")
	b.WriteString(`console.error("Assertion failed:", "`)
	b.WriteString(template.JSEscapeString(assertionDescriptor.String()))
	b.WriteString(`");`)
	b.WriteString("\n\t\t")
	b.WriteString(`fail()`)
	b.WriteString("\n\t")
	b.WriteString(`};`)
	b.WriteString("\n")

	return b.String()
}

func buildVars(variable *sm.MultiHttpEntryVariable) string {
	var b strings.Builder

	if variable.Type == sm.MultiHttpEntryVariableType_REGEX {
		b.WriteString(`match = new RegExp('`)
		b.WriteString(template.JSEscapeString(variable.Expression))
		b.WriteString(`').exec(response.body); `)
	}

	b.WriteString(`vars['`)
	b.WriteString(template.JSEscapeString(variable.Name))
	b.WriteString(`'] = `)

	switch variable.Type {
	case sm.MultiHttpEntryVariableType_JSON_PATH:
		b.WriteString(`jsonpath.query(response.json(), '`)
		b.WriteString(template.JSEscapeString(variable.Expression))
		b.WriteString(`')[0]`)

	case sm.MultiHttpEntryVariableType_REGEX:
		b.WriteString(`match ? match[1] || match[0] : null`)

	case sm.MultiHttpEntryVariableType_CSS_SELECTOR:
		b.WriteString(`response.html('`)
		b.WriteString(template.JSEscapeString(variable.Expression))
		b.WriteString(`')`)

		if variable.Attribute == "" {
			b.WriteString(`.html()`)
		} else {
			b.WriteString(`.first().attr('`)
			b.WriteString(template.JSEscapeString(variable.Attribute))
			b.WriteString(`')`)
		}
	}

	b.WriteString(`;`)

	return b.String()
}

// assertionUsesJSONPath reports whether buildChecks emits a jsonpath.query call
// for assertion.
func assertionUsesJSONPath(assertion *sm.MultiHttpEntryAssertion) bool {
	switch assertion.Type {
	case sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE, sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION:
		return true

	default:
		return false
	}
}

// variableUsesJSONPath reports whether buildVars emits a jsonpath.query call for
// variable.
func variableUsesJSONPath(variable *sm.MultiHttpEntryVariable) bool {
	return variable.Type == sm.MultiHttpEntryVariableType_JSON_PATH
}

// needsJSONPath reports whether the script for settings calls jsonpath.query.
//
// The jsonpath module is fetched from jslib.k6.io, and k6 parses, compiles and
// evaluates it on every run whether the script uses it or not, so the template
// imports it only when this returns true.
func needsJSONPath(settings *sm.MultiHttpSettings) bool {
	for _, entry := range settings.Entries {
		for _, assertion := range entry.Assertions {
			if assertionUsesJSONPath(assertion) {
				return true
			}
		}

		for _, variable := range entry.Variables {
			if variableUsesJSONPath(variable) {
				return true
			}
		}
	}

	return false
}

// needsEncoding reports whether the script for settings calls
// encoding.b64decode, so that the template imports k6/encoding only when
// needed.
func needsEncoding(settings *sm.MultiHttpSettings) bool {
	for _, entry := range settings.Entries {
		if entry.Request != nil && bodyHasPayload(entry.Request.Body) {
			return true
		}
	}

	return false
}

// settingsToScript generates the k6 script for settings. When leanImports
// is true the script imports k6/encoding and jsonpath only if it uses them and
// omits k6/execution, which it never uses; otherwise it imports all of them.
func settingsToScript(settings *sm.MultiHttpSettings, leanImports bool) ([]byte, error) {
	// Convert settings to script using a Go template
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"buildBody":           buildBody,
			"buildChecks":         buildChecks,
			"buildHeaders":        buildHeaders,
			"buildUrl":            interpolation.ExpandVariablesToJS,
			"buildQueryParams":    buildQueryParams,
			"buildVars":           buildVars,
			"leanImports":         func() bool { return leanImports },
			"interpolateBodyVars": interpolateBodyVariables,
			"needsEncoding":       needsEncoding,
			"needsJSONPath":       needsJSONPath,
		}).
		ParseFS(templateFS, "*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing script template: %w", err)
	}

	var buf bytes.Buffer

	// TODO(mem): figure out if we need to transform the data in some way
	// before executing the template
	if err := tmpl.ExecuteTemplate(&buf, "script.tmpl", settings); err != nil {
		return nil, fmt.Errorf("executing script template: %w", err)
	}

	return buf.Bytes(), nil
}
