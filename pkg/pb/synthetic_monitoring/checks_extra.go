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

// Package synthetic_monitoring provides access to types and methods
// that allow for the production and consumption of protocol buffer
// messages used to communicate with synthetic-monitoring-api.
package synthetic_monitoring

//go:generate ../../../scripts/enumer -type=CheckType -trimprefix=CheckType -transform=lower -output=string.go
//go:generate ../../../scripts/enumer -type=MultiHttpEntryAssertionType,MultiHttpEntryAssertionSubjectVariant,MultiHttpEntryAssertionConditionVariant,MultiHttpEntryVariableType -trimprefix=MultiHttpEntryAssertionType_,MultiHttpEntryAssertionSubjectVariant_,MultiHttpEntryAssertionConditionVariant_,MultiHttpEntryVariableType_ -transform=upper -output=multihttp_string.go

import (
	"errors"
	"fmt"
	"mime"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/constraints"
	"golang.org/x/net/http/httpguts"
)

var (
	ErrInvalidTenantId        = errors.New("invalid tenant ID")
	ErrInvalidCheckProbes     = errors.New("invalid check probes")
	ErrInvalidCheckTarget     = errors.New("invalid check target")
	ErrInvalidCheckJob        = errors.New("invalid check job")
	ErrInvalidCheckFrequency  = errors.New("invalid check frequency")
	ErrInvalidCheckTimeout    = errors.New("invalid check timeout")
	ErrInvalidCheckLabelName  = errors.New("invalid check label name")
	ErrInvalidCheckLabelValue = errors.New("invalid check label value")
	ErrInvalidLabelName       = errors.New("invalid label name")
	ErrInvalidLabelValue      = errors.New("invalid label value")
	ErrDuplicateLabelName     = errors.New("duplicate label name")
	ErrInvalidTargetValue     = errors.New("invalid target value")

	ErrInvalidCheckSettings = errors.New("invalid check settings")

	ErrInvalidFQDNLength        = errors.New("invalid FQHN length")
	ErrInvalidFQHNElements      = errors.New("invalid number of elements in FQHN")
	ErrInvalidFQDNElementLength = errors.New("invalid FQHN element length")
	ErrInvalidFQHNElement       = errors.New("invalid FQHN element")

	ErrInvalidPingHostname    = errors.New("invalid ping hostname")
	ErrInvalidPingPayloadSize = errors.New("invalid ping payload size")
	ErrInvalidPingPacketCount = errors.New("invalid ping packet count")

	ErrInvalidDnsName             = errors.New("invalid DNS name")
	ErrInvalidDnsNameElement      = errors.New("invalid DNS name element")
	ErrInvalidDnsServer           = errors.New("invalid DNS server")
	ErrInvalidDnsPort             = errors.New("invalid DNS port")
	ErrInvalidDnsProtocolString   = errors.New("invalid DNS protocol string")
	ErrInvalidDnsProtocolValue    = errors.New("invalid DNS protocol value")
	ErrInvalidDnsRecordTypeString = errors.New("invalid DNS record type string")
	ErrInvalidDnsRecordTypeValue  = errors.New("invalid DNS record type value")

	ErrInvalidHttpUrl                          = errors.New("invalid HTTP URL")
	ErrInvalidHttpMethodString                 = errors.New("invalid HTTP method string")
	ErrInvalidHttpMethodValue                  = errors.New("invalid HTTP method value")
	ErrInvalidHttpUrlHost                      = errors.New("invalid HTTP URL host")
	ErrInvalidHttpHeaders                      = errors.New("invalid HTTP headers")
	ErrInvalidHttpFailIfBodyMatchesRegexp      = errors.New("invalid HTTP fail if body matches regexp")
	ErrInvalidHttpFailIfBodyNotMatchesRegexp   = errors.New("invalid HTTP fail if body not matches regexp")
	ErrInvalidHttpFailIfHeaderMatchesRegexp    = errors.New("invalid HTTP fail if header matches regexp")
	ErrInvalidHttpFailIfHeaderNotMatchesRegexp = errors.New("invalid HTTP fail if header not matches regexp")
	ErrHttpUrlContainsPassword                 = errors.New("HTTP URL contains username and password")
	ErrHttpUrlContainsUsername                 = errors.New("HTTP URL contains username")
	ErrInvalidProxyConnectHeaders              = errors.New("invalid HTTP proxy connect headers")
	ErrInvalidProxyUrl                         = errors.New("invalid proxy URL")
	ErrInvalidProxySettings                    = errors.New("invalid proxy settings")

	ErrInvalidTracerouteHostname = errors.New("invalid traceroute hostname")

	ErrInvalidK6Script = errors.New("invalid K6 script")

	ErrInvalidMultiHttpTargets = errors.New("invalid multi-http targets")

	ErrTooManyMultiHttpTargets         = errors.New("too many multi-http targets")
	ErrTooManyMultiHttpAssertions      = errors.New("too many multi-http assertions")
	ErrTooManyMultiHttpVariables       = errors.New("too many multi-http variables")
	ErrMultiHttpVariableNamesNotUnique = errors.New("multi-http variable names must be unique")

	ErrInvalidHostname = errors.New("invalid hostname")
	ErrInvalidPort     = errors.New("invalid port")

	ErrInvalidIpVersionString = errors.New("invalid ip version string")
	ErrInvalidIpVersionValue  = errors.New("invalid ip version value")

	ErrInvalidCompressionAlgorithmString = errors.New("invalid compression algorithm string")
	ErrInvalidCompressionAlgorithmValue  = errors.New("invalid compression algorithm value")

	ErrInvalidProbeName              = errors.New("invalid probe name")
	ErrInvalidProbeReservedLabelName = errors.New("invalid probe, reserved label name")
	ErrInvalidProbeLabelName         = errors.New("invalid probe label name")
	ErrInvalidProbeLabelValue        = errors.New("invalid probe label value")
	ErrTooManyProbeLabels            = errors.New("too many probe labels")
	ErrInvalidProbeLatitude          = errors.New("invalid probe latitude")
	ErrInvalidProbeLongitude         = errors.New("invalid probe longitude")

	ErrInvalidHttpRequestBodyContentType = errors.New("invalid HTTP request body content type")
	ErrInvalidHttpRequestBodyPayload     = errors.New("invalid HTTP request body payload")
	ErrInvalidQueryFieldName             = errors.New("invalid query field name")

	ErrInvalidMultiHttpAssertion                     = errors.New("invalid multi-http assertion")
	ErrInvalidMultiHttpEntryVariable                 = errors.New("invalid multi-http variable")
	ErrInvalidMultiHttpAssertionMissingValue         = errors.New("invalid multi-http assertion, missing value")
	ErrInvalidMultiHttpAssertionExpressionNotAllowed = errors.New("invalid multi-http assertion, expression not allowed")
	ErrInvalidMultiHttpAssertionMissingHeaderName    = errors.New("invalid multi-http assertion, missing header name")
)

const (
	HealthCheckInterval = 90 * time.Second
	HealthCheckTimeout  = 30 * time.Second
)

const (
	MaxMetricLabels          = 20   // Prometheus allows for 32 labels, but limit to 20.
	MaxLogLabels             = 15   // Loki allows a maximum of 15 labels.
	MaxProbeLabels           = 3    // 3 for probes, leaving 7 for internal use.
	maxValidLabelValueLength = 2048 // This is the actual max label value length.
	MaxLabelValueLength      = 128  // Keep this number low so that the UI remains usable.
	MaxPingPackets           = 10   // Allow 10 packets per ping.
	MaxMultiHttpTargets      = 10   // Max targets per multi-http check.
	MaxMultiHttpAssertions   = 5    // Max assertions per multi-http target.
	MaxMultiHttpVariables    = 5    // Max variables per multi-http target.

	// Frequencies (in milliseconds)
	MaxCheckFrequency      = 1 * 60 * 60 * 1000 // Maximum value for the check's frequency (1 hour).
	minCheckFrequency      = 1 * 1000           // Minimum default value for the check's frequency (1 second).
	minTracerouteFrequency = 120 * 1000         // Minimum value for the traceroute check's frequency (2 min).
	minK6Frequency         = 60 * 1000          // Minimum value for k6-class check's frequency (1 min).

	// Timeouts (in milliseconds)
	minCheckTimeout      = minCheckFrequency
	MaxCheckTimeout      = 1 * 60 * 1000   // Maximum value for the check's timeout (1 minute).
	minScriptedTimeout   = minCheckTimeout // Minimum timeout for scripted checks (1 second).
	maxScriptedTimeout   = MaxCheckTimeout // Maximum timeout for scripted checks (1 minute).
	minTracerouteTimeout = 30 * 1000       // Minimum timeout for traceroute checks (30 second).
	maxTracerouteTimeout = 30 * 1000       // Minimum timeout for traceroute checks (30 second).
)

const (
	// These constants specify the maximum number of labels set by the agent
	// for any metric and log stream for all supported probes, as well as the
	// max number of labels set for the sm_check_info metric.
	// These are constant per agent version but might vary between versions.
	// They can be queried through MaxAgentMetricLabels(), MaxAgentLogLabels()
	// and MaxAgentCheckInfoLabels() exported functions. These are required in
	// order to calculate how many check labels can be set without exceeding
	// specific tenant limits.

	maxAgentMetricLabels    = 9 // Max metric labels set by the agent for any check type
	maxAgentLogLabels       = 7 // Max log labels set by the agent for any check type
	maxAgentCheckInfoLabels = 9 // Max labels set by the agent for sm_check_info metric
)

// MaxAgentMetricLabels returns the maximum number of labels set by the agent
// to any metric.
func MaxAgentMetricLabels() int {
	return maxAgentMetricLabels
}

// MaxAgentLogLabels returns the maximum number of labels set by the agent
// to any log stream.
func MaxAgentLogLabels() int {
	return maxAgentLogLabels
}

// MaxAgentCheckInfoLabels returns the maximum number of labels set by the agent
// for sm_check_info metric.
func MaxAgentCheckInfoLabels() int {
	return maxAgentCheckInfoLabels
}

type validatable interface {
	Validate() error
}

func validateCollection[T validatable](collection []T) error {
	for _, item := range collection {
		if err := item.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// CheckType represents the type of the associated check
type CheckType int32

const (
	CheckTypeDns        CheckType = 0
	CheckTypeHttp       CheckType = 1
	CheckTypePing       CheckType = 2
	CheckTypeTcp        CheckType = 3
	CheckTypeTraceroute CheckType = 4
	CheckTypeScripted   CheckType = 5
	CheckTypeMultiHttp  CheckType = 6
	CheckTypeGrpc       CheckType = 7
	CheckTypeBrowser    CheckType = 8
)

func CheckTypeFromString(in string) (CheckType, bool) {
	ct, err := CheckTypeString(in)
	if err != nil {
		return 0, false
	}

	return ct, true
}

func (c Check) Type() CheckType {
	switch {
	case c.Settings.Dns != nil:
		return CheckTypeDns

	case c.Settings.Http != nil:
		return CheckTypeHttp

	case c.Settings.Ping != nil:
		return CheckTypePing

	case c.Settings.Tcp != nil:
		return CheckTypeTcp

	case c.Settings.Traceroute != nil:
		return CheckTypeTraceroute

	case c.Settings.Scripted != nil:
		return CheckTypeScripted

	case c.Settings.Multihttp != nil:
		return CheckTypeMultiHttp

	case c.Settings.Grpc != nil:
		return CheckTypeGrpc

	case c.Settings.Browser != nil:
		return CheckTypeBrowser

	default:
		panic("unhandled check type")
	}
}

func (c Check) Class() CheckClass {
	return c.Type().Class()
}

func (c CheckType) Class() CheckClass {
	switch c {
	case CheckTypeDns, CheckTypeHttp, CheckTypePing, CheckTypeTcp, CheckTypeTraceroute, CheckTypeGrpc:
		return CheckClass_PROTOCOL

	case CheckTypeScripted, CheckTypeMultiHttp, CheckTypeBrowser: // TODO(mem): does browser belong here?
		return CheckClass_SCRIPTED

	default:
		panic("unhandled check class")
	}
}

func (c Check) Validate() error {
	if c.TenantId == BadID {
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

	if err := c.validateFrequency(); err != nil {
		return err
	}

	if err := c.validateTimeout(); err != nil {
		return err
	}

	if err := validateLabels(c.Labels); err != nil {
		return err
	}

	if err := c.Settings.Validate(); err != nil {
		return err
	}

	if err := c.validateTarget(); err != nil {
		return err
	}

	return nil
}

func (c Check) validateTarget() error {
	// All targets must be valid label values.
	if len(c.Target) > maxValidLabelValueLength {
		return ErrInvalidTargetValue
	}

	switch c.Type() {
	case CheckTypeDns:
		if err := validateDnsTarget(c.Target); err != nil {
			return ErrInvalidDnsName
		}

	case CheckTypeHttp:
		return validateHttpUrl(c.Target)

	case CheckTypePing:
		if err := validateHost(c.Target); err != nil {
			return ErrInvalidPingHostname
		}

	case CheckTypeTcp:
		return validateHostPort(c.Target)

	case CheckTypeTraceroute:
		if err := validateHost(c.Target); err != nil {
			return ErrInvalidTracerouteHostname
		}

	case CheckTypeScripted:
		return nil

	case CheckTypeMultiHttp:
		// TODO(mem): checks MUST have a target, but in this case it's
		// not true that the target must be a valid URL.
		// validation of URLs is the responsibility of the MultihttpEntryRequest
		return nil

	case CheckTypeGrpc:
		return validateHostPort(c.Target)

	case CheckTypeBrowser:
		return nil

	default:
		panic("unhandled check type")
	}

	return nil
}

func (c Check) validateFrequency() error {
	var (
		minFrequency int64 = minCheckFrequency
		maxFrequency int64 = MaxCheckFrequency
	)

	// Some check types have different allowed values for the frequency.

	switch c.Type() {
	case CheckTypeTraceroute:
		minFrequency = minTracerouteFrequency

	case CheckTypeScripted, CheckTypeMultiHttp, CheckTypeBrowser:
		minFrequency = minK6Frequency
	}

	if !inClosedRange(c.Frequency, minFrequency, maxFrequency) {
		return ErrInvalidCheckFrequency
	}

	return nil
}

func (c Check) validateTimeout() error {
	return validateTimeout(c.Type(), c.Timeout, c.Frequency)
}

func validateLabels(labels []Label) error {
	seenLabels := make(map[string]struct{})

	for _, label := range labels {
		if _, found := seenLabels[label.Name]; found {
			return fmt.Errorf("label name %s: %w", label.Name, ErrDuplicateLabelName)
		}

		seenLabels[label.Name] = struct{}{}

		if err := label.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func (c Check) ConfigVersion() string {
	return strconv.FormatInt(int64(c.Modified*1000000000), 10)
}

func (c AdHocCheck) Type() CheckType {
	switch {
	case c.Settings.Dns != nil:
		return CheckTypeDns

	case c.Settings.Http != nil:
		return CheckTypeHttp

	case c.Settings.Ping != nil:
		return CheckTypePing

	case c.Settings.Tcp != nil:
		return CheckTypeTcp

	case c.Settings.Traceroute != nil:
		return CheckTypeTraceroute

	case c.Settings.Scripted != nil:
		return CheckTypeScripted

	case c.Settings.Multihttp != nil:
		return CheckTypeMultiHttp

	case c.Settings.Grpc != nil:
		return CheckTypeGrpc

	case c.Settings.Browser != nil:
		return CheckTypeBrowser

	default:
		panic("unhandled check type")
	}
}

func (c AdHocCheck) Validate() error {
	if c.TenantId < 0 {
		return ErrInvalidTenantId
	}
	if len(c.Probes) == 0 {
		return ErrInvalidCheckProbes
	}
	if len(c.Target) == 0 {
		return ErrInvalidCheckTarget
	}

	if err := c.validateTimeout(); err != nil {
		return err
	}

	if err := c.Settings.Validate(); err != nil {
		return err
	}

	if err := c.validateTarget(); err != nil {
		return err
	}

	return nil
}

func (c AdHocCheck) validateTimeout() error {
	// Ad-hoc checks don't have a frequency, so we pass the timeout value instead.
	return validateTimeout(c.Type(), c.Timeout, c.Timeout)
}

func (c AdHocCheck) validateTarget() error {
	switch c.Type() {
	case CheckTypeDns:
		if err := validateDnsTarget(c.Target); err != nil {
			return ErrInvalidDnsName
		}

	case CheckTypeHttp:
		return validateHttpUrl(c.Target)

	case CheckTypePing:
		if err := validateHost(c.Target); err != nil {
			return ErrInvalidPingHostname
		}

	case CheckTypeTcp:
		return validateHostPort(c.Target)

	case CheckTypeTraceroute:
		if err := validateHost(c.Target); err != nil {
			return ErrInvalidTracerouteHostname
		}

	case CheckTypeScripted:
		return nil

	case CheckTypeMultiHttp:
		return nil

	case CheckTypeGrpc:
		return validateHostPort(c.Target)

	case CheckTypeBrowser:
		return nil

	default:
		panic("unhandled check type")
	}

	return nil
}

func (s CheckSettings) Validate() error {
	var validateFn func() error

	settingsCount := 0

	if s.Ping != nil {
		settingsCount++
		validateFn = s.Ping.Validate
	}

	if s.Http != nil {
		settingsCount++
		validateFn = s.Http.Validate
	}

	if s.Dns != nil {
		settingsCount++
		validateFn = s.Dns.Validate
	}

	if s.Tcp != nil {
		settingsCount++
		validateFn = s.Tcp.Validate
	}

	if s.Traceroute != nil {
		settingsCount++
		validateFn = s.Traceroute.Validate
	}

	if s.Scripted != nil {
		settingsCount++
		validateFn = s.Scripted.Validate
	}

	if s.Multihttp != nil {
		settingsCount++
		validateFn = s.Multihttp.Validate
	}

	if s.Grpc != nil {
		settingsCount++
		validateFn = s.Grpc.Validate
	}

	if settingsCount != 1 {
		return ErrInvalidCheckSettings
	}

	return validateFn()
}

func (s *PingSettings) Validate() error {
	if s.PayloadSize < 0 || s.PayloadSize > 65499 {
		return ErrInvalidPingPayloadSize
	}

	if s.PacketCount < 0 || s.PacketCount > MaxPingPackets {
		return ErrInvalidPingPacketCount
	}

	return nil
}

//nolint:gocyclo
func (s *HttpSettings) Validate() error {
	for _, h := range s.Headers {
		fields := strings.SplitN(h, ":", 2)
		if len(fields) < 2 {
			return ErrInvalidHttpHeaders
		}

		// remove optional leading and trailing whitespace
		fields[1] = strings.TrimSpace(fields[1])

		if !httpguts.ValidHeaderFieldName(fields[0]) {
			return ErrInvalidHttpHeaders
		}

		if !httpguts.ValidHeaderFieldValue(fields[1]) {
			return ErrInvalidHttpHeaders
		}
	}

	if len(s.ProxyURL) > 0 {
		u, err := url.Parse(s.ProxyURL)
		if err != nil {
			return ErrInvalidProxyUrl
		}

		if !(u.Scheme == "http" || u.Scheme == "https") {
			return ErrInvalidProxyUrl
		}
	}

	if len(s.ProxyConnectHeaders) > 0 && len(s.ProxyURL) == 0 {
		return ErrInvalidProxySettings
	}

	for _, h := range s.ProxyConnectHeaders {
		fields := strings.SplitN(h, ":", 2)
		if len(fields) < 2 {
			return ErrInvalidProxyConnectHeaders
		}

		// remove optional leading and trailing whitespace
		fields[1] = strings.TrimSpace(fields[1])

		if !httpguts.ValidHeaderFieldName(fields[0]) {
			return ErrInvalidProxyConnectHeaders
		}

		if !httpguts.ValidHeaderFieldValue(fields[1]) {
			return ErrInvalidProxyConnectHeaders
		}
	}

	for _, reg := range s.FailIfBodyMatchesRegexp {
		_, err := regexp.Compile(reg)
		if err != nil {
			return ErrInvalidHttpFailIfBodyMatchesRegexp
		}
	}

	for _, reg := range s.FailIfBodyNotMatchesRegexp {
		_, err := regexp.Compile(reg)
		if err != nil {
			return ErrInvalidHttpFailIfBodyNotMatchesRegexp
		}
	}

	for _, match := range s.FailIfHeaderMatchesRegexp {
		_, err := regexp.Compile(match.Regexp)
		if err != nil {
			return ErrInvalidHttpFailIfHeaderMatchesRegexp
		}
	}

	for _, match := range s.FailIfHeaderNotMatchesRegexp {
		_, err := regexp.Compile(match.Regexp)
		if err != nil {
			return ErrInvalidHttpFailIfHeaderNotMatchesRegexp
		}
	}

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

func (s *TracerouteSettings) Validate() error {
	return nil
}

func (s *ScriptedSettings) Validate() error {
	if len(s.Script) == 0 {
		return ErrInvalidK6Script
	}

	return nil
}

func (s *MultiHttpSettings) Validate() error {
	if len(s.Entries) == 0 {
		return ErrInvalidMultiHttpTargets
	}

	if len(s.Entries) > MaxMultiHttpTargets {
		return ErrTooManyMultiHttpTargets
	}

	if err := validateCollection(s.Entries); err != nil {
		return err
	}

	return nil
}

func (s *GrpcSettings) Validate() error {
	return nil
}

func (s *BrowserSettings) Validate() error {
	if len(s.Script) == 0 {
		return ErrInvalidK6Script
	}

	return nil
}

func hasUniqueValues[U any, V comparable](slice []U, fn func(U) V) bool {
	set := make(map[V]struct{})

	for _, elem := range slice {
		value := fn(elem)
		if _, found := set[value]; found {
			return false
		}
		set[value] = struct{}{}
	}

	return true
}

func (e *MultiHttpEntry) Validate() error {
	if e.Request == nil {
		return ErrInvalidMultiHttpTargets
	}

	if err := e.Request.Validate(); err != nil {
		return err
	}

	if len(e.Assertions) > MaxMultiHttpAssertions {
		return ErrTooManyMultiHttpAssertions
	}

	if len(e.Variables) > MaxMultiHttpVariables {
		return ErrTooManyMultiHttpVariables
	}

	if err := validateCollection(e.Assertions); err != nil {
		return err
	}

	if err := validateCollection(e.Variables); err != nil {
		return err
	}

	if !hasUniqueValues(e.Variables, func(v *MultiHttpEntryVariable) string { return v.Name }) {
		return ErrMultiHttpVariableNamesNotUnique
	}

	return nil
}

func (h HttpHeader) Validate() error {
	if !httpguts.ValidHeaderFieldName(h.Name) {
		return ErrInvalidHttpHeaders
	}

	if !httpguts.ValidHeaderFieldValue(h.Value) {
		return ErrInvalidHttpHeaders
	}

	return nil
}

func (f QueryField) Validate() error {
	if len(f.Name) == 0 {
		return ErrInvalidQueryFieldName
	}

	// the value might be empty

	// The name can be anything. TODO(mem): is this true?

	return nil
}

func (r *MultiHttpEntryRequest) Validate() error {
	if r == nil {
		return nil
	}

	if err := r.Method.Validate(); err != nil {
		return err
	}

	if !strings.Contains(r.Url, "${") {
		if err := validateHttpUrl(r.Url); err != nil {
			return err
		}
	}

	// TODO(mem): do something with HttpVersion?

	if err := r.Body.Validate(); err != nil {
		return err
	}

	if err := validateCollection(r.Headers); err != nil {
		return err
	}

	if err := validateCollection(r.QueryFields); err != nil {
		return err
	}

	return nil
}

// Validate verifies that the MultiHttpEntryAssertion is valid.
//
// Because of the structure represents multiple orthogonal variants, this
// function has to branch based on the type.
//
//nolint:gocyclo
func (a *MultiHttpEntryAssertion) Validate() error {
	if a == nil {
		return nil
	}

	if _, found := MultiHttpEntryAssertionType_name[int32(a.Type)]; !found {
		// this should never happen
		return ErrInvalidMultiHttpAssertion
	}

	if _, found := MultiHttpEntryAssertionSubjectVariant_name[int32(a.Subject)]; !found {
		// this should never happen
		return ErrInvalidMultiHttpAssertion
	}

	if _, found := MultiHttpEntryAssertionConditionVariant_name[int32(a.Condition)]; !found {
		// this should never happen
		return ErrInvalidMultiHttpAssertion
	}

	switch a.Type {
	case MultiHttpEntryAssertionType_TEXT:
		// Value is required
		if len(a.Value) == 0 {
			return ErrInvalidMultiHttpAssertionMissingValue
		}

		// Expression is not allowed for subjects other than response headers.
		if a.Subject != MultiHttpEntryAssertionSubjectVariant_RESPONSE_HEADERS && len(a.Expression) != 0 {
			return ErrInvalidMultiHttpAssertionExpressionNotAllowed
		}

	case MultiHttpEntryAssertionType_JSON_PATH_VALUE:
		// Subject must not be set
		if a.Subject != 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Value is required
		if len(a.Value) == 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Expression is required
		if len(a.Expression) == 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Condition is covered above

	case MultiHttpEntryAssertionType_JSON_PATH_ASSERTION:
		// Subject must not be set
		if a.Subject != 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Condition must not be set
		if a.Condition != 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Value must not be set
		if len(a.Value) != 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Expression is required
		if len(a.Expression) == 0 {
			return ErrInvalidMultiHttpAssertion
		}

	case MultiHttpEntryAssertionType_REGEX_ASSERTION:
		// Condition must not be set
		if a.Condition != 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Value must not be set
		if len(a.Value) != 0 {
			return ErrInvalidMultiHttpAssertion
		}

		// Expression is required
		if len(a.Expression) == 0 {
			return ErrInvalidMultiHttpAssertion
		}
	}

	return nil
}

func (v *MultiHttpEntryVariable) Validate() error {
	// 1. Type is valid
	if _, found := MultiHttpEntryVariableType_name[int32(v.Type)]; !found {
		return ErrInvalidMultiHttpEntryVariable
	}

	// 2. Name is not empty
	if len(v.Name) == 0 {
		return ErrInvalidMultiHttpEntryVariable
	}

	// 3. Expression is not empty
	if len(v.Expression) == 0 {
		return ErrInvalidMultiHttpEntryVariable
	}

	switch v.Type {
	case MultiHttpEntryVariableType_JSON_PATH:
		// 4. attribute must be empty
		if len(v.Attribute) != 0 {
			return ErrInvalidMultiHttpEntryVariable
		}

	case MultiHttpEntryVariableType_REGEX:
		// 4. attribute must be empty
		if len(v.Attribute) != 0 {
			return ErrInvalidMultiHttpEntryVariable
		}

	case MultiHttpEntryVariableType_CSS_SELECTOR:
		// 4. attribute might be empty
	}

	return nil
}

func (b *HttpRequestBody) Validate() error {
	if b == nil {
		return nil
	}

	if len(b.ContentType) == 0 {
		return ErrInvalidHttpRequestBodyContentType
	}

	if !httpguts.ValidHeaderFieldValue(b.ContentType) {
		return ErrInvalidHttpRequestBodyContentType
	}

	for _, v := range strings.Split(b.ContentType, ",") {
		_, _, err := mime.ParseMediaType(v)
		if err != nil {
			return ErrInvalidHttpRequestBodyContentType
		}
	}

	if len(b.ContentEncoding) > 0 && !httpguts.ValidHeaderFieldValue(b.ContentEncoding) {
		return ErrInvalidHttpRequestBodyContentType
	}

	// Payload can be empty, since Content-Length can be 0.
	// https://datatracker.ietf.org/doc/html/rfc9110#section-8.6

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
		if err := label.Validate(); err != nil {
			return err
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

func (l Label) Validate() error {
	if err := validateLabelValue(l.Name); err != nil {
		return ErrInvalidLabelName
	}

	// This bit is lifted from Prometheus code, except that
	// Prometheus accepts /^[a-zA-Z_][a-zA-Z0-9_]*$/ and we accept
	// /^[a-zA-Z0-9_]+$/ because these names are going to be
	// prefixed with "label_".
	for _, b := range l.Name {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || (b >= '0' && b <= '9')) {
			return ErrInvalidLabelName
		}
	}

	return validateLabelValue(l.Value)
}

func validateLabelValue(v string) error {
	if len(v) == 0 || len(v) > MaxLabelValueLength {
		return ErrInvalidLabelValue
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
	str := string(b)

	switch str {
	case ``:
		return 0, true
	case `""`:
		return 0, true
	case `null`:
		return 0, true
	}

	in, err := strconv.Unquote(str)
	if err != nil {
		return 0, false
	}

	// first try a direct lookup in the known values
	if v, ok := m[in]; ok {
		return v, true
	}

	// not found, try again doing an case-insensitive search

	in = strings.ToLower(in)

	for str, v := range m {
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

// ToIpProtocol converts the IpVersion setting into a pair of IP
// protocol and fallback option.
func (v IpVersion) ToIpProtocol() (string, bool) {
	switch v {
	case IpVersion_V4:
		return "ip4", false

	case IpVersion_V6:
		return "ip6", false

	case IpVersion_Any:
		return "ip6", true
	}

	return "", false
}

func (v CompressionAlgorithm) MarshalJSON() ([]byte, error) {
	if b := lookupValue(int32(v), CompressionAlgorithm_name); b != nil {
		return b, nil
	}

	return nil, ErrInvalidCompressionAlgorithmValue
}

func (out *CompressionAlgorithm) UnmarshalJSON(b []byte) error {
	if v, found := lookupString(b, CompressionAlgorithm_value); found {
		*out = CompressionAlgorithm(v)
		return nil
	}

	return ErrInvalidCompressionAlgorithmString
}

func (v HttpMethod) Validate() error {
	if _, found := HttpMethod_name[int32(v)]; !found {
		return ErrInvalidHttpMethodValue
	}

	return nil
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

// validateDnsTarget checks that the provided target is a valid DNS
// target, meaning it's either "localhost" exactly or a fully qualified
// domain name (with a full stop at the end). To accept something like
// "org" it has to be specified as "org.".
func validateDnsTarget(target string) error {
	labels := strings.Split(target, ".")
	switch len(labels) {
	case 1:
		if target == "localhost" {
			return nil
		}

		// no dots, not "localhost", this is invalid
		return ErrInvalidDnsName

	default:
		if labels[len(labels)-1] == "" {
			// last label is empty, so the target is of the
			// form "foo.bar."; drop the last label
			labels = labels[:len(labels)-1]
		}

		for i, label := range labels {
			err := validateDnsLabel(label, i == len(labels)-1)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func validateHostPort(target string) error {
	if host, port, err := net.SplitHostPort(target); err != nil {
		return ErrInvalidCheckTarget
	} else if validateHost(host) != nil {
		return ErrInvalidHostname
	} else if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return ErrInvalidPort
	}

	return nil
}

func validateHttpUrl(target string) error {
	if len(target) != len(strings.TrimSpace(target)) {
		return ErrInvalidHttpUrl
	}

	u, err := url.Parse(target)
	if err != nil {
		return ErrInvalidHttpUrl
	}

	if _, isSet := u.User.Password(); isSet {
		return ErrHttpUrlContainsPassword
	}

	if u.User.Username() != "" {
		return ErrHttpUrlContainsUsername
	}

	if !(u.Scheme == "http" || u.Scheme == "https") {
		return ErrInvalidHttpUrl
	}

	if len(u.Host) == 0 {
		return ErrInvalidHostname
	}

	hasPort := func(h string) bool {
		for l := len(h) - 1; l > 0; l-- {
			if h[l] == ':' {
				return true
			} else if h[l] == ']' || h[l] == '.' {
				return false
			}
		}

		return false
	}

	hostport := u.Host

	if (u.Host[0] == '[' && u.Host[len(u.Host)-1] == ']') || !hasPort(u.Host) {
		if u.Scheme == "https" {
			hostport += ":443"
		} else {
			hostport += ":80"
		}
	}

	return validateHostPort(hostport)
}

// checkFQHN validates that the provided fully qualified hostname
// follows RFC 1034, section 3.5
// (https://tools.ietf.org/html/rfc1034#section-3.5) and RFC 1123,
// section 2.1 (https://tools.ietf.org/html/rfc1123#section-2.1).
//
// This assumes that the *hostname* part of the FQHN follows the same
// rules.
//
// Note that if there are any IDNA transformations going on, they need
// to happen _before_ calling this function.
func checkFQHN(fqhn string) error {
	if len(fqhn) == 0 || len(fqhn) > 255 {
		return ErrInvalidFQDNLength
	}

	labels := strings.Split(fqhn, ".")

	if len(labels) < 2 {
		return ErrInvalidFQHNElements
	}

	for i, label := range labels {
		err := validateFQHNLabel(label, i == len(labels)-1)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateFQHNLabel(label string, isLast bool) error {
	isLetter := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}

	isDigit := func(r rune) bool {
		return (r >= '0' && r <= '9')
	}

	isDash := func(r rune) bool {
		return (r == '-')
	}

	if len(label) == 0 || len(label) > 63 {
		return ErrInvalidFQDNElementLength
	}

	runes := []rune(label)

	// labels must start with a letter or digit (RFC 1123);
	// reading the RFC strictly, it's likely that the
	// intention was that _only_ the host name could begin
	// with a letter or a digit, but since any portion of
	// the FQHN could be a host name, accept it anywhere.
	if r := runes[0]; !isLetter(r) && !isDigit(r) {
		return ErrInvalidFQHNElement
	}

	// labels must end with a letter or digit
	if r := runes[len(runes)-1]; !isLetter(r) && !isDigit(r) {
		return ErrInvalidFQHNElement
	}

	// these checks allow for all-numeric FQHNs, but the
	// very last label (the TLD) MUST NOT be all numeric
	// because that allows for 256.256.256.256 to be a FQHN,
	// not an invalid IP address, and down that path lies
	// madness.
	if isLast {
		allDigits := true

		for _, r := range runes {
			if !isDigit(r) {
				allDigits = false
				break
			}
		}

		if allDigits {
			return ErrInvalidFQHNElement
		}
	}

	for _, r := range runes {
		// the only valid characters are [-A-Za-z0-9].
		if !isLetter(r) && !isDigit(r) && !isDash(r) {
			return ErrInvalidFQHNElement
		}
	}

	return nil
}

// validateDnsLabel checks that `label` conforms to a minimal set of rules
// regarding the components of a DNS entry, namely that they are not empty,
// they are not longer than 63 characters. This follows RFC 2181.
func validateDnsLabel(label string, isLast bool) error {
	// We are looking at a UTF-8 string, the len function reports the
	// number of ASCII characters, not the number of runes.
	if len(label) == 0 || len(label) > 63 {
		return ErrInvalidDnsNameElement
	}

	return nil
}

func validateTimeout(checkType CheckType, timeout, frequency int64) error {
	var minTimeout, maxTimeout int64

	switch checkType {
	case CheckTypeTraceroute:
		minTimeout, maxTimeout = minTracerouteTimeout, min(frequency, maxTracerouteTimeout)

	case CheckTypeScripted, CheckTypeMultiHttp, CheckTypeBrowser:
		// This is experimental. A large timeout means we have more
		// checks lingering around. timeout must be less or equal than
		// frequency (otherwise we can end up running overlapping
		// checks)
		minTimeout, maxTimeout = minScriptedTimeout, min(frequency, maxScriptedTimeout)

	default:
		// timeout must be within the defined limits, and it must be
		// less than frequency (otherwise we can end up running
		// overlapping checks)
		minTimeout, maxTimeout = minCheckTimeout, min(frequency, MaxCheckTimeout)
	}

	if !inClosedRange(timeout, minTimeout, maxTimeout) {
		return ErrInvalidCheckTimeout
	}

	return nil
}

// inClosedRange returns true if the value `v` is in [lower, upper].
func inClosedRange[T constraints.Ordered](v, lower, upper T) bool {
	return v >= lower && v <= upper
}
