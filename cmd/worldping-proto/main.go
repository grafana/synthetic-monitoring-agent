package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/grafana/worldping-blackbox-sidecar/internal/pkg/pb/worldping"
)

func main() {
	doWrite := flag.Bool("write", false, "write example document")

	flag.Parse()

	if *doWrite {
		write()
	} else {
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

func write() {
	check := worldping.Check{
		Id:        123,
		TennantId: 27172,
		Frequency: 5000,
		Offset:    2300,
		Timeout:   2500,
		Enabled:   true,
		Tags:      []string{"production"},
		Settings: worldping.CheckSettings{
			PingSettings: &worldping.PingSettings{
				Hostname:  "www.grafana.com",
				IpVersion: worldping.IpVersion_V4,
				Validation: []worldping.PingCheckValidation{
					{
						ResponseTimeValidation: &worldping.ResponseTimeValidation{
							Threshold: 250,
							Severity:  worldping.ValidationSeverity_Warning,
						},
					},
				},
			},
		},
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
