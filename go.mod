module github.com/grafana/synthetic-monitoring-agent

go 1.26.0

require (
	github.com/go-kit/kit v0.13.0
	github.com/go-logfmt/logfmt v0.6.1
	github.com/gogo/googleapis v1.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v1.0.0
	github.com/google/uuid v1.6.0
	github.com/miekg/dns v1.1.73
	github.com/mmcloughlin/geohash v0.10.0
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/blackbox_exporter v0.28.0
	github.com/prometheus/client_golang v1.24.1
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.70.1
	github.com/prometheus/prometheus v0.314.0
	github.com/rs/zerolog v1.35.1
	github.com/stretchr/testify v1.12.1
	github.com/tonobo/mtr v0.1.1-0.20210422192847-1c17592ae70b
	golang.org/x/net v0.58.0
	golang.org/x/sync v0.22.0
	google.golang.org/grpc v1.83.1
)

require (
	github.com/KimMachineGun/automemlimit v0.7.5
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/bradfitz/gomemcache v0.0.0-20260422231931-4d751bb6e37c
	github.com/felixge/httpsnoop v1.1.0
	github.com/go-kit/log v0.2.1
	github.com/gogo/status v1.1.1
	github.com/grafana/gsm-api-go-client v0.3.4
	github.com/grafana/loki/pkg/push v0.0.0-20250903135404-0b2d0b070e96
	github.com/jpillora/backoff v1.0.0
	github.com/maypok86/otter/v2 v2.3.0
	github.com/mccutchen/go-httpbin/v2 v2.25.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus-community/pro-bing v0.9.1
	github.com/puzpuzpuz/xsync/v4 v4.5.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.23
	github.com/spf13/afero v1.15.0
	go.opentelemetry.io/collector/client v1.65.0
	go.opentelemetry.io/collector/component v1.65.0
	go.opentelemetry.io/collector/component/componenttest v0.159.0
	go.opentelemetry.io/collector/config/configgrpc v1.65.0
	go.opentelemetry.io/collector/config/confighttp v0.159.0
	go.opentelemetry.io/collector/config/configopaque v1.65.0
	go.opentelemetry.io/collector/config/configoptional v1.65.0
	go.opentelemetry.io/collector/consumer v1.65.0
	go.opentelemetry.io/collector/exporter v1.65.0
	go.opentelemetry.io/collector/exporter/exporterhelper v0.159.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.159.0
	go.opentelemetry.io/collector/pdata v1.65.0
	go.opentelemetry.io/collector/receiver v1.65.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.159.0
	go.uber.org/zap v1.28.0
	golang.org/x/exp v0.0.0-20260820142414-ca536658362e
	gopkg.in/yaml.v3 v3.0.1
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.78
)

require (
	cel.dev/expr v0.25.2 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/cenkalti/backoff/v7 v7.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/foxboron/go-tpm-keyfiles v0.0.0-20251226215517-609e4778396f // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/cel-go v0.30.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/knadh/koanf/maps v0.1.3 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.1 // indirect
	github.com/knadh/koanf/v2 v2.3.6 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.23 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oapi-codegen/runtime v1.6.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pierrec/lz4/v4 v4.1.28 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector v0.159.0 // indirect
	go.opentelemetry.io/collector/component/componentstatus v0.159.0 // indirect
	go.opentelemetry.io/collector/config/configauth v1.65.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.65.0 // indirect
	go.opentelemetry.io/collector/config/configmiddleware v1.65.0 // indirect
	go.opentelemetry.io/collector/config/confignet v1.65.0 // indirect
	go.opentelemetry.io/collector/config/configretry v1.65.0 // indirect
	go.opentelemetry.io/collector/config/configtls v1.65.0 // indirect
	go.opentelemetry.io/collector/confmap v1.65.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.159.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.159.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.159.0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.159.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.159.0 // indirect
	go.opentelemetry.io/collector/extension v1.65.0 // indirect
	go.opentelemetry.io/collector/extension/extensionauth v1.65.0 // indirect
	go.opentelemetry.io/collector/extension/extensionmiddleware v0.159.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.159.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.65.0 // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.159.0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.159.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.159.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.159.0 // indirect
	go.opentelemetry.io/collector/pdata/xpdata v0.159.0 // indirect
	go.opentelemetry.io/collector/pipeline v1.65.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.159.0 // indirect
	go.opentelemetry.io/collector/receiver/receiverhelper v0.159.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.159.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.70.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.70.0 // indirect
	go.opentelemetry.io/otel v1.45.0 // indirect
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/sdk v1.45.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260724162435-b2f20204f0df // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.78 // indirect
)

replace github.com/tonobo/mtr => github.com/grafana/mtr v0.1.1-0.20221107202107-a9806fdda166
