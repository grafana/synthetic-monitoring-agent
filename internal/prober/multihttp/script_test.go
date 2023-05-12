package multihttp

import (
	"net/http/httptest"
	"strings"
	"testing"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/mccutchen/go-httpbin/v2/httpbin"
	"github.com/stretchr/testify/require"
)

func TestBuildUrl(t *testing.T) {
	testcases := map[string]struct {
		request  sm.MultiHttpEntryRequest
		expected string
	}{
		"trivial": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.example.org/",
			},
			expected: "https://www.example.org/",
		},
		"with query fields": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.example.org/",
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "hello",
					},
				},
			},
			expected: "https://www.example.org/?q\\u003Dhello",
		},
		"query without value": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.example.org/",
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "",
					},
				},
			},
			expected: "https://www.example.org/?q",
		},
		"query needs encoding": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.example.org/",
				QueryFields: []*sm.QueryField{
					{
						Name:  "p&q",
						Value: "a b",
					},
				},
			},
			expected: "https://www.example.org/?p%26q\\u003Da+b",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildUrl(&tc.request)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestBuildHeaders(t *testing.T) {
	testcases := map[string]struct {
		input    []*sm.HttpHeader
		expected string
	}{
		"one header": {
			input: []*sm.HttpHeader{
				{
					Name:  "Content-Type",
					Value: "application/json",
				},
			},
			expected: `{'Content-Type':'application/json'}`,
		},
		"two headers": {
			input: []*sm.HttpHeader{
				{
					Name:  "Content-Type",
					Value: "application/json",
				},
				{
					Name:  "Accept",
					Value: "text/html",
				},
			},
			expected: `{'Content-Type':'application/json','Accept':'text/html'}`,
		},
		"blank value": {
			input: []*sm.HttpHeader{
				{
					Name:  "Content-Type",
					Value: "",
				},
			},
			expected: `{'Content-Type':''}`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildHeaders(testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}

func TestAssertionConditionName(t *testing.T) {
	testcases := map[string]struct {
		condition sm.MultiHttpEntryAssertionConditionVariant
		subject   string
		value     string
		expected  string
	}{
		"TestAssertionConditionNameNotContains": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_NOT_CONTAINS,
			subject:   "subject",
			value:     "value",
			expected:  "subject does not contain \"value\"",
		},
		"TestAssertionConditionNameContains": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
			subject:   "subject",
			value:     "value",
			expected:  "subject contains \"value\"",
		},
		"TestAssertionConditionNameEquals": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_EQUALS,
			subject:   "subject",
			value:     "value",
			expected:  "subject equals \"value\"",
		},
		"TestAssertionConditionNameStartsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH,
			subject:   "subject",
			value:     "value",
			expected:  "subject starts with \"value\"",
		},
		"TestAssertionConditionNameEndsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH,
			subject:   "subject",
			value:     "value",
			expected:  "subject ends with \"value\"",
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			cond := assertionCondition(testcase.condition)
			cond.Name(&b, testcase.subject, testcase.value)
			require.Equal(t, testcase.expected, b.String())
		})
	}
}

func TestAssertionConditionRender(t *testing.T) {
	testcases := map[string]struct {
		condition sm.MultiHttpEntryAssertionConditionVariant
		subject   string
		value     string
		expected  string
	}{
		"TestAssertionConditionRenderNotContains": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_NOT_CONTAINS,
			subject:   "subject",
			value:     "val'ue",
			expected:  "!subject.includes('val\\'ue')",
		},
		"TestAssertionConditionRenderContains": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
			subject:   "subject",
			value:     "val'ue",
			expected:  "subject.includes('val\\'ue')",
		},
		"TestAssertionConditionRenderEquals": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_EQUALS,
			subject:   "subject",
			value:     "val'ue",
			expected:  "subject === 'val\\'ue'",
		},
		"TestAssertionConditionRenderStartsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH,
			subject:   "subject",
			value:     "val'ue",
			expected:  "subject.startsWith('val\\'ue')",
		},
		"TestAssertionConditionRenderEndsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH,
			subject:   "subject",
			value:     "val'ue",
			expected:  "subject.endsWith('val\\'ue')",
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			cond := assertionCondition(testcase.condition)
			cond.Render(&b, testcase.subject, testcase.value)
			require.Equal(t, testcase.expected, b.String())
		})
	}
}

func TestBuildChecks(t *testing.T) {
	testcases := map[string]struct {
		assertion *sm.MultiHttpEntryAssertion
		expected  string
	}{
		"TestBuildChecksTextAssertionWithBodySubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:      sm.MultiHttpEntryAssertionType_TEXT,
				Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:   sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Value:     "value",
			},
			expected: `check(response, { 'body contains "value"': response => response.body.includes('value') });`,
		},
		"TestBuildChecksTextAssertionWithHeadersSubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:      sm.MultiHttpEntryAssertionType_TEXT,
				Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:   sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
				Value:     "value",
			},
			expected: `check(response, { 'header contains "value"': response => { const values = Object.entries(response.headers).map((key, value) => key + ': ' + value); return !!values.find(value => value.includes('value')); } });`,
		},
		"TestBuildChecksTextAssertionWithStatusCodeSubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:      sm.MultiHttpEntryAssertionType_TEXT,
				Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:   sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
				Value:     "value",
			},
			expected: `check(response, { 'status code contains "value"': response => response.status.toString().includes('value') });`,
		},
		"TestBuildChecksJsonPathValueAssertionWithBodySubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
				Condition:  sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "/path/to/value",
				Value:      "value",
			},
			expected: `check(response, { '/path/to/value contains "value"': response => jsonpath.query(response.json(), '/path/to/value').some(values => values.includes('value')) });`,
		},
		"TestBuildChecksJsonPathAssertionWithBodySubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "/path/to/value",
			},
			expected: `check(response, { '/path/to/value exists': response => jsonpath.query(response.json(), '/path/to/value').length > 0 });`,
		},
		"TestBuildChecksRegexAssertionWithBodySubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "regex",
			},
			expected: `check(response, { 'body matches /regex/': response => { const expr = new RegExp('regex'); return expr.test(response.body); } });`,
		},
		"TestBuildChecksRegexAssertionWithHeadersSubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
				Expression: "regex",
			},
			expected: `check(response, { 'headers matches /regex/': response => { const expr = new RegExp('regex'); const values = Object.entries(response.headers).map((key, value) => key + ': ' + value); return !!values.find(value => expr.test(value)); } });`,
		},
		"TestBuildChecksRegexAssertionWithStatusCodeSubject": {
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
				Expression: "regex",
			},
			expected: `check(response, { 'status matches /regex/': response => { const expr = new RegExp('regex'); return expr.test(response.status.toString()); } });`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildChecks(testcase.assertion)
			require.Equal(t, testcase.expected, actual)
		})
	}
}

func TestBuildVars(t *testing.T) {
	testcases := map[string]struct {
		input    sm.MultiHttpEntryVariable
		expected string
	}{
		"TestBuildVarsJsonPath": {
			input: sm.MultiHttpEntryVariable{
				Name:       "name",
				Type:       sm.MultiHttpEntryVariableType_JSON_PATH,
				Expression: "jsonPath",
			},
			expected: `vars['name'] = jsonpath.query(response.json(), 'jsonPath')[0];`,
		},
		"TestBuildVarsRegex": {
			input: sm.MultiHttpEntryVariable{
				Name:       "name",
				Type:       sm.MultiHttpEntryVariableType_REGEX,
				Expression: "regex",
			},
			expected: `match = new RegExp('regex').exec(response.body); vars['name'] = match ? match[1] || match[0] : null;`,
		},
		"TestBuildVarsCssSelector": {
			input: sm.MultiHttpEntryVariable{
				Name:       "name",
				Type:       sm.MultiHttpEntryVariableType_CSS_SELECTOR,
				Expression: "cssSelector",
			},
			expected: `vars['name'] = response.html().find('cssSelector').html();`,
		},
		"TestBuildVarsCssSelectorWithAttribute": {
			input: sm.MultiHttpEntryVariable{
				Name:       "name",
				Type:       sm.MultiHttpEntryVariableType_CSS_SELECTOR,
				Expression: "cssSelector",
				Attribute:  "attribute",
			},
			expected: `vars['name'] = response.html().find('cssSelector').first().attr('attribute');`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildVars(&testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}

// TestSettingsToScript tests the conversion of a MultiHttpSettings to a
// Javascript script.
func TestSettingsToScript(t *testing.T) {
	testServer := httptest.NewServer(httpbin.New())
	t.Cleanup(testServer.Close)

	settings := &sm.MultiHttpSettings{
		Entries: []*sm.MultiHttpEntry{
			{
				Request: &sm.MultiHttpEntryRequest{
					Method: sm.HttpMethod_GET,
					Url:    testServer.URL + "/response-headers?foo=bar",
				},
				Assertions: []*sm.MultiHttpEntryAssertion{
					{
						Type:      sm.MultiHttpEntryAssertionType_TEXT,
						Subject:   sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
						Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
						Value:     "httpbin",
					},
					{
						Type:      sm.MultiHttpEntryAssertionType_TEXT,
						Subject:   sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
						Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
						Value:     "foo: bar",
					},
				},
			},
			{
				Request: &sm.MultiHttpEntryRequest{
					Method: sm.HttpMethod_GET,
					Url:    testServer.URL + "/status/266",
				},
				Assertions: []*sm.MultiHttpEntryAssertion{
					{
						Type:      sm.MultiHttpEntryAssertionType_TEXT,
						Subject:   sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
						Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
						Value:     "266",
					},
				},
			},
			{
				Request: &sm.MultiHttpEntryRequest{
					Method: sm.HttpMethod_GET,
					Url:    testServer.URL + "/json",
				},
				Assertions: []*sm.MultiHttpEntryAssertion{
					{
						Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
						Expression: "$.slideshow.author",
						Condition:  sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
						Value:      "Yours",
					},
					{
						Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
						Expression: "$.slideshow.date",
						Condition:  sm.MultiHttpEntryAssertionConditionVariant_NOT_CONTAINS,
						Value:      "2023",
					},
					{
						Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
						Expression: "$.slideshow.author",
						Condition:  sm.MultiHttpEntryAssertionConditionVariant_EQUALS,
						Value:      "Yours Truly",
					},
					{
						Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
						Expression: "$.slideshow.author",
						Condition:  sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH,
						Value:      "Yours",
					},
					{
						Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
						Expression: "$.slideshow.author",
						Condition:  sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH,
						Value:      "Truly",
					},
				},
				Variables: []*sm.MultiHttpEntryVariable{
					{
						Type:       sm.MultiHttpEntryVariableType_JSON_PATH,
						Name:       "author",
						Expression: "$.slideshow.author",
					},
					{
						Type:       sm.MultiHttpEntryVariableType_JSON_PATH,
						Name:       "date",
						Expression: "$.slideshow.date",
					},
				},
			},
			{
				Request: &sm.MultiHttpEntryRequest{
					Method: sm.HttpMethod_GET,
					Url:    testServer.URL + "/json",
				},
				Assertions: []*sm.MultiHttpEntryAssertion{
					{
						Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
						Expression: `$.slideshow.title`,
					},
				},
			},
			{
				Request: &sm.MultiHttpEntryRequest{
					Method: sm.HttpMethod_GET,
					Url:    testServer.URL + "/html",
				},
				Assertions: []*sm.MultiHttpEntryAssertion{
					{
						Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
						Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
						Expression: "had .+ excited the curiosity of the mariners",
					},
					{
						Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
						Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
						Expression: "Content-Type: .*; charset=utf-8",
					},
					{
						Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
						Subject:    sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
						Expression: "2..",
					},
				},
			},
			{
				Request: &sm.MultiHttpEntryRequest{
					Method: sm.HttpMethod_GET,
					Url:    testServer.URL + "/get",
					QueryFields: []*sm.QueryField{
						{Name: "foo", Value: "bar"},
						{Name: "baz", Value: ""},
					},
				},
			},
			{
				Request: &sm.MultiHttpEntryRequest{
					Url: testServer.URL + "/gzip",
					Headers: []*sm.HttpHeader{
						{Name: "Accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
						{Name: "Accept-Encoding", Value: "gzip, deflate, br"},
					},
				},
			},
		},
	}

	require.NoError(t, settings.Validate())

	actual, err := settingsToScript(settings)
	require.NoError(t, err)
	require.NotEmpty(t, actual)
}
