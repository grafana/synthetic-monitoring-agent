##@ Helm chart

.PHONY: chart
chart: chart-lint chart-test chart-check-flags ## Run all Helm chart checks.

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
