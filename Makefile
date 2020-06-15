## This is a self-documented Makefile. For usage information, run `make help`:
##
## For more information, refer to https://suva.sh/posts/well-documented-makefiles/

ROOTDIR := $(abspath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
DISTDIR := $(abspath $(ROOTDIR)/dist)

BUILD_VERSION := $(shell git describe --dirty --tags --always)
BUILD_COMMIT := $(shell git rev-parse HEAD^{commit})
BUILD_STAMP := $(shell date --utc --rfc-3339=seconds)

include config.mk

-include local/Makefile

S := @
V :=

GO := GO111MODULE=on go
GO_VENDOR := $(if $(realpath $(ROOTDIR)/vendor/modules.txt),true,false)
GO_BUILD_COMMON_FLAGS := -trimpath
ifeq ($(GO_VENDOR),true)
	GO_BUILD_MOD_FLAGS := -mod=vendor
	GOLANGCI_LINT_MOD_FLAGS := --modules-download-mode=vendor
else
	GO_BUILD_MOD_FLAGS := -mod=readonly
	GOLANGCI_LINT_MOD_FLAGS := --modules-download-mode=readonly
endif
GO_BUILD_FLAGS := $(GO_BUILD_MOD_FLAGS) $(GO_BUILD_COMMON_FLAGS)

GO_PKGS ?= ./...
SH_FILES ?= $(shell find ./scripts -name *.sh)

COMMANDS := $(shell $(GO) list $(GO_BUILD_MOD_FLAGS) ./cmd/...)

.DEFAULT_GOAL := all

.PHONY: all
all: deps build

##@ Dependencies

.PHONY: deps-go
deps-go: ## Install Go dependencies.
ifeq ($(GO_VENDOR),true)
	$(GO) mod vendor
else
	$(GO) mod download
endif
	$(GO) mod verify
	$(GO) mod tidy

.PHONY: deps
deps: deps-go ## Install all dependencies.

##@ Building

BUILD_GO_TARGETS := $(addprefix build-go-, $(COMMANDS))

.PHONY: $(BUILD_GO_TARGETS)
$(BUILD_GO_TARGETS): build-go-%:
	$(call build_go_command,$*)

.PHONY: build-go
build-go: $(BUILD_GO_TARGETS) ## Build all Go binaries.
	$(S) echo Done.

.PHONY: build
build: build-go ## Build everything.

scripts/go/bin/bra: scripts/go/go.mod
	$(S) cd scripts/go; \
		$(GO) build -o ./bin/bra github.com/unknwon/bra

.PHONY: run
run: scripts/go/bin/bra ## Build and run web server on filesystem changes.
	$(S) GO111MODULE=on scripts/go/bin/bra run

##@ Testing

.PHONY: test-go
test-go: ## Run Go tests.
	$(S) echo "test backend"
	$(GO) test $(GO_BUILD_MOD_FLAGS) -v ./...

.PHONY: test
test: test-go ## Run all tests.

##@ Linting

scripts/go/bin/golangci-lint: scripts/go/go.mod
	$(S) cd scripts/go; \
		$(GO) build -o ./bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint

.PHONY: golangci-lint
golangci-lint: scripts/go/bin/golangci-lint
	$(S) echo "lint via golangci-lint"
	$(S) scripts/go/bin/golangci-lint run \
		$(GOLANGCI_LINT_MOD_FLAGS) \
		--config ./scripts/go/configs/golangci.yml \
		$(GO_PKGS)

scripts/go/bin/gosec: scripts/go/go.mod
	$(S) cd scripts/go; \
		$(GO) build -o ./bin/gosec github.com/securego/gosec/cmd/gosec

# TODO recheck the rules and leave only necessary exclusions
.PHONY: gosec
gosec: scripts/go/bin/gosec
	$(S) echo "lint via gosec"
	$(S) scripts/go/bin/gosec -quiet \
		-exclude= \
		-conf=./scripts/go/configs/gosec.json \
		$(GO_PKGS)

.PHONY: go-vet
go-vet:
	$(S) echo "lint via go vet"
	$(S) $(GO) vet $(GO_BUILD_MOD_FLAGS) $(GO_PKGS)

.PHONY: lint-go
lint-go: go-vet golangci-lint gosec ## Run all Go code checks.

.PHONY: lint
lint: lint-go ## Run all code checks.

##@ Packaging
.PHONY: package
package: build ## Build Debian and RPM packages.
	$(S) echo "Building Debian and RPM packages..."		
	$(S) sh scripts/package/package.sh

.PHONY: publish-packages
publish-packages: package ## Publish Debian and RPM packages to the worldping repository.
	$(S) echo "Publishing Debian and RPM packages...."
	$(S) sh scripts/package/publish.sh
	
##@ Helpers

.PHONY: clean
clean: ## Clean up intermediate build artifacts.
	$(S) echo "Cleaning intermediate build artifacts..."
	$(V) rm -rf node_modules
	$(V) rm -rf public/build
	$(V) rm -rf dist/publish

.PHONY: distclean
distclean: clean ## Clean up all build artifacts.
	$(S) echo "Cleaning all build artifacts..."
	$(V) git clean -Xf

.PHONY: help
help: ## Display this help.
	$(S) awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: docker
docker: build
	$(S) docker build -t $(DOCKER_TAG) ./

define build_go_command
	$(S) echo 'Building $(1)'
	$(S) mkdir -p dist
	$(V) $(GO) build \
		$(GO_BUILD_FLAGS) \
		-o '$(DISTDIR)/$(notdir $(1))' \
		-ldflags '-X "main.commit=$(BUILD_COMMIT)" -X "main.version=$(BUILD_VERSION)" -X "main.buildstamp=$(BUILD_STAMP)"' \
		'$(1)'
endef
