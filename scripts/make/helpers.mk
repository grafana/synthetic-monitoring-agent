##@ Helpers

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

.PHONY: version
version: ## Create version information file.
version: $(DISTDIR)/version.new
	# Look at the new version file and replace it only if it has changed.
	$(S) cmp -s $(DISTDIR)/version.new $(DISTDIR)/version && \
		rm $(DISTDIR)/version.new || \
		mv $(DISTDIR)/version.new $(DISTDIR)/version
	$(S) cat $(DISTDIR)/version

$(DISTDIR)/version $(DISTDIR)/version.new:
	$(S) mkdir -p $(DISTDIR)
	$(S) ./scripts/version > $@

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

ifeq ($(CI),true)
TESTDATA_GO ?= $(GO)
else
TESTDATA_GO ?= $(ROOTDIR)/scripts/docker-run env $(GO)
endif

.PHONY: testdata
testdata: ## Update golden files for tests.
	$(V) $(TESTDATA_GO) test -v -run TestValidateMetrics ./internal/scraper -args -update-golden

# rwildcard will recursively search for files matching the pattern, e.g. $(call rwildcard, src/*.c)
rwildcard = $(call rwildcard_helper, $(dir $1), $(notdir $1))
rwildcard_helper = $(wildcard $(addsuffix $(strip $2), $(strip $1))) \
	    $(foreach d, $(wildcard $(addsuffix *, $(strip $1))), $(call rwildcard_helper, $d/, $2))

.PHONY: update-go-version
update-go-version: ## Update Go version (specify in go.mod)
	$(S) echo 'Updating Go version to $(GO_VERSION)...'
	$(S) cd scripts/go && $(GO) mod edit -go=$(GO_VERSION)
	$(S) sed -i -e 's,^GO_VERSION=.*,GO_VERSION=$(GO_VERSION),' scripts/docker_build

.PHONY: help
help: ## Display this help.
	$(S) awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

