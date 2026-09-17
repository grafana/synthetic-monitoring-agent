package multihttp

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/synthetic-monitoring-agent/internal/testhelper"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite the testdata golden scripts instead of asserting against them")

// TestScriptGolden pins the k6 script that settingsToScript generates for a
// multihttp check using ${variable} interpolation.
//
// Unlike a secret, a variable has no value when the script is built: it is
// extracted from the response to an earlier request in the same check, so the Go
// side emits vars['name'] and k6 resolves it mid-run. There is no substituted
// value to observe at this boundary, which makes the generated script the thing
// worth asserting on.
//
// The existing TestBuildUrl, TestBuildHeaders, TestBuildQueryParams and
// TestBuildVars cover each builder on its own. These fixtures cover the script
// as a whole, so a change in how the pieces are assembled, or in the escaping
// they share, shows up as a readable diff. Regenerate with:
//
//	go test ./internal/prober/multihttp/ -run TestScriptGolden -update
//
// and read the diff before committing it.
func TestScriptGolden(t *testing.T) {
	testcases := map[string]*sm.MultiHttpSettings{
		// One entry with a ${variable} at every site that expands one: the URL
		// path (buildUrl), both halves of a query field (buildQueryParams), a
		// header value (buildHeaders) and the body (interpolateBodyVars).
		// "tenant" appears twice in the body on purpose - interpolateBodyVariables
		// de-duplicates by match, so it must emit one replaceAll, not two.
		"variables_in_request": {
			Entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Method: sm.HttpMethod_POST,
						Url:    "http://example.org/${tenant}/items",
						QueryFields: []*sm.QueryField{
							{Name: "${filterName}", Value: "${filterValue}"},
							{Name: "page", Value: "1"},
						},
						Headers: []*sm.HttpHeader{
							{Name: "Authorization", Value: "Bearer ${accessToken}"},
							{Name: "X-Tenant", Value: "${tenant}"},
						},
						Body: &sm.HttpRequestBody{
							ContentType: "application/json",
							Payload:     []byte(`{"tenant":"${tenant}","again":"${tenant}","user":"${username}"}`),
						},
					},
				},
			},
		},

		// buildVars, once per extraction type. CSS_SELECTOR appears twice because
		// an empty Attribute and a set one take different branches.
		"extracted_variables": {
			Entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Method: sm.HttpMethod_GET,
						Url:    "http://example.org/login",
					},
					Variables: []*sm.MultiHttpEntryVariable{
						{Type: sm.MultiHttpEntryVariableType_JSON_PATH, Name: "accessToken", Expression: "$.token"},
						{Type: sm.MultiHttpEntryVariableType_REGEX, Name: "sessionId", Expression: "session=([a-z0-9]+)"},
						{Type: sm.MultiHttpEntryVariableType_CSS_SELECTOR, Name: "pageTitle", Expression: "h1"},
						{Type: sm.MultiHttpEntryVariableType_CSS_SELECTOR, Name: "csrf", Expression: "input[name=csrf]", Attribute: "value"},
					},
				},
			},
		},

		// How an expanded value is escaped for JavaScript. template.JSEscape
		// passes multi-byte UTF-8 through unchanged and escapes "<" and ">" as
		// \u003C and \u003E. ${my-var} stays a literal string because the
		// userVariables pattern does not allow a hyphen in a name.
		"escaping": {
			Entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Method: sm.HttpMethod_GET,
						Url:    "http://example.org/héllo/${tenant}",
						Headers: []*sm.HttpHeader{
							{Name: "X-Script", Value: "</script> ${tenant}"},
							{Name: "X-Emoji", Value: "emoji 🎉 ${tenant}"},
							{Name: "X-Hyphen", Value: "${my-var}"},
							{Name: "X-Quote", Value: "it's a ${tenant}"},
							{Name: "X-Backslash", Value: `back\slash ${tenant}`},
						},
						Body: &sm.HttpRequestBody{
							ContentType: "text/plain",
							Payload:     []byte("héllo ${tenant}"),
						},
					},
				},
			},
		},

		// The shape the feature exists for: request one logs in and extracts a
		// token, request two sends it as a header.
		"variable_chaining": {
			Entries: []*sm.MultiHttpEntry{
				{
					Request: &sm.MultiHttpEntryRequest{
						Method: sm.HttpMethod_POST,
						Url:    "http://example.org/login",
						Body: &sm.HttpRequestBody{
							ContentType: "application/json",
							Payload:     []byte(`{"user":"check-user"}`),
						},
					},
					Variables: []*sm.MultiHttpEntryVariable{
						{Type: sm.MultiHttpEntryVariableType_JSON_PATH, Name: "accessToken", Expression: "$.token"},
					},
					Assertions: []*sm.MultiHttpEntryAssertion{
						{
							Type:      sm.MultiHttpEntryAssertionType_TEXT,
							Subject:   sm.MultiHttpEntryAssertionSubjectVariant_HTTP_STATUS_CODE,
							Condition: sm.MultiHttpEntryAssertionConditionVariant_EQUALS,
							Value:     "200",
						},
					},
				},
				{
					Request: &sm.MultiHttpEntryRequest{
						Method: sm.HttpMethod_GET,
						Url:    "http://example.org/profile",
						Headers: []*sm.HttpHeader{
							{Name: "Authorization", Value: "Bearer ${accessToken}"},
						},
					},
				},
			},
		},
	}

	for name, settings := range testcases {
		t.Run(name, func(t *testing.T) {
			// The fixtures are kept realistic on purpose: anything this rejects
			// could not reach the agent from the API in the first place.
			require.NoError(t, settings.Validate())

			actual, err := settingsToScript(settings)
			require.NoError(t, err)
			require.NotEmpty(t, actual)

			path := filepath.Join("testdata", name+".js")

			if *updateGolden {
				require.NoError(t, os.WriteFile(path, actual, 0o600))

				return
			}

			require.Equal(t, string(testhelper.MustReadFile(t, path)), string(actual))
		})
	}
}
