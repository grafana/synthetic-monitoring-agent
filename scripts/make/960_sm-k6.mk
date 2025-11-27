XK6_PLATFORMS := $(filter-out linux/arm,$(PLATFORMS)) darwin/arm64 darwin/amd64

.PHONY: sm-k6
sm-k6:
	@true

define sm-k6
$(DISTDIR)/$(1)-$(2)/sm-k6:
	mkdir -p "$(DISTDIR)/$(1)-$(2)"
	# Renovate updates the following line. Keep its syntax as it is.
	curl -sSL https://github.com/grafana/xk6-sm/releases/download/v0.6.12/sm-k6-$(1)-$(2) -o "$$@"
	chmod +x "$$@"

sm-k6: $(DISTDIR)/$(1)-$(2)/sm-k6
endef

$(foreach BUILD_PLATFORM,$(XK6_PLATFORMS), \
	$(eval $(call sm-k6,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))))))

.PHONY: sm-k6-native
sm-k6-native: $(DISTDIR)/$(HOST_OS)-$(HOST_ARCH)/sm-k6
