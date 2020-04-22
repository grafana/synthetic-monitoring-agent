module github.com/grafana/worldping-blackbox-sidecar

go 1.13

require (
	github.com/cortexproject/cortex v1.0.0 // indirect
	github.com/go-logfmt/logfmt v0.5.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.1
	github.com/grafana/loki v6.7.8+incompatible
	github.com/grafana/worldping-api v0.0.0-20200422122327-cabd52ca5303
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/blackbox_exporter v0.16.0
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.9.1
	github.com/prometheus/prometheus v1.8.2-0.20200213233353-b90be6f32a33
	google.golang.org/grpc v1.25.1
	gopkg.in/yaml.v2 v2.2.8
)

replace github.com/Azure/azure-sdk-for-go => github.com/Azure/azure-sdk-for-go v36.2.0+incompatible

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.0+incompatible
