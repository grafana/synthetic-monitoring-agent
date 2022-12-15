package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
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
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Printf("E: Cannot read input: %s", err)
		os.Exit(1)
	}

	var check sm.Check

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
	var (
		settings sm.CheckSettings
		target   string
	)

	switch checkType {
	case CHECK_PING:
		target = "grafana.com"
		settings = sm.CheckSettings{
			Ping: &sm.PingSettings{
				IpVersion: sm.IpVersion_V4,
			},
		}

	case CHECK_HTTP:
		target = "https://grafana.com/"
		settings = sm.CheckSettings{
			Http: &sm.HttpSettings{
				Method:       sm.HttpMethod_GET,
				IpVersion:    sm.IpVersion_V4,
				FailIfNotSSL: true,
			},
		}

	case CHECK_DNS:
		target = "grafana.com"
		settings = sm.CheckSettings{
			Dns: &sm.DnsSettings{
				RecordType: sm.DnsRecordType_A,
				Server:     "8.8.4.4",
				IpVersion:  sm.IpVersion_V4,
				Protocol:   sm.DnsProtocol_TCP,
				Port:       53,
			},
		}
	}

	check := sm.Check{
		Id:        123,
		TenantId:  27172,
		Frequency: 5000,
		Offset:    2300,
		Timeout:   2500,
		Enabled:   true,
		Labels:    []sm.Label{{Name: "environment", Value: "production"}},
		Target:    target,
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
