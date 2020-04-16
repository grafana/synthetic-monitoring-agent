package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/grafana/worldping-api/pkg/pb/worldping"
)

type checkType int

const (
	CHECK_PING checkType = iota
	CHECK_HTTP
	CHECK_DNS
)

func main() {
	doWritePing := flag.Bool("write-ping", false, "write example ping document")
	doWriteHttp := flag.Bool("write-http", false, "write example http document")
	doWriteDns := flag.Bool("write-dns", false, "write example dns document")

	flag.Parse()

	switch {
	case *doWritePing:
		write(CHECK_PING)
	case *doWriteHttp:
		write(CHECK_HTTP)
	case *doWriteDns:
		write(CHECK_DNS)
	default:
		read()
	}
}

func read() {
	input, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Printf("E: Cannot read input: %s", err)
		os.Exit(1)
	}

	var check worldping.Check

	if err := json.Unmarshal(input, &check); err != nil {
		log.Printf("E: Cannot unmarshal input: %s", err)
		os.Exit(1)
	}

	if err := check.Validate(); err != nil {
		log.Printf("Input is invalid: %s", err)
		return
	}

	log.Println("Input is valid")
}

func write(checkType checkType) {
	var settings worldping.CheckSettings

	switch checkType {
	case CHECK_PING:
		settings = worldping.CheckSettings{
			Ping: &worldping.PingSettings{
				Hostname:  "grafana.com",
				IpVersion: worldping.IpVersion_V4,
				Validation: []worldping.PingCheckValidation{
					{
						ResponseTime: &worldping.ResponseTimeValidation{
							Threshold: 250,
							Severity:  worldping.ValidationSeverity_Warning,
						},
					},
				},
			},
		}

	case CHECK_HTTP:
		settings = worldping.CheckSettings{
			Http: &worldping.HttpSettings{
				Url:          "https://grafana.com/",
				Method:       worldping.HttpMethod_GET,
				IpVersion:    worldping.IpVersion_V4,
				ValidateCert: true,
				Validation: []worldping.HttpCheckValidations{
					{
						ResponseTime: &worldping.ResponseTimeValidation{
							Threshold: 250,
							Severity:  worldping.ValidationSeverity_Warning,
						},
					},
				},
			},
		}

	case CHECK_DNS:
		settings = worldping.CheckSettings{
			Dns: &worldping.DnsSettings{
				Name:       "grafana.com",
				RecordType: worldping.DnsRecordType_A,
				Server:     "8.8.4.4",
				IpVersion:  worldping.IpVersion_V4,
				Protocol:   worldping.DnsProtocol_TCP,
				Port:       53,
				Validation: []worldping.DNSCheckValidation{
					{
						ResponseTime: &worldping.ResponseTimeValidation{
							Threshold: 250,
							Severity:  worldping.ValidationSeverity_Warning,
						},
					},
				},
			},
		}
	}

	check := worldping.Check{
		Id:        123,
		TenantId:  27172,
		Frequency: 5000,
		Offset:    2300,
		Timeout:   2500,
		Enabled:   true,
		Labels:    []worldping.Label{{Name: "environment", Value: "production"}},
		Settings:  settings,
	}

	if err := check.Validate(); err != nil {
		log.Printf("Invalid check: %s", err)
		return
	}

	out, err := json.Marshal(&check)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(out))
}
