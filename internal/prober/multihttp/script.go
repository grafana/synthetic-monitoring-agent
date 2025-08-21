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
var secretVariables = regexp.MustCompile(`\$\{secret\.([a-zA-Z0-9_][a-zA-Z0-9_\.\-]*)\}`)

func performVariableExpansion(in string) string {
	if len(in) == 0 {
		return `''`
	}

	var s strings.Builder
	buf := []byte(in)

	// First handle secret variables
	locs := secretVariables.FindAllSubmatchIndex(buf, -1)
	p := 0

	for _, loc := range locs {
		if len(loc) < 4 {
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

		// Generate async secret lookup
		s.WriteString(`await secrets.get('`)
		s.Write(buf[loc[2]:loc[3]])
		s.WriteString(`')`)

		p = loc[1]
	}

	// Then handle regular variables in the remaining text
	remainingText := buf[p:]
	if len(remainingText) > 0 {
		regularLocs := userVariables.FindAllSubmatchIndex(remainingText, -1)

		if len(regularLocs) > 0 {
			if s.Len() > 0 {
				s.WriteRune('+')
			}

			p2 := 0
			for _, loc := range regularLocs {
				if len(loc) < 4 {
					panic("unexpected result while building URL")
				}

				if s.Len() > 0 {
					s.WriteRune('+')
				}

				if pre := remainingText[p2:loc[0]]; len(pre) > 0 {
					s.WriteRune('\'')
					template.JSEscape(&s, pre)
					s.WriteRune('\'')
					s.WriteRune('+')
				}

				s.WriteString(`vars['`)
				s.Write(remainingText[loc[2]:loc[3]])
				s.WriteString(`']`)

				p2 = loc[1]
			}

			if len(remainingText[p2:]) > 0 {
				if s.Len() > 0 {
					s.WriteRune('+')
				}
				s.WriteRune('\'')
				template.JSEscape(&s, remainingText[p2:])
				s.WriteRune('\'')
			}
		} else {
			// No regular variables, just append the remaining text
			if s.Len() > 0 {
				s.WriteRune('+')
			}
			s.WriteRune('\'')
			template.JSEscape(&s, remainingText)
			s.WriteRune('\'')
		}
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

		// Find both regular and secret variables using submatch indices
		regularMatches := userVariables.FindAllSubmatchIndex(body.Payload, -1)
		secretMatches := secretVariables.FindAllSubmatchIndex(body.Payload, -1)

		parsedMatches := make(map[string]struct{})
		out := make([]string, 0, len(regularMatches)+len(secretMatches))

		// Handle regular variables
		for _, match := range regularMatches {
			if len(match) < 4 {
				continue
			}
			m := string(body.Payload[match[0]:match[1]])
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
			buf.Write(body.Payload[match[2]:match[3]])
			buf.WriteString("'])")
			out = append(out, buf.String())

			parsedMatches[m] = struct{}{}
		}

		// Handle secret variables
		for _, match := range secretMatches {
			if len(match) < 4 {
				continue
			}
			m := string(body.Payload[match[0]:match[1]])
			if _, found := parsedMatches[m]; found {
				continue
			}

			buf.Reset()
			buf.WriteString(bodyVarName)
			buf.WriteString("=")
			buf.WriteString(bodyVarName)
			buf.WriteString(".replaceAll('")
			buf.WriteString(m)
			buf.WriteString("', await secrets.get('")
			// writing the secret name from the capture group
			buf.Write(body.Payload[match[2]:match[3]])
			buf.WriteString("'))")
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

func hasSecretVariables(settings *sm.MultiHttpSettings) bool {
	for _, entry := range settings.Entries {
		// Check URL
		if secretVariables.MatchString(entry.Request.Url) {
			return true
		}

		// Check headers
		for _, header := range entry.Request.Headers {
			if secretVariables.MatchString(header.Value) {
				return true
			}
		}

		// Check query fields
		for _, field := range entry.Request.QueryFields {
			if secretVariables.MatchString(field.Name) || secretVariables.MatchString(field.Value) {
				return true
			}
		}

		// Check body
		if entry.Request.Body != nil && secretVariables.MatchString(string(entry.Request.Body.Payload)) {
			return true
		}
	}
	return false
}

func settingsToScript(settings *sm.MultiHttpSettings) ([]byte, error) {
	// Convert settings to script using a Go template
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"buildBody":           buildBody,
			"buildChecks":         buildChecks,
			"buildHeaders":        buildHeaders,
			"buildUrl":            performVariableExpansion,
			"buildQueryParams":    buildQueryParams,
			"buildVars":           buildVars,
			"interpolateBodyVars": interpolateBodyVariables,
		}).
		ParseFS(templateFS, "*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing script template: %w", err)
	}

	var buf bytes.Buffer

	// Create template data with secret variable detection
	templateData := struct {
		*sm.MultiHttpSettings
		HasSecretVariables bool
	}{
		MultiHttpSettings:  settings,
		HasSecretVariables: hasSecretVariables(settings),
	}

	// TODO(mem): figure out if we need to transform the data in some way
	// before executing the template
	if err := tmpl.ExecuteTemplate(&buf, "script.tmpl", templateData); err != nil {
		return nil, fmt.Errorf("executing script template: %w", err)
	}

	return buf.Bytes(), nil
}
