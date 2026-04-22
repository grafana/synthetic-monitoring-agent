.PHONY: generate-renovate-config
generate-renovate-config: ## Generate renovate config.
	$(S) echo 'Generating renovate configuration...'
	$(V) $(ROOTDIR)/scripts/generate-renovate-config
	$(S) echo 'Done.'
