module github.com/grafana/synthetic-monitoring-agent

go 1.24.0

toolchain go1.24.6

require (
	github.com/go-kit/kit v0.13.0
	github.com/go-logfmt/logfmt v0.6.0
	github.com/gogo/googleapis v1.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v1.0.0
	github.com/google/uuid v1.6.0
	github.com/miekg/dns v1.1.68
	github.com/mmcloughlin/geohash v0.10.0
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/blackbox_exporter v0.27.0
	github.com/prometheus/client_golang v1.23.0
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.66.0
	github.com/prometheus/prometheus v0.305.0
	github.com/rs/zerolog v1.34.0
	github.com/stretchr/testify v1.11.1
	github.com/tonobo/mtr v0.1.1-0.20210422192847-1c17592ae70b
	golang.org/x/net v0.43.0
	golang.org/x/sync v0.16.0
	google.golang.org/grpc v1.75.0
)

require (
	github.com/KimMachineGun/automemlimit v0.7.4
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/felixge/httpsnoop v1.0.4
	github.com/go-kit/log v0.2.1
	github.com/gogo/status v1.1.1
	github.com/grafana/loki/pkg/push v0.0.0-20241004191050-c2f38e18c6b8
	github.com/jpillora/backoff v1.0.0
	github.com/mccutchen/go-httpbin/v2 v2.18.3
	github.com/prometheus-community/pro-bing v0.7.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.22
	github.com/spf13/afero v1.14.0
	golang.org/x/exp v0.0.0-20250819193227-8b4c13bb791b
	gopkg.in/yaml.v3 v3.0.1
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.76
)

require (
	cel.dev/expr v0.24.0 // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/cel-go v0.25.0 // indirect
	github.com/grafana/regexp v0.0.0-20240518133315-a468a5bfb3bc // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.76 // indirect
)

replace github.com/tonobo/mtr => github.com/grafana/mtr v0.1.1-0.20221107202107-a9806fdda166
