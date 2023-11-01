package multihttp

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	kitlog "github.com/go-kit/kit/log" //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-kit/log/level"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/mccutchen/go-httpbin/v2/httpbin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestBuildQueryParams(t *testing.T) {
	testcases := map[string]struct {
		request  sm.MultiHttpEntryRequest
		expected []string
	}{
		"trivial": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "hello",
					},
				},
			},
			expected: []string{`url.searchParams.append('q', 'hello')`},
		},
		"multiple": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "hello",
					},
					{
						Name:  "w",
						Value: "goodbye",
					},
				},
			},
			expected: []string{`url.searchParams.append('q', 'hello')`, `url.searchParams.append('w', 'goodbye')`},
		},
		"without value": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "",
					},
				},
			},
			expected: []string{`url.searchParams.append('q', '')`},
		},
		"variable in query value": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "${variable}",
					},
				},
			},
			expected: []string{`url.searchParams.append('q', vars['variable'])`},
		},
		"multiple variables in query value": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "${variable1}and${variable2}",
					},
				},
			},
			expected: []string{`url.searchParams.append('q', vars['variable1']+'and'+vars['variable2'])`},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildQueryParams("url", &tc.request)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestBuildUrl(t *testing.T) {
	testcases := map[string]struct {
		request  sm.MultiHttpEntryRequest
		expected string
	}{
		"trivial": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.example.org/",
			},
			expected: `'https://www.example.org/'`,
		},
		"variable in url": {
			request: sm.MultiHttpEntryRequest{
				Url: "${variable}",
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "hello",
					},
				},
			},
			expected: `vars['variable']`,
		},
		"multiple variables in url": {
			request: sm.MultiHttpEntryRequest{
				Url:         "https://www.${variable1}.com/${variable2}",
				QueryFields: []*sm.QueryField{},
			},
			expected: `'https://www.'+vars['variable1']+'.com/'+vars['variable2']`,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := performVariableExpansion(tc.request.Url)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestBuildHeaders(t *testing.T) {
	type input struct {
		headers []*sm.HttpHeader
		body    *sm.HttpRequestBody
	}

	testcases := map[string]struct {
		input    input
		expected string
	}{
		"one header": {
			input: input{
				headers: []*sm.HttpHeader{
					{
						Name:  "Content-Type",
						Value: "application/json",
					},
				},
			},
			expected: `{"Content-Type":'application/json'}`,
		},
		"two headers": {
			input: input{
				headers: []*sm.HttpHeader{
					{
						Name:  "Content-Type",
						Value: "application/json",
					},
					{
						Name:  "Accept",
						Value: "text/html",
					},
				},
			},
			expected: `{"Content-Type":'application/json',"Accept":'text/html'}`,
		},
		"blank value": {
			input: input{
				headers: []*sm.HttpHeader{
					{
						Name:  "Content-Type",
						Value: "",
					},
				},
			},
			expected: `{"Content-Type":''}`,
		},
		"body-content-type+content-encoding": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentType:     "text/plain",
					ContentEncoding: "none",
					Payload:         []byte("test"),
				},
			},
			expected: `{'Content-Type':"text/plain",'Content-Encoding':"none"}`,
		},
		"body-content-type": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentType: "text/plain",
					Payload:     []byte("test"),
				},
			},
			expected: `{'Content-Type':"text/plain"}`,
		},
		"body-content-encoding": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentEncoding: "none",
					Payload:         []byte("test"),
				},
			},
			expected: `{'Content-Encoding':"none"}`,
		},
		"body-content-type+content-encoding+headers": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentType:     "text/plain",
					ContentEncoding: "none",
					Payload:         []byte("test"),
				},
				headers: []*sm.HttpHeader{
					{
						Name:  "X-Some-Header",
						Value: "some value",
					},
				},
			},
			expected: `{'Content-Type':"text/plain",'Content-Encoding':"none","X-Some-Header":'some value'}`,
		},
		"body-content-type+headers": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentType: "text/plain",
					Payload:     []byte("test"),
				},
				headers: []*sm.HttpHeader{
					{
						Name:  "X-Some-Header",
						Value: "some value",
					},
				},
			},
			expected: `{'Content-Type':"text/plain","X-Some-Header":'some value'}`,
		},
		"body-content-encoding+headers": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentEncoding: "none",
					Payload:         []byte("test"),
				},
				headers: []*sm.HttpHeader{
					{
						Name:  "X-Some-Header",
						Value: "some value",
					},
				},
			},
			expected: `{'Content-Encoding':"none","X-Some-Header":'some value'}`,
		},
		"empty": {
			input: input{
				body:    nil,
				headers: nil,
			},
			expected: ``,
		},
		"do what I say": {
			input: input{
				body: &sm.HttpRequestBody{
					ContentType:     "text/plain",
					ContentEncoding: "none",
					Payload:         []byte("test"),
				},
				headers: []*sm.HttpHeader{
					{
						Name:  "Content-Type",
						Value: "application/json",
					},
				},
			},
			expected: `{'Content-Type':"text/plain",'Content-Encoding':"none","Content-Type":'application/json'}`,
		},
		"variable in value": {
			input: input{
				body: nil,
				headers: []*sm.HttpHeader{
					{
						Name:  "Authorization",
						Value: "Bearer ${accessToken}",
					},
				},
			},
			expected: `{"Authorization":'Bearer '+vars['accessToken']}`,
		},
		"multiple variables in value": {
			input: input{
				body: nil,
				headers: []*sm.HttpHeader{
					{
						Name:  "Authorization",
						Value: "Bearer ${accessToken}${andsomeother}",
					},
				},
			},
			expected: `{"Authorization":'Bearer '+vars['accessToken']+vars['andsomeother']}`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildHeaders(testcase.input.headers, testcase.input.body)
			require.Equal(t, testcase.expected, actual)
		})
	}
}

func TestBuildBody(t *testing.T) {
	type input struct {
		body *sm.HttpRequestBody
	}

	testcases := map[string]struct {
		input    input
		expected string
	}{
		"not empty": {
			input:    input{body: &sm.HttpRequestBody{Payload: []byte("test")}},
			expected: `encoding.b64decode("dGVzdA", 'rawstd', "b")`,
		},
		"nil": {
			input:    input{body: nil},
			expected: "null",
		},
		"empty": {
			input:    input{body: &sm.HttpRequestBody{Payload: []byte("")}},
			expected: `""`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildBody(testcase.input.body)
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
			expected:  `subject does not contain \"value\"`,
		},
		"TestAssertionConditionNameContains": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
			subject:   "subject",
			value:     "value",
			expected:  `subject contains \"value\"`,
		},
		"TestAssertionConditionNameEquals": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_EQUALS,
			subject:   "subject",
			value:     "value",
			expected:  `subject equals \"value\"`,
		},
		"TestAssertionConditionNameStartsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH,
			subject:   "subject",
			value:     "value",
			expected:  `subject starts with \"value\"`,
		},
		"TestAssertionConditionNameEndsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH,
			subject:   "subject",
			value:     "value",
			expected:  `subject ends with \"value\"`,
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
			expected:  `!subject.includes("val\'ue")`,
		},
		"TestAssertionConditionRenderContains": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
			subject:   "subject",
			value:     "val'ue",
			expected:  `subject.includes("val\'ue")`,
		},
		"TestAssertionConditionRenderEquals": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_EQUALS,
			subject:   "subject",
			value:     "val'ue",
			expected:  `subject === "val\'ue"`,
		},
		"TestAssertionConditionRenderStartsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_STARTS_WITH,
			subject:   "subject",
			value:     "val'ue",
			expected:  `subject.startsWith("val\'ue")`,
		},
		"TestAssertionConditionRenderEndsWith": {
			condition: sm.MultiHttpEntryAssertionConditionVariant_ENDS_WITH,
			subject:   "subject",
			value:     "val'ue",
			expected:  `subject.endsWith("val\'ue")`,
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
		urlVarName string
		method     string
		assertion  *sm.MultiHttpEntryAssertion
		expected   string
	}{
		"TestBuildChecksTextAssertionWithBodySubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:      sm.MultiHttpEntryAssertionType_TEXT,
				Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:   sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Value:     "value",
			},
			expected: `currentCheck = check(response, { "body contains \"value\"": response => response.body.includes("value") }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "response.body.includes(\"value\")");
		fail()
	};
`,
		},
		"TestBuildChecksTextAssertionWithHeadersSubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:      sm.MultiHttpEntryAssertionType_TEXT,
				Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:   sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
				Value:     "value",
			},
			expected: `currentCheck = check(response, { "header contains \"value\"": response => { const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); return !!values.find(value => value.includes("value")); } }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "value.includes(\"value\")");
		fail()
	};
`,
		},
		"TestBuildChecksTextAssertionWithHeadersSubjectAndExpression": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_TEXT,
				Condition:  sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
				Expression: "Content-Type",
				Value:      "value",
			},
			expected: `currentCheck = check(response, { "header contains \"value\"": response => { return assertHeader(response.headers, "Content-Type", v => value.includes("value")); } }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "value.includes(\"value\")");
		fail()
	};
`,
		},
		"TestBuildChecksTextAssertionWithStatusCodeSubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:      sm.MultiHttpEntryAssertionType_TEXT,
				Condition: sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:   sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
				Value:     "value",
			},
			expected: `currentCheck = check(response, { "status code contains \"value\"": response => response.status.toString().includes("value") }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "response.status.toString().includes(\"value\")");
		fail()
	};
`,
		},
		"TestBuildChecksJsonPathValueAssertionWithBodySubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_VALUE,
				Condition:  sm.MultiHttpEntryAssertionConditionVariant_CONTAINS,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "/path/to/value",
				Value:      "value",
			},
			expected: `currentCheck = check(response, { "/path/to/value contains \"value\"": response => jsonpath.query(response.json(), "/path/to/value").some(values => values.includes("value")) }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "values.includes(\"value\")");
		fail()
	};
`,
		},
		"TestBuildChecksJsonPathAssertionWithBodySubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_JSON_PATH_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "/path/to/value",
			},
			expected: `currentCheck = check(response, { "/path/to/value exists": response => jsonpath.query(response.json(), "/path/to/value").length > 0 }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "JsonPath expression /path/to/value");
		fail()
	};
`,
		},
		"TestBuildChecksRegexAssertionWithBodySubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_BODY,
				Expression: "regex",
			},
			expected: `currentCheck = check(response, { "body matches /regex/": response => { const expr = new RegExp("regex"); return expr.test(response.body); } }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "Body matchesregex");
		fail()
	};
`,
		},
		"TestBuildChecksRegexAssertionWithHeadersSubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS,
				Expression: "regex",
			},
			expected: `currentCheck = check(response, { "headers matches /regex/": response => { const expr = new RegExp("regex"); const values = Object.entries(response.headers).map(header => header[0].toLowerCase() + ': ' + header[1]); return !!values.find(value => expr.test(value)); } }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "Headers matchregex");
		fail()
	};
`,
		},
		"TestBuildChecksRegexAssertionWithStatusCodeSubject": {
			urlVarName: "url",
			method:     "GET",
			assertion: &sm.MultiHttpEntryAssertion{
				Type:       sm.MultiHttpEntryAssertionType_REGEX_ASSERTION,
				Subject:    sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
				Expression: "regex",
			},
			expected: `currentCheck = check(response, { "status matches /regex/": response => { const expr = new RegExp("regex"); return expr.test(response.status.toString()); } }, {"url": url.toString(), "method": "GET"});
	if(!currentCheck) {
		console.error("Assertion failed:", "Status matchesregex");
		fail()
	};
`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildChecks(testcase.urlVarName, testcase.method, testcase.assertion)
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
			expected: `vars['name'] = response.html('cssSelector').html();`,
		},
		"TestBuildVarsCssSelectorWithAttribute": {
			input: sm.MultiHttpEntryVariable{
				Name:       "name",
				Type:       sm.MultiHttpEntryVariableType_CSS_SELECTOR,
				Expression: "cssSelector",
				Attribute:  "attribute",
			},
			expected: `vars['name'] = response.html('cssSelector').first().attr('attribute');`,
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
						Expression: "content-type: .*; charset=utf-8",
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

	check := sm.Check{
		Target:  settings.Entries[0].Request.Url,
		Job:     "test",
		Timeout: 10000,
		Settings: sm.CheckSettings{
			Multihttp: settings,
		},
	}

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)
	// logger := zerolog.New(zerolog.NewTestWriter(t))
	logger := zerolog.Nop()
	k6path := filepath.Join(testhelper.ModuleDir(t), "dist", "k6")
	runner := k6runner.New(k6path)

	prober, err := NewProber(ctx, check, logger, runner)
	require.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	require.NotNil(t, reg)

	var buf bytes.Buffer
	userLogger := level.NewFilter(kitlog.NewLogfmtLogger(&buf), level.AllowInfo(), level.SquelchNoLevel(false))
	require.NotNil(t, userLogger)

	success := prober.Probe(ctx, check.Target, reg, userLogger)

	t.Log("Log entries:\n" + buf.String())

	require.True(t, success)
}

func TestReplaceVariablesInString(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected string
	}{
		"no replacements": {
			input:    "plain string",
			expected: `'plain string'`,
		},
		"one variable": {
			input:    "this is a ${var} to replace",
			expected: `'this is a '+vars['var']+' to replace'`,
		},
		"two variables": {
			input:    "this is ${v1} and ${v2}",
			expected: `'this is '+vars['v1']+' and '+vars['v2']`,
		},
		"multiple instances": {
			input:    "this is ${v1}, ${v2} and ${v1} again",
			expected: `'this is '+vars['v1']+', '+vars['v2']+' and '+vars['v1']+' again'`,
		},
	}

	for name, testcase := range testcases {
		actual := performVariableExpansion(testcase.input)
		require.Equal(t, testcase.expected, actual, name)
	}
}
