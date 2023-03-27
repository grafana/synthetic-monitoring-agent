module github.com/grafana/synthetic-monitoring-agent

go 1.18

require (
	github.com/go-kit/kit v0.12.0
	github.com/go-logfmt/logfmt v0.6.0
	github.com/gogo/googleapis v1.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v0.0.4
	github.com/google/uuid v1.3.0
	github.com/miekg/dns v1.1.50
	github.com/mmcloughlin/geohash v0.10.0
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/blackbox_exporter v0.23.1-0.20221215210014-35f2661a935a
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/client_model v0.3.0
	github.com/prometheus/common v0.42.0
	github.com/prometheus/prometheus v0.42.0
	github.com/rs/zerolog v1.29.0
	github.com/stretchr/testify v1.8.2
	github.com/tonobo/mtr v0.1.1-0.20210422192847-1c17592ae70b
	golang.org/x/net v0.8.0
	golang.org/x/sync v0.1.0
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/grpc v1.54.0
)

require (
	github.com/go-kit/log v0.2.1
	github.com/go-ping/ping v0.0.0-20211130115550-779d1e919534
	github.com/gogo/status v1.1.1
	github.com/jpillora/backoff v1.0.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.22
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.68
)

require (
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v0.0.0-20181115115552-c206103e1f37 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grafana/regexp v0.0.0-20221122212121-6b5c0a4cb7fd // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/goleak v1.2.0 // indirect
	golang.org/x/exp v0.0.0-20230124195608-d38c7dcee874 // indirect
	golang.org/x/mod v0.8.0 // indirect
	golang.org/x/oauth2 v0.5.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.6.0 // indirect
	google.golang.org/genproto v0.0.0-20230124163310-31e0e69b6fc2 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.68 // indirect
)

replace github.com/Azure/azure-sdk-for-go => github.com/Azure/azure-sdk-for-go v36.2.0+incompatible

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.0+incompatible

// Without the following replace, you get an error like
//
//     k8s.io/client-go@v12.0.0+incompatible: invalid version: +incompatible suffix not allowed: module contains a go.mod file, so semantic import versioning is required
//
// This is telling you that you cannot have a version 12.0.0 and tag
// that as "incompatible", that you should be calling the module
// something like "k8s.io/client-go/v12".
//
// 78d2af792bab is the commit tagged as v12.0.0.

replace k8s.io/client-go => k8s.io/client-go v0.22.1

// replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab

replace github.com/tonobo/mtr => github.com/grafana/mtr v0.1.1-0.20221107202107-a9806fdda166
