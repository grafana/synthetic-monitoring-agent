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

// package worldping provides access to types and methods that allow for
// the production and consumption of protocol buffer messages used to
// communicate with worldping-api.
package worldping

import (
	fmt "fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidCheckProbes = fmt.Errorf("invalid check probes")

	ErrInvalidCheckSettings = fmt.Errorf("invalid check settings")

	ErrInvalidDnsProtocolString = fmt.Errorf("invalid DNS protocl string")
	ErrInvalidDnsProtocolValue  = fmt.Errorf("invalid DNS protocol value")

	ErrInvalidDnsRecordTypeString = fmt.Errorf("invalid DNS record type string")
	ErrInvalidDnsRecordTypeValue  = fmt.Errorf("invalid DNS record type value")

	ErrInvalidHttpMethodString = fmt.Errorf("invalid HTTP method string")
	ErrInvalidHttpMethodValue  = fmt.Errorf("invalid HTTP method value")

	ErrInvalidIpVersionString = fmt.Errorf("invalid ip version string")
	ErrInvalidIpVersionValue  = fmt.Errorf("invalid ip version value")

	ErrInvalidValidationMethodString = fmt.Errorf("invalid validation method string")
	ErrInvalidValidationMethodValue  = fmt.Errorf("invalid validation method value")

	ErrInvalidValidationSeverityString = fmt.Errorf("invalid validation severity string")
	ErrInvalidValidationSeverityValue  = fmt.Errorf("invalid validation severity value")

	ErrInvalidProbe = fmt.Errorf("invalid probe")
)

func (c *Check) Validate() error {
	if len(c.Probes) == 0 {
		return ErrInvalidCheckProbes
	}

	settingsCount := 0

	if c.Settings.Ping != nil {
		settingsCount++
	}

	if c.Settings.Http != nil {
		settingsCount++
	}

	if c.Settings.Dns != nil {
		settingsCount++
	}

	if settingsCount != 1 {
		return ErrInvalidCheckSettings
	}

	return nil
}

func (p *Probe) Validate() error {
	if p.Name == "" {
		return ErrInvalidProbe
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

func (v ValidationMethod) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), ValidationMethod_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidValidationSeverityValue
}

func (out *ValidationMethod) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, ValidationMethod_value); found {
		*out = ValidationMethod(v)
		return nil
	}

	return ErrInvalidValidationMethodString
}

func (v ValidationSeverity) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), ValidationSeverity_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidValidationSeverityValue
}

func (out *ValidationSeverity) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, ValidationSeverity_value); found {
		*out = ValidationSeverity(v)
		return nil
	}

	return ErrInvalidValidationSeverityString
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
