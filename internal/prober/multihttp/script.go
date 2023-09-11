package multihttp

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// embed script template
//
//go:embed script.tmpl
var templateFS embed.FS

func buildUrl(req *sm.MultiHttpEntryRequest) string {
	// If we are here, the request has already been validated, and the URL
	// should be valid. This function should never return an error.
	u, _ := url.Parse(req.Url)

	var buf strings.Builder

	for _, field := range req.QueryFields {
		if buf.Len() > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(url.QueryEscape(field.Name))
		if len(field.Value) > 0 {
			buf.WriteByte('=')
			buf.WriteString(url.QueryEscape(field.Value))
		}
	}

	u.RawQuery = buf.String()

	return template.JSEscapeString(u.String())
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
		buf.WriteString(`", 'rawstd', "b")`)

		return buf.String()
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
		buf.WriteString(`":"`)
		buf.WriteString(template.JSEscapeString(header.Value))
		buf.WriteRune('"')

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
func buildChecks(url, method string, assertion *sm.MultiHttpEntryAssertion) string {
	var b strings.Builder

	b.WriteString(`check(response, { "`)

	switch assertion.Type {
	case sm.MultiHttpEntryAssertionType_TEXT:
		cond := assertionCondition(assertion.Condition)

		switch assertion.Subject {
		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY, sm.MultiHttpEntryAssertionSubjectVariant_DEFAULT_SUBJECT:
			cond.Name(&b, "body", assertion.Value)
			b.WriteString(`": response => `)
			cond.Render(&b, "response.body", assertion.Value)

		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS:
			cond.Name(&b, "header", assertion.Value)
			b.WriteString(`": response => { `)
			if len(assertion.Expression) == 0 {
				// No expression provided, match the entire value against all headers.
				b.WriteString(`const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); `)
				b.WriteString(`return !!values.find(value => `)
				cond.Render(&b, "value", assertion.Value)
				b.WriteString(`);`)
			} else {
				// Expression provided, search for a matching header.
				b.WriteString(`return assertHeader(response.headers, "`)
				b.WriteString(template.JSEscapeString(assertion.Expression))
				b.WriteString(`", `)
				b.WriteString(`v => `)
				cond.Render(&b, "value", assertion.Value)
				b.WriteString(`);`)
			}
			b.WriteString(` }`)

		case sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE:
			cond.Name(&b, "status code", assertion.Value)
			b.WriteString(`": response => `)
			cond.Render(&b, `response.status.toString()`, assertion.Value)
		}

	case sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE:
		cond := assertionCondition(assertion.Condition)
		cond.Name(&b, assertion.Expression, assertion.Value)
		b.WriteString(`": response => jsonpath.query(response.json(), "`)
		b.WriteString(template.JSEscapeString(assertion.Expression))
		b.WriteString(`").some(values => `)
		cond.Render(&b, `values`, assertion.Value)
		b.WriteString(`)`)

	case sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION:
		b.WriteString(template.JSEscapeString(assertion.Expression))
		b.WriteString(` exists": response => jsonpath.query(response.json(), "`)
		b.WriteString(template.JSEscapeString(assertion.Expression))
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

		case sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS:
			b.WriteString(`headers matches /`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`/": response => { const expr = new RegExp("`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`"); `)
			b.WriteString(`const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); `)
			b.WriteString(`return !!values.find(value => expr.test(value)); }`)

		case sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE:
			b.WriteString(`status matches /`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`/": response => { const expr = new RegExp("`)
			b.WriteString(template.JSEscapeString(assertion.Expression))
			b.WriteString(`"); `)
			b.WriteString(`return expr.test(response.status.toString()); }`)
		}
	}

	b.WriteString(` }`)
	b.WriteString(`, `)

	// Add tags to the check: url, method
	b.WriteString(`{`)
	b.WriteString(`"url": "`)
	b.WriteString(url)
	b.WriteString(`", "method": "`)
	b.WriteString(method)
	b.WriteRune('"')
	b.WriteString(`}`)

	b.WriteString(`);`)

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

func settingsToScript(settings *sm.MultiHttpSettings) ([]byte, error) {
	// Convert settings to script using a Go template
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"buildBody":    buildBody,
			"buildChecks":  buildChecks,
			"buildHeaders": buildHeaders,
			"buildUrl":     buildUrl,
			"buildVars":    buildVars,
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
