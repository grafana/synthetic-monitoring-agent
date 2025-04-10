.PHONY: generate-policy-bot-config
generate-policy-bot-config: ## Generate policy bot config.
	$(S) echo 'Generating policy bot configuration...'
	$(V) $(ROOTDIR)/scripts/gen-policy-bot-config "$(ROOTDIR)"
	$(S) echo 'Done.'
