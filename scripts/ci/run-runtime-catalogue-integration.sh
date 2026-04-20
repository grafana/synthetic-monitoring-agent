#!/bin/sh
set -eu

# Run the k6/browser runtime catalogue integration tests inside the browser
# image by installing a minimal Go toolchain at runtime.

: "${K6_PATH:=/usr/libexec/sm-k6/k6-v1}"
: "${SM_RUNTIME_CATALOGUE_INTEGRATION:=1}"
: "${GOCACHE:=/tmp/go-cache}"
: "${GOMODCACHE:=/tmp/go-mod-cache}"
: "${HOME:=/tmp}"

export K6_PATH
export SM_RUNTIME_CATALOGUE_INTEGRATION
export GOCACHE
export GOMODCACHE
export HOME

apk add --no-cache go git build-base >/dev/null

exec go test -run TestValidateMetricCatalogueIntegration -count=1 -v ./internal/scraper
