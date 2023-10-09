package multihttp

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// embed script template
//
//go:embed script.tmpl
var templateFS embed.FS

var userVariables = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

func performVariableExpansion(in string) string {
	if len(in) == 0 {
		return `''`
	}

	var s strings.Builder
	buf := []byte(in)
	locs := userVariables.FindAllSubmatchIndex(buf, -1)

	p := 0
	for _, loc := range locs {
		if len(loc) < 4 { // put the bounds checker at ease
			panic("unexpected result while building URL")
		}

		if s.Len() > 0 {
			s.WriteRune('+')
		}

		if pre := buf[p:loc[0]]; len(pre) > 0 {
			s.WriteRune('\'')
			template.JSEscape(&s, pre)
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
		template.JSEscape(&s, buf[p:])
		s.WriteRune('\'')
	}

	return s.String()
}

// Query params must be appended to a URL that has already been created.
// urlVarName is the variable name to reference when appending params.
func buildQueryParams(urlVarName string, req *sm.MultiHttpEntryRequest) []string {
	var buf strings.Builder
	out := make([]string, 0, len(req.QueryFields))
	for _, field := range req.QueryFields {
		buf.Reset()
		buf.WriteString(urlVarName)
		buf.WriteString(".searchParams.append(")
		buf.WriteString(performVariableExpansion(field.Name))
		buf.WriteString(", ")
		buf.WriteString(performVariableExpansion(field.Value))
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
		buf.WriteString(`":`)
		buf.WriteString(performVariableExpansion(header.Value))

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
	var b strings.Builder
	var assertionDescriptor strings.Builder
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

func settingsToScript(settings *sm.MultiHttpSettings) ([]byte, error) {
	// Convert settings to script using a Go template
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"buildBody":        buildBody,
			"buildChecks":      buildChecks,
			"buildHeaders":     buildHeaders,
			"buildUrl":         performVariableExpansion,
			"buildQueryParams": buildQueryParams,
			"buildVars":        buildVars,
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
