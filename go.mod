module github.com/grafana/synthetic-monitoring-agent

go 1.24.0

toolchain go1.24.6

require (
	github.com/go-kit/kit v0.13.0
	github.com/go-logfmt/logfmt v0.6.1
	github.com/gogo/googleapis v1.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v1.0.0
	github.com/google/uuid v1.6.0
	github.com/miekg/dns v1.1.68
	github.com/mmcloughlin/geohash v0.10.0
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/blackbox_exporter v0.27.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.66.1
	github.com/prometheus/prometheus v0.305.0
	github.com/rs/zerolog v1.34.0
	github.com/stretchr/testify v1.11.1
	github.com/tonobo/mtr v0.1.1-0.20210422192847-1c17592ae70b
	golang.org/x/net v0.47.0
	golang.org/x/sync v0.18.0
	google.golang.org/grpc v1.77.0
)

require (
	github.com/KimMachineGun/automemlimit v0.7.5
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/bradfitz/gomemcache v0.0.0-20250403215159-8d39553ac7cf
	github.com/felixge/httpsnoop v1.0.4
	github.com/go-kit/log v0.2.1
	github.com/gogo/status v1.1.1
	github.com/grafana/gsm-api-go-client v0.2.1
	github.com/grafana/loki/pkg/push v0.0.0-20250903135404-0b2d0b070e96
	github.com/jpillora/backoff v1.0.0
	github.com/maypok86/otter/v2 v2.2.1
	github.com/mccutchen/go-httpbin/v2 v2.19.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus-community/pro-bing v0.7.0
	github.com/puzpuzpuz/xsync/v4 v4.2.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.23
	github.com/spf13/afero v1.15.0
	golang.org/x/exp v0.0.0-20251113190631-e25ba8c21ef6
	gopkg.in/yaml.v3 v3.0.1
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.77
)

require (
	cel.dev/expr v0.24.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/cel-go v0.25.0 // indirect
	github.com/grafana/regexp v0.0.0-20240518133315-a468a5bfb3bc // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.k6.io/k6 v1.3.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/oauth2 v0.32.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.77 // indirect
)

replace github.com/tonobo/mtr => github.com/grafana/mtr v0.1.1-0.20221107202107-a9806fdda166
