package synthetic_monitoring

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestRemoteInfoMarshalZerologObject(t *testing.T) {
	remoteInfo := RemoteInfo{
		Name:     "the name",
		Url:      "https://example.com",
		Username: "the username",
		Password: "the password",
	}

	var buf bytes.Buffer

	logger := zerolog.New(&buf)

	logger.Info().Interface("remote_info", remoteInfo).Send()

	expected := `{"level":"info","remote_info":{"name":"the name","url":"https://example.com","username":"the username","password":"<redacted>"}}` + "\n"
	actual := buf.String()

	require.Equal(t, expected, actual)
}

func TestCheckSettingsRedacted(t *testing.T) {
	const secret = "averysecretvalue"

	// Every case carries the secret in each field that could hold one, plus a value that has to stay untouched.
	testcases := map[string]struct {
		settings CheckSettings
		kept     []string
	}{
		"http": {
			settings: CheckSettings{
				Http: &HttpSettings{
					Method:              HttpMethod_POST,
					Headers:             []string{"Authorization: Bearer " + secret, secret},
					ProxyConnectHeaders: []string{"Proxy-Authorization: " + secret},
					Body:                secret,
					BearerToken:         secret,
					BasicAuth:           &BasicAuth{Username: "grafana", Password: secret},
					TlsConfig: &TLSConfig{
						ServerName: "example.org",
						CACert:     []byte(secret),
						ClientCert: []byte(secret),
						ClientKey:  []byte(secret),
					},
					Oauth2Config: &OAuth2Config{
						ClientId:       "the client id",
						ClientSecret:   secret,
						TokenURL:       "https://example.org/token",
						EndpointParams: []Label{{Name: "audience", Value: secret}},
						TlsConfig:      &TLSConfig{ClientKey: []byte(secret)},
					},
					ValidStatusCodes: []int32{200},
				},
			},
			kept: []string{"POST", "grafana", "example.org", "the client id", "https://example.org/token", "audience", "Authorization", "Proxy-Authorization"},
		},
		"tcp": {
			settings: CheckSettings{
				Tcp: &TcpSettings{
					SourceIpAddress: "10.0.0.1",
					TlsConfig:       &TLSConfig{ClientKey: []byte(secret)},
					QueryResponse: []TCPQueryResponse{
						{Send: []byte(secret), Expect: []byte(secret), StartTLS: true},
					},
				},
			},
			kept: []string{"10.0.0.1"},
		},
		"grpc": {
			settings: CheckSettings{
				Grpc: &GrpcSettings{
					Service:   "the service",
					TlsConfig: &TLSConfig{ClientKey: []byte(secret)},
				},
			},
			kept: []string{"the service"},
		},
		"multihttp": {
			settings: CheckSettings{
				Multihttp: &MultiHttpSettings{
					Entries: []*MultiHttpEntry{
						{
							Request: &MultiHttpEntryRequest{
								Url:         "https://example.org/login",
								Headers:     []*HttpHeader{{Name: "Authorization", Value: secret}},
								QueryFields: []*QueryField{{Name: "token", Value: secret}},
								Body:        &HttpRequestBody{ContentType: "application/json", Payload: []byte(secret)},
							},
							Assertions: []*MultiHttpEntryAssertion{
								{Type: MultiHttpEntryAssertionType_TEXT, Value: secret},
							},
							Variables: []*MultiHttpEntryVariable{
								{Name: "sessionId", Expression: "$.session"},
							},
						},
					},
				},
			},
			kept: []string{"https://example.org/login", "Authorization", "token", "application/json", "sessionId", "$.session"},
		},
		"scripted": {
			settings: CheckSettings{Scripted: &ScriptedSettings{Script: []byte(secret)}},
		},
		"browser": {
			settings: CheckSettings{Browser: &BrowserSettings{Script: []byte(secret)}},
		},
		"ping": {
			settings: CheckSettings{Ping: &PingSettings{SourceIpAddress: "10.0.0.1", PacketCount: 3}},
			kept:     []string{"10.0.0.1"},
		},
		"dns": {
			settings: CheckSettings{Dns: &DnsSettings{Server: "8.8.8.8", RecordType: DnsRecordType_A}},
			kept:     []string{"8.8.8.8"},
		},
		"traceroute": {
			settings: CheckSettings{Traceroute: &TracerouteSettings{MaxHops: 64}},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			original, err := json.Marshal(tc.settings)
			require.NoError(t, err)

			redacted, err := json.Marshal(tc.settings.Redacted())
			require.NoError(t, err)

			require.NotContains(t, string(redacted), secret,
				"redacted settings must not contain any value that could be a credential")

			for _, kept := range tc.kept {
				require.Contains(t, string(redacted), kept,
					"redacting must not throw away the values that make the log useful")
			}

			afterwards, err := json.Marshal(tc.settings)
			require.NoError(t, err)
			require.Equal(t, string(original), string(afterwards), "Redacted must not modify the receiver")
		})
	}
}

func TestCheckMarshalZerologObject(t *testing.T) {
	check := Check{
		Id:               1565289,
		TenantId:         1,
		Frequency:        120000,
		Timeout:          10000,
		Enabled:          true,
		Probes:           []int64{7193},
		Target:           "https://intranet.example.org/alerts",
		Job:              "Intranet - Alerts",
		BasicMetricsOnly: true,
		AlertSensitivity: "none",
		Settings: CheckSettings{
			Http: &HttpSettings{
				BasicAuth:   &BasicAuth{Username: "grafana", Password: "randompasswordinplaintext"},
				BearerToken: "supersecretbearertoken",
			},
		},
	}

	var buf bytes.Buffer

	logger := zerolog.New(&buf)
	logger.Info().Interface("check", check).Send()

	expected := `{"level":"info","check":{"id":1565289,"tenantId":1,"type":"http","frequency":120000,"offset":0,"timeout":10000,"enabled":true,"labels":null,"probes":[7193],"target":"https://intranet.example.org/alerts","job":"Intranet - Alerts","basicMetricsOnly":true,"alertSensitivity":"none","channels":null,"created":0,"modified":0,"settings":{"http":{"ipVersion":"Any","method":"GET","noFollowRedirects":false,"basicAuth":{"username":"grafana","password":"<redacted>"},"bearerToken":"<redacted>","failIfSSL":false,"failIfNotSSL":false}}}}` + "\n"

	require.Equal(t, expected, buf.String())
}

func TestCheckChangeMarshalZerologObject(t *testing.T) {
	// A delete operation has a check that holds nothing but an ID. Check.Type panics on settings call.
	checkChange := CheckChange{
		Operation: CheckOperation_CHECK_DELETE,
		Check:     Check{Id: 1565289},
	}

	var buf bytes.Buffer

	logger := zerolog.New(&buf)

	require.NotPanics(t, func() {
		logger.Info().Interface("check change", checkChange).Send()
	})

	require.Contains(t, buf.String(), `"operation":"CHECK_DELETE"`)
	require.Contains(t, buf.String(), `"id":1565289`)
	require.NotContains(t, buf.String(), `"type"`)
}

func TestAdHocRequestMarshalZerologObject(t *testing.T) {
	const (
		checkPassword  = "randompasswordinplaintext"
		remotePassword = "theremotepassword"
	)

	request := AdHocRequest{
		AdHocCheck: AdHocCheck{
			Id:       "the ad-hoc check id",
			TenantId: 1,
			Target:   "https://example.org",
			Settings: CheckSettings{
				Http: &HttpSettings{
					BasicAuth: &BasicAuth{Username: "grafana", Password: checkPassword},
				},
			},
		},
		Tenant: &Tenant{
			Id:            1,
			MetricsRemote: &RemoteInfo{Name: "metrics", Url: "https://example.org/push", Username: "1", Password: remotePassword},
			SecretStore:   &SecretStore{Url: "https://example.org/secrets", Token: "thesecretstoretoken"},
		},
	}

	var buf bytes.Buffer

	logger := zerolog.New(&buf)
	logger.Info().Interface("request", &request).Send()

	actual := buf.String()

	require.NotContains(t, actual, checkPassword)
	require.NotContains(t, actual, remotePassword)
	require.NotContains(t, actual, "thesecretstoretoken")
	require.Contains(t, actual, "the ad-hoc check id")
	require.Contains(t, actual, "https://example.org/push")
}

func TestChangesMarshalZerologObject(t *testing.T) {
	const (
		checkPassword  = "randompasswordinplaintext"
		remotePassword = "theremotepassword"
	)

	changes := Changes{
		Checks: []CheckChange{
			{
				Operation: CheckOperation_CHECK_ADD,
				Check: Check{
					Id: 1565289,
					Settings: CheckSettings{
						Http: &HttpSettings{
							BasicAuth: &BasicAuth{Username: "grafana", Password: checkPassword},
						},
					},
				},
			},
		},
		Tenants: []Tenant{
			{
				Id:            1,
				MetricsRemote: &RemoteInfo{Url: "https://example.org/push", Username: "1", Password: remotePassword},
			},
		},
		IsDeltaFirstBatch: true,
	}

	var buf bytes.Buffer

	logger := zerolog.New(&buf)
	logger.Info().Interface("changes", changes).Send()

	actual := buf.String()

	require.NotContains(t, actual, checkPassword)
	require.NotContains(t, actual, remotePassword)
	require.Contains(t, actual, `"id":1565289`)
	require.Contains(t, actual, "https://example.org/push")
	require.Contains(t, actual, `"isDeltaFirstBatch":true`)
}

// TestCheckJSONRoundTrip protects the wire format.
func TestCheckJSONRoundTrip(t *testing.T) {
	const (
		password    = "randompasswordinplaintext"
		bearerToken = "supersecretbearertoken"
	)

	check := Check{
		Id:       1565289,
		TenantId: 1,
		Target:   "https://intranet.example.org/alerts",
		Job:      "Intranet - Alerts",
		Settings: CheckSettings{
			Http: &HttpSettings{
				BasicAuth:   &BasicAuth{Username: "grafana", Password: password},
				BearerToken: bearerToken,
			},
		},
	}

	encoded, err := json.Marshal(&check)
	require.NoError(t, err)

	var decoded Check

	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, check, decoded)
	require.Equal(t, password, decoded.Settings.Http.BasicAuth.Password)
	require.Equal(t, bearerToken, decoded.Settings.Http.BearerToken)
}
