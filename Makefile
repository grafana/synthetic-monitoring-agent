## This is a self-documented Makefile. For usage information, run `make help`:
##
## For more information, refer to https://suva.sh/posts/well-documented-makefiles/

-include local/Makefile

S := @
V :=

GO = GO111MODULE=on go
GO_PKGS ?= ./...
SH_FILES ?= $(shell find ./scripts -name *.sh)

COMMANDS := $(shell $(GO) list -mod=vendor ./cmd/...)

.DEFAULT_GOAL := all

.PHONY: all
all: deps build

##@ Dependencies

.PHONY: deps-go
deps-go: ## Install Go dependencies.
	$(S) true
	$(GO) mod download

.PHONY: deps
deps: deps-go ## Install all dependencies.

##@ Building

BUILD_GO_TARGETS := $(addprefix build-go-, $(COMMANDS))

.PHONY: $(BUILD_GO_TARGETS)
$(BUILD_GO_TARGETS): build-go-%:
	$(S) echo 'Building $*'
	$(GO) build -mod=vendor $*

.PHONY: build-go
build-go: $(BUILD_GO_TARGETS) ## Build all Go binaries.
	$(S) true

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
	$(GO) test -mod=vendor -v ./...

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
	    --modules-download-mode=vendor \
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
	$(S) $(GO) vet -mod vendor $(GO_PKGS)

.PHONY: lint-go
lint-go: go-vet golangci-lint gosec ## Run all Go code checks.

.PHONY: lint
lint: lint-go ## Run all code checks.

##@ Helpers

.PHONY: clean
clean: ## Clean up intermediate build artifacts.
	$(S) echo "cleaning"
	rm -rf node_modules
	rm -rf public/build

.PHONY: help
help: ## Display this help.
	$(S) awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: docker
docker: build
	$(S) docker build -t us.gcr.io/kubernetes-dev/worldping-blackbox-sidecar:latest .