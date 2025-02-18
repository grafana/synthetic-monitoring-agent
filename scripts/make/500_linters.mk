##@ Linting

SH_FILES   ?= $(shell $(ROOTDIR)/scripts/list-sh-scripts)
BASH_FILES ?= $(shell $(ROOTDIR)/scripts/list-sh-scripts -sbash)

ifeq ($(CI),true)
GOLANGCI_LINT ?= golangci-lint
SHELLCHECK ?= shellcheck
endif

ifeq ($(origin GOLANGCI_LINT),undefined)
GOLANGCI_LINT ?= ./scripts/docker-run golangci-lint
endif

ifeq ($(origin SHELLCHECK),undefined)
SHELLCHECK ?= ./scripts/docker-run shellcheck
endif

.PHONY: lint
lint: ## Run all code checks.
	$(S) true

.PHONY: lint-go
lint: lint-go
lint-go: go-vet golangci-lint ## Run all Go code checks.

ifneq ($(strip $(V)),)
GOLANGCI_LINT_EXTRA_FLAGS := --verbose
endif

.PHONY: golangci-lint
golangci-lint:
	$(S) echo "lint via golangci-lint"
	$(S) $(GOLANGCI_LINT) run \
		$(GOLANGCI_LINT_MOD_FLAGS) \
		--config '$(ROOTDIR)/.golangci.yaml' \
		$(GOLANGCI_LINT_EXTRA_FLAGS) \
		$(GO_PKGS)

.PHONY: go-vet
go-vet:
	$(S) echo "lint via go vet"
	$(S) $(GO) vet $(GO_BUILD_MOD_FLAGS) $(GO_PKGS)

.PHONY: lint-sh
lint: lint-sh
lint-sh: ## Run all shell code checks.
	$(S) echo "lint via shellcheck"
	$(S) if test -n "$(SH_FILES)" ; then $(SHELLCHECK) -a --color=auto --shell=sh $(SH_FILES) ; fi
	$(S) if test -n "$(BASH_FILES)" ; then $(SHELLCHECK) -a --color=auto --shell=bash $(BASH_FILES) ; fi
