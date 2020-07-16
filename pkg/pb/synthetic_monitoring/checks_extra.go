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

// package synthetic_monitoring provides access to types and methods
// that allow for the production and consumption of protocol buffer
// messages used to communicate with synthetic-monitoring-api.
package synthetic_monitoring

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrInvalidTenantId        = errors.New("invalid tentantId")
	ErrInvalidCheckProbes     = errors.New("invalid check probes")
	ErrInvalidCheckTarget     = errors.New("invalid check target")
	ErrInvalidCheckJob        = errors.New("invalid check job")
	ErrInvalidCheckFrequency  = errors.New("invalid check frequency")
	ErrInvalidCheckLabelName  = errors.New("invalid check label name")
	ErrTooManyCheckLabels     = errors.New("too many check labels")
	ErrInvalidCheckLabelValue = errors.New("invalid check label value")

	ErrInvalidCheckSettings = errors.New("invalid check settings")

	ErrInvalidFQHNLenght        = errors.New("invalid FQHN lenght")
	ErrInvalidFQHNElements      = errors.New("invalid number of elements in fqhn")
	ErrInvalidFQHNElementLenght = errors.New("invalid FQHN element lenght")
	ErrInvalidFQHNElement       = errors.New("invalid FQHN element")

	ErrInvalidPingHostname    = errors.New("invalid ping hostname")
	ErrInvalidPingPayloadSize = errors.New("invalid ping payload size")

	ErrInvalidDnsName             = errors.New("invalid DNS name")
	ErrInvalidDnsServer           = errors.New("invalid DNS server")
	ErrInvalidDnsPort             = errors.New("invalid DNS port")
	ErrInvalidDnsProtocolString   = errors.New("invalid DNS protocol string")
	ErrInvalidDnsProtocolValue    = errors.New("invalid DNS protocol value")
	ErrInvalidDnsRecordTypeString = errors.New("invalid DNS record type string")
	ErrInvalidDnsRecordTypeValue  = errors.New("invalid DNS record type value")

	ErrInvalidHttpUrl          = errors.New("invalid HTTP URL")
	ErrInvalidHttpMethodString = errors.New("invalid HTTP method string")
	ErrInvalidHttpMethodValue  = errors.New("invalid HTTP method value")

	ErrInvalidTcpHostname = errors.New("invalid TCP hostname")
	ErrInvalidTcpPort     = errors.New("invalid TCP port")

	ErrInvalidIpVersionString = errors.New("invalid ip version string")
	ErrInvalidIpVersionValue  = errors.New("invalid ip version value")

	ErrInvalidProbeName              = errors.New("invalid probe name")
	ErrInvalidProbeReservedLabelName = errors.New("invalid probe, reserved label name")
	ErrInvalidProbeLabelName         = errors.New("invalid probe label name")
	ErrInvalidProbeLabelValue        = errors.New("invalid probe label value")
	ErrTooManyProbeLabels            = errors.New("too many probe labels")
	ErrInvalidProbeLatitude          = errors.New("invalid probe latitude")
	ErrInvalidProbeLongitude         = errors.New("invalid probe longitude")
)

const (
	MaxCheckLabels = 5
	MaxProbeLabels = 3
)

func (c *Check) Validate() error {
	if c.TenantId < 0 {
		return ErrInvalidTenantId
	}
	if len(c.Probes) == 0 {
		return ErrInvalidCheckProbes
	}
	if len(c.Target) == 0 {
		return ErrInvalidCheckTarget
	}
	if len(c.Job) == 0 {
		return ErrInvalidCheckJob
	}

	// frequency must be in [1, 120] seconds
	if c.Frequency < 1*1000 || c.Frequency > 120*1000 {
		return ErrInvalidCheckFrequency
	}

	// timeout must be in [1, 10] seconds
	if c.Timeout < 1*1000 || c.Timeout > 10*1000 {
		return ErrInvalidCheckFrequency
	}

	if len(c.Labels) > MaxCheckLabels {
		return ErrTooManyCheckLabels
	}

	for _, label := range c.Labels {

		if label.Name == "" {
			return ErrInvalidCheckLabelName
		}

		if len(label.Value) == 0 || len(label.Value) > 32 {
			return ErrInvalidCheckLabelValue
		}
	}

	settingsCount := 0

	if c.Settings.Ping != nil {
		settingsCount++

		if err := validateHost(c.Target); err != nil {
			return ErrInvalidPingHostname
		}

		if err := c.Settings.Ping.Validate(); err != nil {
			return err
		}
	}

	if c.Settings.Http != nil {
		settingsCount++

		if target, err := url.Parse(c.Target); err != nil {
			return ErrInvalidHttpUrl
		} else if !(target.Scheme == "http" || target.Scheme == "https") {
			return ErrInvalidHttpUrl
		}

		if err := c.Settings.Http.Validate(); err != nil {
			return err
		}
	}

	if c.Settings.Dns != nil {
		settingsCount++

		if err := validateHost(c.Target); err != nil {
			return ErrInvalidDnsName
		}

		if err := c.Settings.Dns.Validate(); err != nil {
			return err
		}
	}

	if c.Settings.Tcp != nil {
		settingsCount++

		if err := validateHostPort(c.Target); err != nil {
			return err
		}

		if err := c.Settings.Tcp.Validate(); err != nil {
			return err
		}
	}

	if settingsCount != 1 {
		return ErrInvalidCheckSettings
	}

	return nil
}

func (c *Check) ConfigVersion() string {
	return strconv.FormatInt(int64(c.Modified*1000000000), 10)
}

func (s *PingSettings) Validate() error {
	if s.PayloadSize < 0 || s.PayloadSize > 65499 {
		return ErrInvalidPingPayloadSize
	}

	return nil
}

func (s *HttpSettings) Validate() error {
	return nil
}

func (s *DnsSettings) Validate() error {
	if len(s.Server) == 0 || validateHost(s.Server) != nil {
		return ErrInvalidDnsServer
	}

	if s.Port < 0 || s.Port > 65535 {
		return ErrInvalidDnsPort
	}

	return nil
}

func (s *TcpSettings) Validate() error {
	return nil
}

func (p *Probe) Validate() error {
	if p.TenantId < 0 {
		return ErrInvalidTenantId
	}
	if p.Name == "" {
		return ErrInvalidProbeName
	}
	if len(p.Labels) > MaxProbeLabels {
		return ErrTooManyProbeLabels
	}
	for _, label := range p.Labels {
		if label.Name == "" {
			return ErrInvalidProbeLabelName
		}

		if len(label.Value) == 0 || len(label.Value) > 32 {
			return ErrInvalidProbeLabelValue
		}
	}

	if p.Latitude < -90 || p.Latitude > 90 {
		return ErrInvalidProbeLatitude
	}

	if p.Longitude < -180 || p.Longitude > 180 {
		return ErrInvalidProbeLongitude
	}

	return nil
}

func lookupValue(v int32, m map[int32]string) []byte {
	if str, ok := m[v]; ok {
		return []byte(`"` + str + `"`)
	}

	return nil
}

func lookupString(b []byte, m map[string]int32) (int32, bool) {
	in, err := strconv.Unquote(string(b))
	if err != nil {
		return 0, false
	}

	// first try a direct lookup in the known values
	if v, ok := m[in]; ok {
		return v, true
	}

	// not found, try again doing an case-insensitive search

	in = strings.ToLower(in)

	for str, v := range IpVersion_value {
		if strings.ToLower(str) == in {
			return v, true
		}
	}

	return 0, false
}

func (v IpVersion) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), IpVersion_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidIpVersionValue
}

func (out *IpVersion) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, IpVersion_value); found {
		*out = IpVersion(v)
		return nil
	}

	return ErrInvalidIpVersionString
}

func (v HttpMethod) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), HttpMethod_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidHttpMethodValue
}

func (out *HttpMethod) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, HttpMethod_value); found {
		*out = HttpMethod(v)
		return nil
	}

	return ErrInvalidHttpMethodString
}

func (v DnsRecordType) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), DnsRecordType_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidDnsRecordTypeValue
}

func (out *DnsRecordType) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, DnsRecordType_value); found {
		*out = DnsRecordType(v)
		return nil
	}

	return ErrInvalidDnsRecordTypeString
}

func (v DnsProtocol) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), DnsProtocol_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidDnsProtocolValue
}

func (out *DnsProtocol) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, DnsProtocol_value); found {
		*out = DnsProtocol(v)
		return nil
	}

	return ErrInvalidDnsProtocolString
}

func validateHost(target string) error {
	if ip := net.ParseIP(target); ip != nil {
		return nil
	}

	return checkFQHN(target)
}

func validateHostPort(target string) error {
	if host, port, err := net.SplitHostPort(target); err != nil {
		return ErrInvalidCheckTarget
	} else if validateHost(host) != nil {
		return ErrInvalidTcpHostname
	} else if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return ErrInvalidTcpPort
	}

	return nil
}

// checkFQHN validates that the provided fully qualified hostname
// follows RFC 1034, section 3.5
// (https://tools.ietf.org/html/rfc1034#section-3.5).
//
// This assumes that the *hostname* part of the FQHN follows the same
// rules.
//
// Note that if there are any IDNA transformations going on, they need
// to happen _before_ calling this function.
func checkFQHN(fqhn string) error {
	if len(fqhn) == 0 || len(fqhn) > 255 {
		return ErrInvalidFQHNLenght
	}

	labels := strings.Split(fqhn, ".")

	if len(labels) < 2 {
		return ErrInvalidFQHNElements
	}

	isLetter := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}

	isDigit := func(r rune) bool {
		return (r >= '0' && r <= '9')
	}

	isDash := func(r rune) bool {
		return (r == '-')
	}

	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return ErrInvalidFQHNElementLenght
		}

		runes := []rune(label)

		// labels must start with a letter
		if r := runes[0]; !isLetter(r) {
			return ErrInvalidFQHNElement
		}

		// labels must end with a letter or digit
		if r := runes[len(runes)-1]; !isLetter(r) && !isDigit(r) {
			return ErrInvalidFQHNElement
		}

		for _, r := range runes {
			// the only valid characters are [-A-Za-z0-9].
			if !isLetter(r) && !isDigit(r) && !isDash(r) {
				return ErrInvalidFQHNElement
			}
		}
	}

	return nil
}
