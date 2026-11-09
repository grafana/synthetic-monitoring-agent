package synthetic_monitoring

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScriptReferencesSecrets(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		script string
		want   bool
	}{
		"empty script": {
			script: ``,
			want:   false,
		},
		"no mention of the module": {
			script: "import http from 'k6/http'\nexport default function(){ http.get('https://grafana.com/') }\n",
			want:   false,
		},

		// Each form an import statement can take. A check that matched
		// statement shapes would need a rule for every one.
		"default import": {
			script: "import secrets from 'k6/secrets'\n",
			want:   true,
		},
		"default import, double quotes": {
			script: "import secrets from \"k6/secrets\"\n",
			want:   true,
		},
		"named import": {
			script: "import { secrets } from 'k6/secrets'\n",
			want:   true,
		},
		"named import, aliased": {
			script: "import { secrets as s } from 'k6/secrets'\n",
			want:   true,
		},
		"named import, spread over lines": {
			script: "import {\n  secrets,\n} from 'k6/secrets'\n",
			want:   true,
		},
		"namespace import": {
			script: "import * as s from 'k6/secrets'\n",
			want:   true,
		},

		// These load the module without an import statement, so a check that
		// matches statement shapes misses them.
		"dynamic import": {
			script: "export default async function(){ const s = await import('k6/secrets') }\n",
			want:   true,
		},
		"require, conventional name": {
			script: "const secrets = require('k6/secrets')\n",
			want:   true,
		},
		"require, any other name": {
			script: "const sec = require('k6/secrets')\n",
			want:   true,
		},
		"require, declared with let": {
			script: "let secrets = require('k6/secrets')\n",
			want:   true,
		},
		"module name held in a variable": {
			script: "const mod = 'k6/secrets'\nconst s = await import(mod)\n",
			want:   true,
		},

		// The parser separates comments from code, so a mention in a comment
		// needs no rule of its own.
		"line comment": {
			script: "// import secrets from 'k6/secrets'\nexport default function(){}\n",
			want:   false,
		},
		"block comment": {
			script: "/*\nimport secrets from 'k6/secrets'\n*/\nexport default function(){}\n",
			want:   false,
		},
		"trailing comment after real code": {
			script: "import http from 'k6/http' // not k6/secrets\n",
			want:   false,
		},

		// A quote inside a regular expression literal. Only the surrounding
		// code says whether a leading slash starts a regular expression or
		// divides, and a scan that guesses wrong misreads the rest of the file.
		"quote inside a regex literal": {
			script: "const re = /['\"]/\nexport default function(){ re.test('x') }\n",
			want:   false,
		},
		"regex literal then a real import": {
			script: "const re = /['\"]/\nimport secrets from 'k6/secrets'\n",
			want:   true,
		},

		// The name appears but nothing loads it. A yes here only narrows which
		// probes can run the check.
		"name in an unrelated string": {
			script: "const label = 'k6/secrets'\nexport default function(){ console.log(label) }\n",
			want:   true,
		},
		"name inside a template literal": {
			script: "const doc = `see k6/secrets for details`\n",
			want:   true,
		},

		// Known gap: the name does not exist until the script runs.
		"module name built by concatenation": {
			script: "const s = require('k6' + '/secrets')\n",
			want:   false,
		},

		// This form loads the module without binding it to a name. The author
		// still wanted secrets, so the answer is yes.
		"side-effect only import": {
			script: "import 'k6/secrets'\n",
			want:   true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, scriptReferencesSecrets([]byte(tc.script)))
		})
	}
}

func TestHttpSettingsReferenceSecrets(t *testing.T) {
	t.Parallel()

	const ref = `${secrets.my-token}`

	testcases := map[string]struct {
		settings *HttpSettings
		want     bool
	}{
		"nil settings": {
			settings: nil,
			want:     false,
		},
		"empty settings": {
			settings: &HttpSettings{},
			want:     false,
		},
		"plaintext bearer token": {
			settings: &HttpSettings{BearerToken: "plain-token"},
			want:     false,
		},
		"reference in bearer token": {
			settings: &HttpSettings{BearerToken: ref},
			want:     true,
		},
		"reference surrounded by text": {
			settings: &HttpSettings{BearerToken: "Bearer " + ref + " trailing"},
			want:     true,
		},
		"reference in basic auth password": {
			settings: &HttpSettings{BasicAuth: &BasicAuth{Username: "u", Password: ref}},
			want:     true,
		},
		"plaintext basic auth password": {
			settings: &HttpSettings{BasicAuth: &BasicAuth{Username: "u", Password: "pw"}},
			want:     false,
		},
		"reference in CA cert": {
			settings: &HttpSettings{TlsConfig: &TLSConfig{CACert: []byte(ref)}},
			want:     true,
		},
		"reference in client cert": {
			settings: &HttpSettings{TlsConfig: &TLSConfig{ClientCert: []byte(ref)}},
			want:     true,
		},
		"reference in client key": {
			settings: &HttpSettings{TlsConfig: &TLSConfig{ClientKey: []byte(ref)}},
			want:     true,
		},
		"plaintext PEM in TLS fields": {
			settings: &HttpSettings{TlsConfig: &TLSConfig{
				CACert: []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----"),
			}},
			want: false,
		},

		// This name cannot resolve, but it was still meant to be a secret.
		"reference with an unusable name": {
			settings: &HttpSettings{BearerToken: `${secrets.My_Token}`},
			want:     true,
		},
		"empty secret name": {
			settings: &HttpSettings{BearerToken: `${secrets.}`},
			want:     true,
		},

		"reference in a custom header value": {
			settings: &HttpSettings{Headers: []string{"X-Api-Key: " + ref}},
			want:     true,
		},
		"reference with text in a custom header value": {
			settings: &HttpSettings{Headers: []string{"Authorization: ApiKey " + ref}},
			want:     true,
		},
		"plaintext custom header": {
			settings: &HttpSettings{Headers: []string{"X-Api-Key: plain-key"}},
			want:     false,
		},

		// Header names are plain text, so a reference there is never resolved.
		"reference in a custom header name": {
			settings: &HttpSettings{Headers: []string{ref + ": value"}},
			want:     false,
		},
		"custom header without a colon": {
			settings: &HttpSettings{Headers: []string{ref}},
			want:     false,
		},

		"reference in a reserved header": {
			settings: &HttpSettings{Headers: []string{"X-Sm-Id: " + ref}},
			want:     true,
		},
		"variable syntax is not a secret reference": {
			settings: &HttpSettings{BearerToken: "${my_var}"},
			want:     false,
		},
		"dollar without a placeholder": {
			settings: &HttpSettings{BearerToken: "$secrets.my-token"},
			want:     false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got bool

			tc.settings.secretFields(func(SecretField) bool {
				got = true

				return false
			})

			require.Equal(t, tc.want, got)
		})
	}
}

func TestCheckSettingsSecretFields(t *testing.T) {
	t.Parallel()

	const ref = `${secrets.my-token}`

	everyHttpField := CheckSettings{Http: &HttpSettings{
		BearerToken: ref,
		BasicAuth:   &BasicAuth{Username: "u", Password: ref},
		TlsConfig: &TLSConfig{
			CACert:     []byte(ref),
			ClientCert: []byte(ref),
			ClientKey:  []byte(ref),
		},
		Headers: []string{"X-Api-Key: " + ref, "X-Tenant: " + ref},
	}}

	t.Run("every http field, in order", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []SecretField{
			SecretFieldHttpBearerToken,
			SecretFieldHttpBasicAuthPassword,
			SecretFieldHttpTLSCACert,
			SecretFieldHttpTLSClientCert,
			SecretFieldHttpTLSClientKey,
			SecretFieldHttpHeaders,
		}, slices.Collect(everyHttpField.SecretFields()))
	})

	// The field names a place in the settings, so many headers report it once.
	t.Run("several custom headers", func(t *testing.T) {
		t.Parallel()

		settings := CheckSettings{Http: &HttpSettings{Headers: []string{
			"X-Api-Key: " + ref,
			"X-Plain: value",
			"X-Tenant: tenant-" + ref,
		}}}
		require.Equal(t, []SecretField{SecretFieldHttpHeaders}, slices.Collect(settings.SecretFields()))
	})

	// Headers without a reference add no field.
	t.Run("plain custom headers", func(t *testing.T) {
		t.Parallel()

		plain := *everyHttpField.Http
		plain.Headers = []string{"X-Plain: value"}

		require.Equal(t, []SecretField{
			SecretFieldHttpBearerToken,
			SecretFieldHttpBasicAuthPassword,
			SecretFieldHttpTLSCACert,
			SecretFieldHttpTLSClientCert,
			SecretFieldHttpTLSClientKey,
		}, slices.Collect(CheckSettings{Http: &plain}.SecretFields()))
	})

	// The runtime panics if yield is called again after it returned false.
	t.Run("stops when the caller breaks", func(t *testing.T) {
		t.Parallel()

		total := len(slices.Collect(everyHttpField.SecretFields()))

		for stopAfter := 1; stopAfter <= total; stopAfter++ {
			var seen int

			require.NotPanics(t, func() {
				for range everyHttpField.SecretFields() {
					seen++
					if seen == stopAfter {
						break
					}
				}
			})
			require.Equal(t, stopAfter, seen)
		}
	})

	t.Run("script", func(t *testing.T) {
		t.Parallel()

		settings := CheckSettings{Scripted: &ScriptedSettings{Script: []byte(`import secrets from 'k6/secrets'`)}}
		require.Equal(t, []SecretField{SecretFieldScriptedScript}, slices.Collect(settings.SecretFields()))
	})

	t.Run("browser", func(t *testing.T) {
		t.Parallel()

		settings := CheckSettings{Browser: &BrowserSettings{Script: []byte(`import secrets from 'k6/secrets'`)}}
		require.Equal(t, []SecretField{SecretFieldBrowserScript}, slices.Collect(settings.SecretFields()))
	})

	t.Run("plaintext only", func(t *testing.T) {
		t.Parallel()

		settings := CheckSettings{Http: &HttpSettings{BearerToken: "plain"}}
		require.Empty(t, slices.Collect(settings.SecretFields()))
	})
}

func TestCheckSettingsReferencesSecrets(t *testing.T) {
	t.Parallel()

	const ref = `${secrets.my-token}`

	usingScript := []byte("import secrets from 'k6/secrets'\n")
	plainScript := []byte("import http from 'k6/http'\n")

	testcases := map[string]struct {
		settings CheckSettings
		want     bool
	}{
		// Nothing set, which is what a check being deleted looks like.
		"no settings at all": {
			settings: CheckSettings{},
			want:     false,
		},

		"http with a reference": {
			settings: CheckSettings{Http: &HttpSettings{BearerToken: ref}},
			want:     true,
		},
		"http without one": {
			settings: CheckSettings{Http: &HttpSettings{BearerToken: "plain"}},
			want:     false,
		},
		"scripted using the module": {
			settings: CheckSettings{Scripted: &ScriptedSettings{Script: usingScript}},
			want:     true,
		},
		"scripted not using it": {
			settings: CheckSettings{Scripted: &ScriptedSettings{Script: plainScript}},
			want:     false,
		},
		"browser using the module": {
			settings: CheckSettings{Browser: &BrowserSettings{Script: usingScript}},
			want:     true,
		},
		"browser not using it": {
			settings: CheckSettings{Browser: &BrowserSettings{Script: plainScript}},
			want:     false,
		},

		// None of these has a field that takes a secret.
		"dns": {
			settings: CheckSettings{Dns: &DnsSettings{}},
			want:     false,
		},
		"grpc": {
			settings: CheckSettings{Grpc: &GrpcSettings{}},
			want:     false,
		},
		"ping": {
			settings: CheckSettings{Ping: &PingSettings{}},
			want:     false,
		},
		"tcp": {
			settings: CheckSettings{Tcp: &TcpSettings{}},
			want:     false,
		},
		"traceroute": {
			settings: CheckSettings{Traceroute: &TracerouteSettings{}},
			want:     false,
		},

		// Multihttp has credential fields, but nothing resolves secrets in
		// them yet.
		"multihttp": {
			settings: CheckSettings{Multihttp: &MultiHttpSettings{}},
			want:     false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.settings.ReferencesSecrets())
		})
	}
}

// TestReferencesSecretsCoversEveryCheckType fails when someone adds a check
// type without deciding whether it can carry a secret.
func TestReferencesSecretsCoversEveryCheckType(t *testing.T) {
	t.Parallel()

	settingsFor := map[CheckType]CheckSettings{
		CheckTypeDns:        {Dns: &DnsSettings{}},
		CheckTypeHttp:       {Http: &HttpSettings{}},
		CheckTypePing:       {Ping: &PingSettings{}},
		CheckTypeTcp:        {Tcp: &TcpSettings{}},
		CheckTypeTraceroute: {Traceroute: &TracerouteSettings{}},
		CheckTypeScripted:   {Scripted: &ScriptedSettings{}},
		CheckTypeMultiHttp:  {Multihttp: &MultiHttpSettings{}},
		CheckTypeGrpc:       {Grpc: &GrpcSettings{}},
		CheckTypeBrowser:    {Browser: &BrowserSettings{}},
	}

	for _, checkType := range CheckTypeValues() {
		settings, found := settingsFor[checkType]
		require.Truef(t, found,
			"check type %q has no entry here, so nobody has decided whether it can carry a secret", checkType)

		// Empty settings hold no reference, so a yes here means the type has
		// no case of its own and fell through to the safe default.
		require.Falsef(t, settings.ReferencesSecrets(),
			"empty %q settings answered yes, which means the type is missing a case in ReferencesSecrets", checkType)
	}
}

func TestCheckReferencesSecrets(t *testing.T) {
	t.Parallel()

	check := Check{Settings: CheckSettings{Http: &HttpSettings{BearerToken: `${secrets.my-token}`}}}
	require.True(t, check.ReferencesSecrets())

	adhoc := AdHocCheck{Settings: CheckSettings{Http: &HttpSettings{BearerToken: "plain"}}}
	require.False(t, adhoc.ReferencesSecrets())
}
