##@ Helpers

DRONE_SERVER ?= https://drone.grafana.net

export DRONE_SERVER
export DRONE_TOKEN

ifeq ($(origin GITIGNORE_GEN),undefined)
GITIGNORE_GEN ?= $(ROOTDIR)/scripts/go/bin/gitignore-gen
LOCAL_GITIGNORE_GEN = yes
endif

ifeq ($(origin BRA_BIN),undefined)
BRA_BIN ?= $(ROOTDIR)/scripts/go/bin/bra
LOCAL_BRA = yes
endif

ifeq ($(LOCAL_BRA),yes)
$(BRA_BIN): scripts/go/go.mod
	$(S) cd scripts/go && \
		$(GO) mod download && \
		$(GO) build -o "$@" github.com/unknwon/bra
endif

.PHONY: run
run: $(BRA_Bin) ## Build and run web server on filesystem changes.
	$(S) $(GO_BUILD_MOD_FLAGS) $(BRA_BIN) run

.PHONY: clean
clean: ## Clean up intermediate build artifacts.
	$(S) echo "Cleaning intermediate build artifacts..."
	$(V) rm -rf node_modules
	$(V) rm -rf public/build
	$(V) rm -rf dist/build
	$(V) rm -rf dist/publish

.PHONY: distclean
distclean: clean ## Clean up all build artifacts.
	$(S) echo "Cleaning all build artifacts..."
	$(V) git clean -Xf

.PHONY: update-tools
update-tools: ## Update tools
	$(S) echo "Updating tools..."
	$(V) cd scripts/go && ./update
	$(S) echo "Done."

ifeq ($(LOCAL_GITIGNORE_GEN),yes)
$(GITIGNORE_GEN): scripts/go/go.mod
	$(S) cd scripts/go && \
		$(GO) mod download && \
		$(GO) build -o "$@" github.com/mem/gitignore-gen
endif

.PHONY: update-gitignore
update-gitignore: $(GITIGNORE_GEN)
	$(V) $(GITIGNORE_GEN) -config-filename scripts/configs/gitignore.yaml > $(ROOTDIR)/.gitignore

.PHONY: docker-build
docker-build:
	$(ROOTDIR)/scripts/docker_build build

.PHONY: docker-image
docker-image: docker-build
	$(S) DOCKER_BUILDKIT=1 docker build --file Dockerfile --tag $(DOCKER_TAG):$(BUILD_VERSION) .

.PHONY: docker-push
docker-push:  docker
	$(S) docker push $(DOCKER_TAG)
	$(S) docker tag $(DOCKER_TAG) $(DOCKER_TAG):$(BUILD_VERSION)
	$(S) docker push $(DOCKER_TAG):$(BUILD_VERSION)

.PHONY: generate
generate: $(ROOTDIR)/pkg/accounting/data.go
generate: $(ROOTDIR)/pkg/pb/synthetic_monitoring/checks.pb.go
generate: $(ROOTDIR)/pkg/pb/synthetic_monitoring/string.go
generate: $(ROOTDIR)/pkg/pb/synthetic_monitoring/multihttp_string.go
generate:
	$(S) true

$(ROOTDIR)/pkg/accounting/data.go: $(ROOTDIR)/pkg/accounting/data.go.tmpl $(wildcard $(ROOTDIR)/internal/scraper/testdata/*.txt)
	$(S) echo "Generating $@ ..."
	$(V) $(GO) generate -v "$(@D)"

$(ROOTDIR)/pkg/pb/synthetic_monitoring/%.pb.go: $(ROOTDIR)/pkg/pb/synthetic_monitoring/%.proto
	$(S) echo "Generating $@ ..."
	$(V) $(ROOTDIR)/scripts/genproto.sh

$(ROOTDIR)/pkg/pb/synthetic_monitoring/string.go: $(wildcard $(ROOTDIR)/pkg/pb/synthetic_monitoring/*.pb.go) $(ROOTDIR)/pkg/pb/synthetic_monitoring/checks_extra.go
	$(S) echo "Generating $@ ..."
	$(V) $(GO) generate -v "$(@D)"

$(ROOTDIR)/pkg/pb/synthetic_monitoring/multihttp_string.go: $(wildcard $(ROOTDIR)/pkg/pb/synthetic_monitoring/*.pb.go) $(ROOTDIR)/pkg/pb/synthetic_monitoring/checks_extra.go
	$(S) echo "Generating $@ ..."
	$(V) $(GO) generate -v "$(@D)"

.PHONY: testdata
testdata: ## Update golden files for tests.
	$(V) $(GO) test -v -run TestValidateMetrics ./internal/scraper -args -update-golden

.PHONY: drone
drone: .drone.yml ## Update drone files
	$(S) true

# rwildcard will recursively search for files matching the pattern, e.g. $(call rwildcard, src/*.c)
rwildcard = $(call rwildcard_helper, $(dir $1), $(notdir $1))
rwildcard_helper = $(wildcard $(addsuffix $(strip $2), $(strip $1))) \
	    $(foreach d, $(wildcard $(addsuffix *, $(strip $1))), $(call rwildcard_helper, $d/, $2))

DRONE_SOURCE_FILES := $(call rwildcard, $(ROOTDIR)/scripts/configs/drone/*.jsonnet) $(call rwildcard, $(ROOTDIR)/scripts/configs/drone/*.libsonnet)

.drone.yml: $(DRONE_SOURCE_FILES)
	$(S) echo 'Regenerating $@...'
ifneq ($(origin DRONE_TOKEN),environment)
ifeq ($(origin DRONE_TOKEN),undefined)
	$(S) echo 'E: DRONE_TOKEN should set in the environment. Stop.'
else
	$(S) echo 'E: DRONE_TOKEN should *NOT* be set in a makefile, set it in the environment. Stop.'
endif
	$(S) false
endif
	$(V) ./scripts/generate-drone-yaml "$(GBT_IMAGE)" "$(GH_REPO_NAME)" "$(ROOTDIR)/.drone.yml" "$(ROOTDIR)/scripts/configs/drone/main.jsonnet"

.PHONY: dronefmt
dronefmt: ## Format drone jsonnet files
	$(S) $(foreach src, $(filter-out $(ROOTDIR)/scripts/configs/drone/vendor/%, $(DRONE_SOURCE_FILES)), \
		echo "==== Formatting $(src)" ; \
		jsonnetfmt -i --pretty-field-names --pad-arrays --pad-objects --no-use-implicit-plus "$(src)" ; \
	)

.PHONY: update-go-version
update-go-version: ## Update Go version (specify in go.mod)
	$(S) echo 'Updating Go version to $(GO_VERSION)...'
	$(S) cd scripts/go && $(GO) mod edit -go=$(GO_VERSION)
	$(S) sed -i -e 's,^GO_VERSION=.*,GO_VERSION=$(GO_VERSION),' scripts/docker_build
	$(S) $(MAKE) --always-make --no-print-directory .drone.yml S=$(S) V=$(V)

.PHONY: help
help: ## Display this help.
	$(S) awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

