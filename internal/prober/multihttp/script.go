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

func buildBody(body *sm.HttpRequestBody) string {
	switch {
	case body == nil:
		return "null"

	case len(body.Payload) == 0:
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
	case body == nil || len(body.Payload) == 0:
		return nil

	default:
		var buf strings.Builder

		names := interpolation.VariableNames(string(body.Payload))
		out := make([]string, 0, len(names))

		// For every variable in the body, this block returns
		// {bodyVarName} = {bodyVarName}.replaceAll('${variable}', vars['variable'])
		for _, name := range names {
			buf.Reset()
			buf.WriteString(bodyVarName)
			buf.WriteString("=")
			buf.WriteString(bodyVarName)
			buf.WriteString(".replaceAll('${")
			buf.WriteString(name)
			buf.WriteString("}', vars['")
			buf.WriteString(name)
			buf.WriteString("'])")
			out = append(out, buf.String())
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

type jsExpr string

// jsString returns s as an escaped JavaScript string literal.
func jsString(s string) jsExpr {
	return jsExpr(`"` + template.JSEscapeString(s) + `"`)
}

// Render writes the condition as a JavaScript expression comparing subject
// against value.
func (c assertionCondition) Render(w *strings.Builder, subject string, value jsExpr) {
	switch sm.MultiHttpEntryAssertionConditionVariant(c) {
	case sm.MultiHttpEntryAssertionConditionVariant_NOT_CONTAINS:
		w.WriteRune('!')
		fallthrough

	case sm.MultiHttpEntryAssertionConditionVariant_CONTAINS, sm.MultiHttpEntryAssertionConditionVariant_DEFAULT_CONDITION:
		w.WriteString(subject)
		w.WriteString(`.includes(`)
		w.WriteString(string(value))
		w.WriteString(`)`)

	case sm.MultiHttpEntryAssertionConditionVariant_EQUALS:
		w.WriteString(subject)
		w.WriteString(` === `)
		w.WriteString(string(value))

	case sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH:
		w.WriteString(subject)
		w.WriteString(`.startsWith(`)
		w.WriteString(string(value))
		w.WriteString(`)`)

	case sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH:
		w.WriteString(subject)
		w.WriteString(`.endsWith(`)
		w.WriteString(string(value))
		w.WriteString(`)`)
	}
}

// assertionValue is the value side of an assertion. A value without variable
// references renders as a string literal. A value with references renders as
// an expression over vars, which k6 evaluates after earlier requests set them.
// The check name keeps the value as written either way, because it becomes a
// metric label and must not change from run to run.
type assertionValue struct {
	// literal is the value as written, as a string literal. The failure log
	// uses it, so the log never shows what a variable resolved to.
	literal jsExpr
	// expr is what the condition compares against.
	expr jsExpr
	// names lists the distinct variables expr reads, in order of appearance.
	names []string
}

func newAssertionValue(value string) assertionValue {
	v := assertionValue{
		literal: jsString(value),
		names:   interpolation.VariableNames(value),
	}

	if len(v.names) == 0 {
		v.expr = v.literal

		return v
	}

	// A variable can hold a number or an object, depending on how it was
	// extracted. The leading empty string makes every + a string
	// concatenation, so ${x} behaves as if the user had typed its value.
	// Without it, ${a}${b} with the numbers 20 and 0 would add up to 20.
	v.expr = jsExpr("''+" + interpolation.ExpandVariablesToJS(value))

	return v
}

// writeCondition writes the condition twice: into checkJS, the JavaScript of
// the check that k6 runs, and into failureLog, the text of the "Assertion
// failed:" log line, with the value as written.
func (v assertionValue) writeCondition(cond assertionCondition, checkJS, failureLog *strings.Builder, subject string) {
	cond.Render(checkJS, subject, v.expr)
	cond.Render(failureLog, subject, v.literal)
}

// openBlock starts the block body of a check's arrow function. The block first
// makes the check fail when a variable it reads is unset. Without that, the
// comparison would run against the string "undefined", and a condition such
// as "does not contain" would pass. vars is a plain object, so the own-property
// test keeps a name such as constructor from reading an inherited value.
func (v assertionValue) openBlock(w *strings.Builder) {
	w.WriteString(`{ `)

	for _, name := range v.names {
		w.WriteString(`if (!Object.hasOwn(vars, '`)
		w.WriteString(name)
		w.WriteString(`') || vars['`)
		w.WriteString(name)
		w.WriteString(`'] == null) { console.error("Variable is not defined:", "`)
		w.WriteString(name)
		w.WriteString(`"); return false; } `)
	}
}

// openBody and closeBody turn an arrow function body that is a single
// expression into a block, so the check in openBlock can run first. They
// write nothing for a value without variables.
func (v assertionValue) openBody(w *strings.Builder) {
	if len(v.names) == 0 {
		return
	}

	v.openBlock(w)
	w.WriteString(`return `)
}

func (v assertionValue) closeBody(w *strings.Builder) {
	if len(v.names) == 0 {
		return
	}

	w.WriteString(`; }`)
}

// writeExprBody writes an arrow function body that does nothing but compare
// subject with the value.
func (v assertionValue) writeExprBody(cond assertionCondition, checkJS, failureLog *strings.Builder, subject string) {
	v.openBody(checkJS)
	v.writeCondition(cond, checkJS, failureLog, subject)
	v.closeBody(checkJS)
}

// buildChecks takes a single assertion and produces the corresponding JavaScript code.
//
// This function is a mess because a single assertion represents multiple types
// of checks.
func buildChecks(urlVarName, method string, assertion *sm.MultiHttpEntryAssertion) string {
	var (
		checkJS    strings.Builder
		failureLog strings.Builder
	)

	checkJS.WriteString(`currentCheck = check(response, { "`)

	switch assertion.Type {
	case sm.MultiHttpEntryAssertionType_TEXT:
		cond := assertionCondition(assertion.Condition)
		value := newAssertionValue(assertion.Value)

		switch assertion.Subject {
		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY, sm.MultiHttpEntryAssertionSubjectVariant_DEFAULT_SUBJECT:
			cond.Name(&checkJS, "body", assertion.Value)
			checkJS.WriteString(`": response => `)
			value.writeExprBody(cond, &checkJS, &failureLog, "response.body")

		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS:
			cond.Name(&checkJS, "header", assertion.Value)
			checkJS.WriteString(`": response => `)
			value.openBlock(&checkJS)

			if len(assertion.Expression) == 0 {
				// No expression provided, match the entire value against all headers.
				checkJS.WriteString(`const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); `)
				checkJS.WriteString(`return !!values.find(value => `)
				value.writeCondition(cond, &checkJS, &failureLog, "value")
				checkJS.WriteString(`);`)
			} else {
				// Expression provided, search for a matching header.
				checkJS.WriteString(`return assertHeader(response.headers, "`)
				checkJS.WriteString(template.JSEscapeString(assertion.Expression))
				checkJS.WriteString(`", `)
				checkJS.WriteString(`value => `)
				value.writeCondition(cond, &checkJS, &failureLog, "value")
				checkJS.WriteString(`);`)
			}

			checkJS.WriteString(` }`)

		case sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE:
			cond.Name(&checkJS, "status code", assertion.Value)
			checkJS.WriteString(`": response => `)
			value.writeExprBody(cond, &checkJS, &failureLog, `response.status.toString()`)
		}

	case sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE:
		cond := assertionCondition(assertion.Condition)
		value := newAssertionValue(assertion.Value)
		cond.Name(&checkJS, assertion.Expression, assertion.Value)
		checkJS.WriteString(`": response => `)
		value.openBody(&checkJS)
		checkJS.WriteString(`jsonpath.query(response.json(), "`)
		checkJS.WriteString(template.JSEscapeString(assertion.Expression))
		checkJS.WriteString(`").some(values => `)

		// A JSONPath result keeps its JSON type, and the variable side is
		// a string. Converting the result too lets a numeric 42 match ${id}.
		subject := `values`
		if len(value.names) > 0 {
			subject = `String(values)`
		}

		value.writeCondition(cond, &checkJS, &failureLog, subject)
		checkJS.WriteString(`)`)
		value.closeBody(&checkJS)

	case sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION:
		checkJS.WriteString(template.JSEscapeString(assertion.Expression))
		checkJS.WriteString(` exists": response => jsonpath.query(response.json(), "`)
		checkJS.WriteString(template.JSEscapeString(assertion.Expression))
		failureLog.WriteString(`JsonPath expression `)
		failureLog.WriteString(assertion.Expression)
		checkJS.WriteString(`").length > 0`)

	case sm.MultiHttpEntryAssertionType_REGEX_ASSERTION:
		switch assertion.Subject {
		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY, sm.MultiHttpEntryAssertionSubjectVariant_DEFAULT_SUBJECT:
			checkJS.WriteString(`body matches /`)
			checkJS.WriteString(template.JSEscapeString(assertion.Expression))
			checkJS.WriteString(`/": response => { const expr = new RegExp("`)
			checkJS.WriteString(template.JSEscapeString(assertion.Expression))
			checkJS.WriteString(`"); `)
			checkJS.WriteString(`return expr.test(response.body); }`)
			failureLog.WriteString("Body matches")
			failureLog.WriteString(assertion.Expression)

		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS:
			checkJS.WriteString(`headers matches /`)
			checkJS.WriteString(template.JSEscapeString(assertion.Expression))
			checkJS.WriteString(`/": response => { const expr = new RegExp("`)
			checkJS.WriteString(template.JSEscapeString(assertion.Expression))
			checkJS.WriteString(`"); `)
			checkJS.WriteString(`const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); `)
			checkJS.WriteString(`return !!values.find(value => expr.test(value)); }`)
			failureLog.WriteString("Headers match")
			failureLog.WriteString(assertion.Expression)

		case sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE:
			checkJS.WriteString(`status matches /`)
			checkJS.WriteString(template.JSEscapeString(assertion.Expression))
			checkJS.WriteString(`/": response => { const expr = new RegExp("`)
			checkJS.WriteString(template.JSEscapeString(assertion.Expression))
			checkJS.WriteString(`"); `)
			checkJS.WriteString(`return expr.test(response.status.toString()); }`)
			failureLog.WriteString("Status matches")
			failureLog.WriteString(assertion.Expression)
		}
	}

	checkJS.WriteString(` }`)
	checkJS.WriteString(`, `)

	// Add tags to the check: url, method
	checkJS.WriteString(`{`)
	checkJS.WriteString(`"url": `)
	checkJS.WriteString(urlVarName)
	checkJS.WriteString(`.toString(), `)
	checkJS.WriteString(`"method": "`)
	checkJS.WriteString(method)
	checkJS.WriteRune('"')
	checkJS.WriteString(`}`)

	checkJS.WriteString(`);`)

	checkJS.WriteString("\n\t")
	checkJS.WriteString(`if(!currentCheck) {`)
	checkJS.WriteString("\n\t\t")
	checkJS.WriteString(`console.error("Assertion failed:", "`)
	checkJS.WriteString(template.JSEscapeString(failureLog.String()))
	checkJS.WriteString(`");`)
	checkJS.WriteString("\n\t\t")
	checkJS.WriteString(`fail()`)
	checkJS.WriteString("\n\t")
	checkJS.WriteString(`};`)
	checkJS.WriteString("\n")

	return checkJS.String()
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

func settingsToScript(settings *sm.MultiHttpSettings) ([]byte, error) {
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
			"interpolateBodyVars": interpolateBodyVariables,
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
