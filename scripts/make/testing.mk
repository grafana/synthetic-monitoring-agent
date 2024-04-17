##@ Testing

GO_TEST_ARGS ?= $(GO_PKGS)

TEST_OUTPUT := $(DISTDIR)/test

ifeq ($(origin GOTESTSUM),undefined)
ifneq ($(LOCAL_GOTESTSUM),yes)
GOTESTSUM ?= $(ROOTDIR)/scripts/docker-run gotestsum
endif
endif

ifeq ($(LOCAL_GOTESTSUM),yes)
GOTESTSUM ?= $(ROOTDIR)/scripts/go/bin/gotestsum
$(GOTESTSUM): scripts/go/go.mod
	$(S) cd scripts/go && \
		$(GO) mod download && \
		$(GO) build -o $(GOTESTSUM) gotest.tools/gotestsum
endif

.PHONY: test-go
test-go: ## Run Go tests.
test-go: $(if $(filter $(LOCAL_GOTESTSUM),yes),$(GOTESTSUM))
test-go:
	$(S) echo "test backend"
	env CGO_ENABLED=1 $(GOTESTSUM) \
		--format standard-verbose \
		--jsonfile $(TEST_OUTPUT).json \
		--junitfile $(TEST_OUTPUT).xml \
		-- \
		$(GO_BUILD_MOD_FLAGS) \
		-cover \
		-coverprofile=$(TEST_OUTPUT).cov \
		-race \
		$(GO_TEST_ARGS)
	$(S) $(ROOTDIR)/scripts/report-test-coverage $(TEST_OUTPUT).cov

.PHONY: test
test: test-go ## Run all tests.

.PHONY: test-go-fast
test-go-fast: GO_TEST_ARGS += -short
test-go-fast: test-go ## Run only fast Go tests.
	$(S) true

.PHONY: test-fast
test-fast: test-go-fast ## Run only fast tests.
