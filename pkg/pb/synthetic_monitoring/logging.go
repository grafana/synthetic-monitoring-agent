// Copyright 2020 Grafana Labs
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package synthetic_monitoring

import (
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/rs/zerolog"
)

const redactedSecret = "<redacted>"

func (ri RemoteInfo) MarshalZerologObject(e *zerolog.Event) {
	e.Str("name", ri.Name).
		Str("url", ri.Url).
		Str("username", ri.Username).
		Str("password", redactedSecret)
}

func (t Tenant) MarshalZerologObject(e *zerolog.Event) {
	e.Int64("id", t.Id).
		Int64("orgId", t.OrgId).
		Int64("stackId", t.StackId).
		Str("status", t.Status.String()).
		Str("reason", t.Reason).
		Strs("costAttributionLabels", t.CostAttributionLabels).
		Str("labelMode", t.LabelMode.String()).
		Float64("created", t.Created).
		Float64("modified", t.Modified)

	if t.MetricsRemote != nil {
		e.Object("metricsRemote", t.MetricsRemote)
	}

	if t.EventsRemote != nil {
		e.Object("eventsRemote", t.EventsRemote)
	}

	if t.Limits != nil {
		e.Interface("limits", t.Limits)
	}

	if t.SecretStore != nil {
		e.Dict("secretStore", e.CreateDict().
			Str("url", t.SecretStore.Url).
			Str("token", redactedSecret).
			Float64("expiry", t.SecretStore.Expiry))
	}
}

func (c Check) MarshalZerologObject(e *zerolog.Event) {
	e.Int64("id", c.Id).
		Int64("tenantId", c.TenantId)

	if checkType, found := c.Settings.checkType(); found {
		e.Str("type", checkType.String())
	}

	e.Int64("frequency", c.Frequency).
		Int64("offset", c.Offset).
		Int64("timeout", c.Timeout).
		Bool("enabled", c.Enabled).
		Interface("labels", c.Labels).
		Ints64("probes", c.Probes).
		Str("target", c.Target).
		Str("job", c.Job).
		Bool("basicMetricsOnly", c.BasicMetricsOnly).
		Str("alertSensitivity", c.AlertSensitivity).
		Interface("channels", c.Channels).
		Float64("created", c.Created).
		Float64("modified", c.Modified).
		Interface("settings", c.Settings.Redacted())
}

func (cc CheckChange) MarshalZerologObject(e *zerolog.Event) {
	e.Str("operation", cc.Operation.String()).
		Object("check", cc.Check)
}

func (c Changes) MarshalZerologObject(e *zerolog.Event) {
	checks := e.CreateArray()
	for _, checkChange := range c.Checks {
		checks.Object(checkChange)
	}

	tenants := e.CreateArray()
	for _, tenant := range c.Tenants {
		tenants.Object(tenant)
	}

	e.Array("checks", checks).
		Array("tenants", tenants).
		Bool("isDeltaFirstBatch", c.IsDeltaFirstBatch)
}

func (c AdHocCheck) MarshalZerologObject(e *zerolog.Event) {
	e.Str("id", c.Id).
		Int64("tenantId", c.TenantId)

	if checkType, found := c.Settings.checkType(); found {
		e.Str("type", checkType.String())
	}

	e.Int64("timeout", c.Timeout).
		Ints64("probes", c.Probes).
		Str("target", c.Target).
		Interface("channels", c.Channels).
		Interface("settings", c.Settings.Redacted())
}

func (r AdHocRequest) MarshalZerologObject(e *zerolog.Event) {
	e.Object("adHocCheck", r.AdHocCheck)

	if r.Tenant != nil {
		e.Object("tenant", r.Tenant)
	}
}

// Redacted returns a copy of the settings with every value that might have a credential replaced by a placeholder.
func (s CheckSettings) Redacted() CheckSettings {
	redacted := *(proto.Clone(&s).(*CheckSettings))

	if redacted.Http != nil {
		redactHttpSettings(redacted.Http)
	}

	if redacted.Tcp != nil {
		redactTcpSettings(redacted.Tcp)
	}

	if redacted.Grpc != nil {
		redactTlsConfig(redacted.Grpc.TlsConfig)
	}

	if redacted.Multihttp != nil {
		redactMultiHttpSettings(redacted.Multihttp)
	}

	if redacted.Scripted != nil {
		redacted.Scripted.Script = redactBytes(redacted.Scripted.Script)
	}

	if redacted.Browser != nil {
		redacted.Browser.Script = redactBytes(redacted.Browser.Script)
	}

	return redacted
}

func redactHttpSettings(s *HttpSettings) {
	if s.BasicAuth != nil {
		s.BasicAuth.Password = redactString(s.BasicAuth.Password)
	}

	s.BearerToken = redactString(s.BearerToken)
	s.Body = redactString(s.Body)
	s.Headers = redactHeaders(s.Headers)
	s.ProxyConnectHeaders = redactHeaders(s.ProxyConnectHeaders)

	redactTlsConfig(s.TlsConfig)

	if s.Oauth2Config != nil {
		s.Oauth2Config.ClientSecret = redactString(s.Oauth2Config.ClientSecret)

		for i := range s.Oauth2Config.EndpointParams {
			s.Oauth2Config.EndpointParams[i].Value = redactedSecret
		}

		redactTlsConfig(s.Oauth2Config.TlsConfig)
	}
}

func redactTcpSettings(s *TcpSettings) {
	redactTlsConfig(s.TlsConfig)

	// TCP usually carries credentials as-is.
	for i := range s.QueryResponse {
		s.QueryResponse[i].Send = redactBytes(s.QueryResponse[i].Send)
		s.QueryResponse[i].Expect = redactBytes(s.QueryResponse[i].Expect)
	}
}

func redactMultiHttpSettings(s *MultiHttpSettings) {
	for _, entry := range s.Entries {
		if entry == nil {
			continue
		}

		if entry.Request != nil {
			for _, header := range entry.Request.Headers {
				if header != nil {
					header.Value = redactedSecret
				}
			}

			for _, field := range entry.Request.QueryFields {
				if field != nil {
					field.Value = redactedSecret
				}
			}

			if entry.Request.Body != nil {
				entry.Request.Body.Payload = redactBytes(entry.Request.Body.Payload)
			}
		}

		for _, assertion := range entry.Assertions {
			if assertion != nil {
				assertion.Value = redactString(assertion.Value)
			}
		}
	}
}

func redactTlsConfig(c *TLSConfig) {
	if c == nil {
		return
	}

	c.CACert = redactBytes(c.CACert)
	c.ClientCert = redactBytes(c.ClientCert)
	c.ClientKey = redactBytes(c.ClientKey)
}

func redactHeaders(headers []string) []string {
	redacted := make([]string, 0, len(headers))

	for _, header := range headers {
		name, _, found := strings.Cut(header, ":")
		if !found {
			redacted = append(redacted, redactedSecret)
			continue
		}

		redacted = append(redacted, name+": "+redactedSecret)
	}

	return redacted
}

func redactString(s string) string {
	if s == "" {
		return s
	}

	return redactedSecret
}

func redactBytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}

	return []byte(redactedSecret)
}
