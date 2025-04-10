##@ Testing

GO_TEST_ARGS ?= $(GO_PKGS)
GO_TEST_FORMAT ?= standard-verbose

TEST_OUTPUT := $(DISTDIR)/test

ifeq ($(CI),true)
GOTESTSUM ?= gotestsum
endif

ifeq ($(origin GOTESTSUM),undefined)
GOTESTSUM ?= ./scripts/docker-run gotestsum
endif

.PHONY: test-go
test-go: export CGO_ENABLED=1 # Required so that -race works.
test-go: ## Run Go tests.
	$(S) echo "test backend"
	$(S) mkdir -p '$(DISTDIR)'
	$(GOTESTSUM) \
		--format $(GO_TEST_FORMAT) \
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
