#!/usr/bin/env bash
# shellcheck disable=SC3044
#
# Generate all protobuf bindings.
# Run from repository root.
set -e
set -u

if test ! -e "scripts/genproto.sh" ; then
	echo "must be run from repository root"
	exit 255
fi

if ! command -v protoc > /dev/null 2>&1 ; then
	echo "could not find protoc 3.5.1, is it installed + in PATH?"
	exit 255
fi

echo "Installing Protocol Buffers plugins"
GO111MODULE=on go mod download

### INSTALL_PKGS="github.com/gogo/protobuf/protoc-gen-gogofast github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger"
INSTALL_PKGS="github.com/gogo/protobuf/protoc-gen-gogofast"
for pkg in ${INSTALL_PKGS}; do
    GO111MODULE=on go install "$pkg"
done

PB_ROOT="$(GO111MODULE=on go list -f '{{.Dir}}' ./pkg/pb)"
PB_PATH="${PB_ROOT}"
GOGO_PROTOBUF_ROOT="$(GO111MODULE=on go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)"
GOGO_PROTOBUF_PATH="${GOGO_PROTOBUF_ROOT}:${GOGO_PROTOBUF_ROOT}/protobuf"
GOGO_GOOGLEAPIS_ROOT="$(GO111MODULE=on go list -f '{{ .Dir }}' -m github.com/gogo/googleapis)"
GOGO_GOOGLEAPIS_PATH="${GOGO_GOOGLEAPIS_ROOT}"

DIRS="pkg/pb/synthetic_monitoring"

for dir in ${DIRS}; do
	echo "Generating Protocol Buffers code in ${dir}"
	pushd "${dir}" > /dev/null
		protoc --gogofast_out=plugins=grpc:. \
			-I=. \
			-I="${PB_PATH}" \
			-I="${GOGO_PROTOBUF_PATH}" \
			-I="${GOGO_GOOGLEAPIS_PATH}" \
			./*.proto

		sed -i.bak -E 's,import _ "github.com/gogo/protobuf/gogoproto",,g' -- *.pb.go
		sed -i.bak -E 's,(import |\t)_ "google/protobuf",,g' -- *.pb.go
		sed -i.bak -E 's,(import |\t)_ "google/api",,g' -- *.pb.go
		sed -i.bak -E 's,golang/protobuf,gogo/protobuf,g' -- *.pb.go
		sed -i.bak -E 's,(import |\t)protobuf "google/protobuf",protobuf "github.com/gogo/protobuf/types", g' -- *.pb.go
		rm -f -- *.bak
		goimports -w ./*.pb.go
	popd > /dev/null
done
