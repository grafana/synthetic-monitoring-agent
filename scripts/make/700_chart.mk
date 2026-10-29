##@ Helm chart

HELM_DOCS_IMAGE ?= jnorwood/helm-docs:v1.14.2@sha256:7e562b49ab6b1dbc50c3da8f2dd6ffa8a5c6bba327b1c6335cc15ce29267979c

.PHONY: chart
chart: chart-lint chart-test chart-check-flags chart-check-docs ## Run all Helm chart checks.

.PHONY: chart-lint
chart-lint: ## Lint the Helm chart and validate the rendered manifests.
	$(S) echo 'Linting the Helm chart...'
	$(V) $(ROOTDIR)/scripts/lint-chart

.PHONY: chart-test
chart-test: ## Run the Helm chart unit tests.
	$(S) echo 'Running the Helm chart unit tests...'
	$(V) helm unittest $(ROOTDIR)/charts/synthetic-monitoring-agent

.PHONY: chart-check-flags
chart-check-flags: ## Check the agent supports the chart's flags. AGENT_IMAGE overrides the appVersion image.
	$(S) echo 'Checking the agent supports the chart flags...'
	$(V) $(ROOTDIR)/scripts/check-chart-flags $(AGENT_IMAGE)

.PHONY: chart-docs
chart-docs: ## Regenerate the Helm chart README from README.md.gotmpl and values.yaml.
	$(S) echo 'Generating the Helm chart README...'
	$(V) docker run --rm --user $$(id -u):$$(id -g) -v $(ROOTDIR)/charts:/helm-docs $(HELM_DOCS_IMAGE)

.PHONY: chart-check-docs
chart-check-docs: ## Check the Helm chart README is up to date.
	$(S) echo 'Checking the Helm chart README is up to date...'
	$(V) docker run --rm --user $$(id -u):$$(id -g) -v $(ROOTDIR)/charts:/helm-docs $(HELM_DOCS_IMAGE) --dry-run 2> /dev/null | diff -u $(ROOTDIR)/charts/synthetic-monitoring-agent/README.md - || { echo 'README.md is stale: run make chart-docs'; exit 1; }
