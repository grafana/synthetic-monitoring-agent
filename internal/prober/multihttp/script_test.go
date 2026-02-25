package multihttp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kitlog "github.com/go-kit/kit/log" //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-kit/log/level"
	"github.com/grafana/synthetic-monitoring-agent/internal/k6runner"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/prober/interpolation"
	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/mccutchen/go-httpbin/v2/httpbin"
	"github.com/prometheus/client_golang/prometheus"
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

func TestBuildQueryParamsWithSecretVariables(t *testing.T) {
	testcases := map[string]struct {
		request  sm.MultiHttpEntryRequest
		expected []string
	}{
		"secret variable in query name": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "${secrets.api_key}",
						Value: "value",
					},
				},
			},
			expected: []string{`url.searchParams.append(await secrets.get('api_key'), 'value')`},
		},
		"secret variable in query value": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "key",
						Value: "${secrets.api_secret}",
					},
				},
			},
			expected: []string{`url.searchParams.append('key', await secrets.get('api_secret'))`},
		},
		"secret variable in both query name and value": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "${secrets.param_name}",
						Value: "${secrets.param_value}",
					},
				},
			},
			expected: []string{`url.searchParams.append(await secrets.get('param_name'), await secrets.get('param_value'))`},
		},
		"mixed regular and secret variables": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "${variable}",
						Value: "${secrets.api_key}",
					},
				},
			},
			expected: []string{`url.searchParams.append(vars['variable'], await secrets.get('api_key'))`},
		},
		"multiple secret variables in query value": {
			request: sm.MultiHttpEntryRequest{
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "${secrets.prefix}and${secrets.suffix}",
					},
				},
			},
			expected: []string{`url.searchParams.append('q', await secrets.get('prefix')+'and'+await secrets.get('suffix'))`},
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

func TestBuildUrlWithSecretVariables(t *testing.T) {
	testcases := map[string]struct {
		request  sm.MultiHttpEntryRequest
		expected string
	}{
		"secret variable in url": {
			request: sm.MultiHttpEntryRequest{
				Url: "${secrets.api_endpoint}",
			},
			expected: `await secrets.get('api_endpoint')`,
		},
		"multiple secret variables in url": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.${secrets.domain}.com/${secrets.path}",
			},
			expected: `'https://www.'+await secrets.get('domain')+'.com/'+await secrets.get('path')`,
		},
		"mixed regular and secret variables in url": {
			request: sm.MultiHttpEntryRequest{
				Url: "https://www.${variable}.com/${secrets.api_path}",
			},
			expected: `'https://www.${variable}.com/'+await secrets.get('api_path')`,
		},
		"secret variable with query params": {
			request: sm.MultiHttpEntryRequest{
				Url: "${secrets.base_url}",
				QueryFields: []*sm.QueryField{
					{
						Name:  "q",
						Value: "hello",
					},
				},
			},
			expected: `await secrets.get('base_url')`,
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

func TestBuildHeadersWithSecretVariables(t *testing.T) {
	testcases := map[string]struct {
		headers  []*sm.HttpHeader
		body     *sm.HttpRequestBody
		expected string
	}{
		"secret variable in header value": {
			headers: []*sm.HttpHeader{
				{
					Name:  "Authorization",
					Value: "Bearer ${secrets.auth_token}",
				},
			},
			expected: `{"Authorization":'Bearer '+await secrets.get('auth_token')}`,
		},
		"secret variable in header name": {
			headers: []*sm.HttpHeader{
				{
					Name:  "${secrets.header_name}",
					Value: "value",
				},
			},
			expected: `{"${secrets.header_name}":'value'}`,
		},
		"mixed regular and secret variables in headers": {
			headers: []*sm.HttpHeader{
				{
					Name:  "X-API-Key",
					Value: "${secrets.api_key}",
				},
				{
					Name:  "X-User-ID",
					Value: "${user_id}",
				},
			},
			expected: `{"X-API-Key":await secrets.get('api_key'),"X-User-ID":vars['user_id']}`,
		},
		"secret variable with content type": {
			headers: []*sm.HttpHeader{
				{
					Name:  "Authorization",
					Value: "${secrets.token}",
				},
			},
			body: &sm.HttpRequestBody{
				ContentType: "application/json",
			},
			expected: `{'Content-Type':"application/json","Authorization":await secrets.get('token')}`,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := buildHeaders(tc.headers, tc.body)
			require.Equal(t, tc.expected, actual)
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
			expected: `encoding.b64decode("dGVzdA", 'rawstd', "s")`,
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

func TestInterpolateBodyVariables(t *testing.T) {
	t.Parallel()

	type input struct {
		body *sm.HttpRequestBody
	}

	testcases := map[string]struct {
		input    input
		expected []string
	}{
		"no variables": {
			input:    input{body: &sm.HttpRequestBody{Payload: []byte("test")}},
			expected: []string{},
		},
		"basic": {
			input: input{body: &sm.HttpRequestBody{Payload: []byte("test ${variable1}")}},
			expected: []string{
				"body=body.replaceAll('${variable1}', vars['variable1'])",
			},
		},
		"several variables with repeats": {
			input: input{body: &sm.HttpRequestBody{Payload: []byte("${variable1} is ${variable1} fun ${variable2} ok ${variable3}")}},
			expected: []string{
				"body=body.replaceAll('${variable1}', vars['variable1'])",
				"body=body.replaceAll('${variable2}', vars['variable2'])",
				"body=body.replaceAll('${variable3}', vars['variable3'])",
			},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			actual := interpolateBodyVariables("body", tc.input.body)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestInterpolateBodyVariablesWithSecrets(t *testing.T) {
	testcases := map[string]struct {
		bodyVarName string
		body        *sm.HttpRequestBody
		expected    []string
	}{
		"secret variable in body": {
			bodyVarName: "body",
			body: &sm.HttpRequestBody{
				Payload: []byte(`{"password": "${secrets.user_password}"}`),
			},
			expected: []string{`body=body.replaceAll('${secrets.user_password}', await secrets.get('user_password'))`},
		},
		"multiple secret variables in body": {
			bodyVarName: "body",
			body: &sm.HttpRequestBody{
				Payload: []byte(`{"username": "${secrets.username}", "password": "${secrets.password}"}`),
			},
			expected: []string{
				`body=body.replaceAll('${secrets.username}', await secrets.get('username'))`,
				`body=body.replaceAll('${secrets.password}', await secrets.get('password'))`,
			},
		},
		"mixed regular and secret variables in body": {
			bodyVarName: "body",
			body: &sm.HttpRequestBody{
				Payload: []byte(`{"user": "${user_id}", "token": "${secrets.auth_token}"}`),
			},
			expected: []string{
				`body=body.replaceAll('${user_id}', vars['user_id'])`,
				`body=body.replaceAll('${secrets.auth_token}', await secrets.get('auth_token'))`,
			},
		},
		"duplicate secret variables in body": {
			bodyVarName: "body",
			body: &sm.HttpRequestBody{
				Payload: []byte(`{"token1": "${secrets.token}", "token2": "${secrets.token}"}`),
			},
			expected: []string{`body=body.replaceAll('${secrets.token}', await secrets.get('token'))`},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := interpolateBodyVariables(tc.bodyVarName, tc.body)
			require.Equal(t, tc.expected, actual)
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
		LogResponses: true,
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

	check := model.Check{
		Check: sm.Check{
			Target:  settings.Entries[0].Request.Url,
			Job:     "test",
			Timeout: 10000,
			Settings: sm.CheckSettings{
				Multihttp: settings,
			},
		},
	}

	ctx, cancel := testhelper.Context(context.Background(), t)
	t.Cleanup(cancel)

	k6path := testhelper.K6Path(t)
	store := testhelper.NoopSecretStore{}
	runner, err := k6runner.New(k6runner.RunnerOpts{Uri: k6path})
	require.NoError(t, err)

	logger := testhelper.Logger(t)
	prober, err := NewProber(ctx, check, logger, runner, http.Header{}, &store)
	require.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	require.NotNil(t, reg)

	var buf bytes.Buffer
	userLogger := level.NewFilter(kitlog.NewLogfmtLogger(&buf), level.AllowInfo(), level.SquelchNoLevel(false))
	require.NotNil(t, userLogger)

	success, duration := prober.Probe(ctx, check.Target, reg, userLogger)

	t.Log("Log entries:\n" + buf.String())

	require.True(t, success)
	require.NotEqual(t, 0, duration)
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

func TestHasSecretVariables(t *testing.T) {
	testcases := map[string]struct {
		settings *sm.MultiHttpSettings
		expected bool
	}{
		"no secret variables": {
			settings: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "https://example.com",
							Headers: []*sm.HttpHeader{
								{Name: "Content-Type", Value: "application/json"},
							},
						},
					},
				},
			},
			expected: false,
		},
		"secret variable in URL": {
			settings: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "${secrets.api_endpoint}",
						},
					},
				},
			},
			expected: true,
		},
		"secret variable in header": {
			settings: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "https://example.com",
							Headers: []*sm.HttpHeader{
								{Name: "Authorization", Value: "Bearer ${secrets.token}"},
							},
						},
					},
				},
			},
			expected: true,
		},
		"secret variable in query field": {
			settings: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "https://example.com",
							QueryFields: []*sm.QueryField{
								{Name: "key", Value: "${secrets.api_key}"},
							},
						},
					},
				},
			},
			expected: true,
		},
		"secret variable in body": {
			settings: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "https://example.com",
							Body: &sm.HttpRequestBody{
								Payload: []byte(`{"password": "${secrets.user_password}"}`),
							},
						},
					},
				},
			},
			expected: true,
		},
		"secret variable in second entry": {
			settings: &sm.MultiHttpSettings{
				Entries: []*sm.MultiHttpEntry{
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "https://example.com",
						},
					},
					{
						Request: &sm.MultiHttpEntryRequest{
							Url: "${secrets.api_url}",
						},
					},
				},
			},
			expected: true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := hasSecretVariables(tc.settings)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestSecretVariableEdgeCases(t *testing.T) {
	testcases := map[string]struct {
		input    string
		expected string
	}{
		"empty string": {
			input:    "",
			expected: `''`,
		},
		"only secret variable": {
			input:    "${secrets.key}",
			expected: `await secrets.get('key')`,
		},
		"secret variable with special characters": {
			input:    "${secrets.api-key_123}",
			expected: `await secrets.get('api-key_123')`,
		},
		"secret variable with periods": {
			input:    "${secrets.api.v1.key}",
			expected: `await secrets.get('api.v1.key')`,
		},
		"mixed content with secret variable": {
			input:    "prefix${secrets.key}suffix",
			expected: `'prefix'+await secrets.get('key')+'suffix'`,
		},
		"multiple secret variables": {
			input:    "${secrets.key1}${secrets.key2}",
			expected: `await secrets.get('key1')+await secrets.get('key2')`,
		},
		"secret variable with escaped content": {
			input:    "https://example.com/${secrets.path}?q=test",
			expected: `'https://example.com/'+await secrets.get('path')+'?q\u003Dtest'`,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := performVariableExpansion(tc.input)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestSecretNameFormatValidation(t *testing.T) {
	testcases := map[string]struct {
		input       string
		expected    string
		shouldMatch bool
	}{
		// Valid formats - should match and generate correct output
		"simple lowercase": {
			input:       "${secrets.key}",
			expected:    `await secrets.get('key')`,
			shouldMatch: true,
		},
		"with uppercase": {
			input:       "${secrets.API_KEY}",
			expected:    `await secrets.get('API_KEY')`,
			shouldMatch: true,
		},
		"with numbers": {
			input:       "${secrets.key123}",
			expected:    `await secrets.get('key123')`,
			shouldMatch: true,
		},
		"with dashes": {
			input:       "${secrets.api-key}",
			expected:    `await secrets.get('api-key')`,
			shouldMatch: true,
		},
		"with periods": {
			input:       "${secrets.api.v1.key}",
			expected:    `await secrets.get('api.v1.key')`,
			shouldMatch: true,
		},
		"with underscores": {
			input:       "${secrets.api_key}",
			expected:    `await secrets.get('api_key')`,
			shouldMatch: true,
		},
		"mixed characters": {
			input:       "${secrets.api-v1.key_123}",
			expected:    `await secrets.get('api-v1.key_123')`,
			shouldMatch: true,
		},
		"starts with number": {
			input:       "${secrets.123key}",
			expected:    `await secrets.get('123key')`,
			shouldMatch: true,
		},
		"single character": {
			input:       "${secrets.a}",
			expected:    `await secrets.get('a')`,
			shouldMatch: true,
		},
		"single number": {
			input:       "${secrets.1}",
			expected:    `await secrets.get('1')`,
			shouldMatch: true,
		},
		"multiple periods": {
			input:       "${secrets.a.b.c.d}",
			expected:    `await secrets.get('a.b.c.d')`,
			shouldMatch: true,
		},
		"consecutive dashes": {
			input:       "${secrets.api--key}",
			expected:    `await secrets.get('api--key')`,
			shouldMatch: true,
		},
		"consecutive periods": {
			input:       "${secrets.api..key}",
			expected:    `await secrets.get('api..key')`,
			shouldMatch: true,
		},
		"consecutive underscores": {
			input:       "${secrets.api__key}",
			expected:    `await secrets.get('api__key')`,
			shouldMatch: true,
		},
		"starts with dash": {
			input:       "${secrets.-key}",
			expected:    `'${secrets.-key}'`,
			shouldMatch: false,
		},
		"starts with period": {
			input:       "${secrets..key}",
			expected:    `'${secrets..key}'`,
			shouldMatch: false,
		},
		"starts with underscore": {
			input:       "${secrets._key}",
			expected:    `await secrets.get('_key')`,
			shouldMatch: true,
		},
		"ends with dash": {
			input:       "${secrets.key-}",
			expected:    `await secrets.get('key-')`,
			shouldMatch: true,
		},
		"ends with period": {
			input:       "${secrets.key.}",
			expected:    `await secrets.get('key.')`,
			shouldMatch: true,
		},
		"ends with underscore": {
			input:       "${secrets.key_}",
			expected:    `await secrets.get('key_')`,
			shouldMatch: true,
		},
		"all dashes": {
			input:       "${secrets.---}",
			expected:    `'${secrets.---}'`,
			shouldMatch: false,
		},
		"all periods": {
			input:       "${secrets...}",
			expected:    `'${secrets...}'`,
			shouldMatch: false,
		},
		"all underscores": {
			input:       "${secrets.___}",
			expected:    `await secrets.get('___')`,
			shouldMatch: true,
		},
		"very long name": {
			input:       "${secrets.this-is-a-very-long-secret-name-with-many-characters-and-numbers-123456789}",
			expected:    `await secrets.get('this-is-a-very-long-secret-name-with-many-characters-and-numbers-123456789')`,
			shouldMatch: true,
		},
		"complex nested structure": {
			input:       "${secrets.prod.api.v1.user.auth.token}",
			expected:    `await secrets.get('prod.api.v1.user.auth.token')`,
			shouldMatch: true,
		},

		// Invalid formats - should not match and return literal string
		"invalid character space": {
			input:       "${secrets.key name}",
			expected:    `'${secrets.key name}'`,
			shouldMatch: false,
		},
		"invalid character at": {
			input:       "${secrets.key@name}",
			expected:    `'${secrets.key@name}'`,
			shouldMatch: false,
		},
		"invalid character hash": {
			input:       "${secrets.key#name}",
			expected:    `'${secrets.key#name}'`,
			shouldMatch: false,
		},
		"invalid character dollar": {
			input:       "${secrets.key$name}",
			expected:    `'${secrets.key$name}'`,
			shouldMatch: false,
		},
		"invalid character percent": {
			input:       "${secrets.key%name}",
			expected:    `'${secrets.key%name}'`,
			shouldMatch: false,
		},
		"invalid character caret": {
			input:       "${secrets.key^name}",
			expected:    `'${secrets.key^name}'`,
			shouldMatch: false,
		},
		"invalid character ampersand": {
			input:       "${secrets.key&name}",
			expected:    `'${secrets.key\u0026name}'`,
			shouldMatch: false,
		},
		"invalid character asterisk": {
			input:       "${secrets.key*name}",
			expected:    `'${secrets.key*name}'`,
			shouldMatch: false,
		},
		"invalid character parentheses": {
			input:       "${secrets.key(name)}",
			expected:    `'${secrets.key(name)}'`,
			shouldMatch: false,
		},
		"invalid character brackets": {
			input:       "${secrets.key[name]}",
			expected:    `'${secrets.key[name]}'`,
			shouldMatch: false,
		},
		"invalid character braces": {
			input:       "${secrets.key{name}}",
			expected:    `'${secrets.key{name}}'`,
			shouldMatch: false,
		},
		"invalid character pipe": {
			input:       "${secrets.key|name}",
			expected:    `'${secrets.key|name}'`,
			shouldMatch: false,
		},
		"invalid character backslash": {
			input:       "${secrets.key\\name}",
			expected:    `'${secrets.key\\name}'`,
			shouldMatch: false,
		},
		"invalid character forward slash": {
			input:       "${secrets.key/name}",
			expected:    `'${secrets.key/name}'`,
			shouldMatch: false,
		},
		"invalid character semicolon": {
			input:       "${secrets.key;name}",
			expected:    `'${secrets.key;name}'`,
			shouldMatch: false,
		},
		"invalid character colon": {
			input:       "${secrets.key:name}",
			expected:    `'${secrets.key:name}'`,
			shouldMatch: false,
		},
		"invalid character quote": {
			input:       "${secrets.key\"name}",
			expected:    `'${secrets.key\"name}'`,
			shouldMatch: false,
		},
		"invalid character single quote": {
			input:       "${secrets.key'name}",
			expected:    `'${secrets.key\'name}'`,
			shouldMatch: false,
		},
		"invalid character comma": {
			input:       "${secrets.key,name}",
			expected:    `'${secrets.key,name}'`,
			shouldMatch: false,
		},
		"invalid character question mark": {
			input:       "${secrets.key?name}",
			expected:    `'${secrets.key?name}'`,
			shouldMatch: false,
		},
		"invalid character exclamation": {
			input:       "${secrets.key!name}",
			expected:    `'${secrets.key!name}'`,
			shouldMatch: false,
		},
		"invalid character tilde": {
			input:       "${secrets.key~name}",
			expected:    `'${secrets.key~name}'`,
			shouldMatch: false,
		},
		"invalid character backtick": {
			input:       "${secrets.key`name}",
			expected:    `'${secrets.key` + "`" + `name}'`,
			shouldMatch: false,
		},
		"invalid character plus": {
			input:       "${secrets.key+name}",
			expected:    `'${secrets.key+name}'`,
			shouldMatch: false,
		},
		"invalid character equals": {
			input:       "${secrets.key=name}",
			expected:    `'${secrets.key\u003Dname}'`,
			shouldMatch: false,
		},
		"invalid character less than": {
			input:       "${secrets.key<name}",
			expected:    `'${secrets.key\u003Cname}'`,
			shouldMatch: false,
		},
		"invalid character greater than": {
			input:       "${secrets.key>name}",
			expected:    `'${secrets.key\u003Ename}'`,
			shouldMatch: false,
		},
		"empty secret name": {
			input:       "${secrets.}",
			expected:    `'${secrets.}'`,
			shouldMatch: false,
		},
		"missing secret prefix": {
			input:       "${key}",
			expected:    `vars['key']`,
			shouldMatch: false,
		},
		"wrong prefix": {
			input:       "${env.key}",
			expected:    `'${env.key}'`,
			shouldMatch: false,
		},
		"malformed syntax": {
			input:       "${secretkey}",
			expected:    `vars['secretkey']`,
			shouldMatch: false,
		},
		"malformed syntax with space": {
			input:       "${secrets key}",
			expected:    `'${secrets key}'`,
			shouldMatch: false,
		},
		"malformed syntax with colon": {
			input:       "${secrets:key}",
			expected:    `'${secrets:key}'`,
			shouldMatch: false,
		},
		"malformed syntax with slash": {
			input:       "${secrets/key}",
			expected:    `'${secrets/key}'`,
			shouldMatch: false,
		},
		"unicode characters": {
			input:       "${secrets.kéy}",
			expected:    `'${secrets.kéy}'`,
			shouldMatch: false,
		},
		"emoji characters": {
			input:       "${secrets.key🔑}",
			expected:    `'${secrets.key🔑}'`,
			shouldMatch: false,
		},
		"control characters": {
			input:       "${secrets.key\x00name}",
			expected:    `'${secrets.key\u0000name}'`,
			shouldMatch: false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := performVariableExpansion(tc.input)
			require.Equal(t, tc.expected, actual)

			// Additional validation: check if the regex actually matches
			matches := interpolation.SecretRegex.MatchString(tc.input)
			require.Equal(t, tc.shouldMatch, matches,
				"Regex match result doesn't match expected. Input: %s, Expected match: %v, Actual match: %v",
				tc.input, tc.shouldMatch, matches)
		})
	}
}

func TestSecretNameFormatInContexts(t *testing.T) {
	testcases := map[string]struct {
		context    string
		secretName string
		expected   string
	}{
		"in URL": {
			context:    "url",
			secretName: "api.v1.endpoint",
			expected:   `await secrets.get('api.v1.endpoint')`,
		},
		"in header value": {
			context:    "header",
			secretName: "auth-token-123",
			expected:   `{"Authorization":await secrets.get('auth-token-123')}`,
		},
		"in query parameter": {
			context:    "query",
			secretName: "api_key.prod",
			expected:   `url.searchParams.append('key', await secrets.get('api_key.prod'))`,
		},
		"in request body": {
			context:    "body",
			secretName: "user.password.encrypted",
			expected:   `body=body.replaceAll('${secrets.user.password.encrypted}', await secrets.get('user.password.encrypted'))`,
		},
		"mixed with regular variables": {
			context:    "mixed",
			secretName: "prod.api.v1.key",
			expected:   `'https://api.${env}.com/'+await secrets.get('prod.api.v1.key')`,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			var actual string
			switch tc.context {
			case "url":
				actual = performVariableExpansion("${secrets." + tc.secretName + "}")
			case "header":
				headers := []*sm.HttpHeader{
					{Name: "Authorization", Value: "${secrets." + tc.secretName + "}"},
				}
				actual = buildHeaders(headers, nil)
			case "query":
				req := sm.MultiHttpEntryRequest{
					QueryFields: []*sm.QueryField{
						{Name: "key", Value: "${secrets." + tc.secretName + "}"},
					},
				}
				result := buildQueryParams("url", &req)
				actual = result[0]
			case "body":
				body := &sm.HttpRequestBody{
					Payload: []byte(`{"password": "${secrets.` + tc.secretName + `}"}`),
				}
				result := interpolateBodyVariables("body", body)
				actual = result[0]
			case "mixed":
				actual = performVariableExpansion("https://api.${env}.com/${secrets." + tc.secretName + "}")
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestSecretNameFormatEdgeCases(t *testing.T) {
	testcases := map[string]struct {
		input       string
		expected    string
		description string
	}{
		"zero length name": {
			input:       "${secrets.}",
			expected:    `'${secrets.}'`,
			description: "Empty secret name should not match",
		},
		"single dot": {
			input:       "${secrets..}",
			expected:    `'${secrets..}'`,
			description: "Single dot should not match as it doesn't start with a valid character",
		},
		"single dash": {
			input:       "${secrets.-}",
			expected:    `'${secrets.-}'`,
			description: "Single dash should not match as it doesn't start with a valid character",
		},
		"single underscore": {
			input:       "${secrets._}",
			expected:    `await secrets.get('_')`,
			description: "Single underscore should match as it starts with a valid character",
		},
		"leading and trailing dots": {
			input:       "${secrets..key..}",
			expected:    `'${secrets..key..}'`,
			description: "Leading and trailing dots should not match as it doesn't start with a valid character",
		},
		"leading and trailing dashes": {
			input:       "${secrets.--key--}",
			expected:    `'${secrets.--key--}'`,
			description: "Leading and trailing dashes should not match as it doesn't start with a valid character",
		},
		"leading and trailing underscores": {
			input:       "${secrets.__key__}",
			expected:    `await secrets.get('__key__')`,
			description: "Leading and trailing underscores should be preserved",
		},
		"alternating special characters": {
			input:       "${secrets.a-b_c.d-e_f}",
			expected:    `await secrets.get('a-b_c.d-e_f')`,
			description: "Alternating special characters should be preserved",
		},
		"consecutive special characters": {
			input:       "${secrets.a--b__c..d}",
			expected:    `await secrets.get('a--b__c..d')`,
			description: "Consecutive special characters should be preserved",
		},
		"numbers only": {
			input:       "${secrets.123456789}",
			expected:    `await secrets.get('123456789')`,
			description: "Numbers only should be valid",
		},
		"mixed case with numbers": {
			input:       "${secrets.ApiKey123}",
			expected:    `await secrets.get('ApiKey123')`,
			description: "Mixed case with numbers should be valid",
		},
		"complex nested structure": {
			input:       "${secrets.prod.api.v1.user.auth.token.encrypted}",
			expected:    `await secrets.get('prod.api.v1.user.auth.token.encrypted')`,
			description: "Complex nested structure should be valid",
		},
		"very long name with all valid characters": {
			input:       "${secrets.this-is-a-very-long-secret-name-with-many-characters-and-numbers-123456789-and-underscores_and_periods.and.more}",
			expected:    `await secrets.get('this-is-a-very-long-secret-name-with-many-characters-and-numbers-123456789-and-underscores_and_periods.and.more')`,
			description: "Very long name with all valid characters should be valid",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			actual := performVariableExpansion(tc.input)
			require.Equal(t, tc.expected, actual, tc.description)
		})
	}
}
