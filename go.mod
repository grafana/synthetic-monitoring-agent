module github.com/grafana/synthetic-monitoring-agent

go 1.18

require (
	github.com/OneOfOne/xxhash v1.2.6 // indirect
	github.com/go-kit/kit v0.12.0
	github.com/go-logfmt/logfmt v0.5.1
	github.com/gogo/googleapis v1.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v0.0.4
	github.com/google/uuid v1.3.0
	github.com/miekg/dns v1.1.50
	github.com/mmcloughlin/geohash v0.10.0
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/blackbox_exporter v0.21.0
	github.com/prometheus/client_golang v1.13.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.37.0
	// This is actually version v2.16.0
	//
	// Without this, you get:
	//
	// require github.com/prometheus/prometheus: version "v2.16.0" invalid: module contains a go.mod file, so major version must be compatible: should be v0 or v1, not v2
	//
	// If you add the +incompatible bit that the error message hints
	// at, you get a different error (see below).
	github.com/prometheus/prometheus v1.8.2-0.20200727090838-6f296594a852
	github.com/rs/zerolog v1.28.0
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stretchr/testify v1.8.0
	github.com/tonobo/mtr v0.1.1-0.20210422192847-1c17592ae70b
	golang.org/x/net v0.0.0-20220425223048-2871e0cb64e4
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/grpc v1.50.0
)

require (
	github.com/go-kit/log v0.2.1
	github.com/go-ping/ping v0.0.0-20211130115550-779d1e919534
	github.com/gogo/status v1.1.1
	github.com/jpillora/backoff v1.0.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.21
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.65
)

require (
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v0.0.0-20181115115552-c206103e1f37 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/oauth2 v0.0.0-20220223155221-ee480838109b // indirect
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/genproto v0.0.0-20210917145530-b395a37504d4 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.65 // indirect
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

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab

replace github.com/tonobo/mtr => github.com/grafana/mtr v0.1.1-0.20211103212629-0a455647759f

replace github.com/prometheus/blackbox_exporter v0.21.0 => github.com/grafana/blackbox_exporter v0.21.1-0.20220614164936-0cf374fec170
