XK6_PLATFORMS := $(filter-out linux/arm,$(PLATFORMS)) darwin/arm64 darwin/amd64

.PHONY: sm-k6
sm-k6:
	@true

define sm-k6-v1-binary
$(DISTDIR)/$(1)-$(2)/k6-v1:
	mkdir -p "$(DISTDIR)/$(1)-$(2)"
	# Renovate updates the following line. Keep its syntax as it is.
	curl -sSL https://github.com/grafana/xk6-sm/releases/download/v0.6.20/sm-k6-$(1)-$(2) -o "$$@" # k6-v1
	chmod +x "$$@"

sm-k6: $(DISTDIR)/$(1)-$(2)/k6-v1
endef

define sm-k6-v2-binary
$(DISTDIR)/$(1)-$(2)/k6-v2:
	mkdir -p "$(DISTDIR)/$(1)-$(2)"
	# Renovate updates the following line. Keep its syntax as it is.
	curl -sSL https://github.com/grafana/xk6-sm/releases/download/v2.0.0/sm-k6-$(1)-$(2) -o "$$@" # k6-v2
	chmod +x "$$@"

sm-k6: $(DISTDIR)/$(1)-$(2)/k6-v2
endef

$(foreach BUILD_PLATFORM,$(XK6_PLATFORMS), \
	$(eval $(call sm-k6-v1-binary,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))))))

$(foreach BUILD_PLATFORM,$(XK6_PLATFORMS), \
	$(eval $(call sm-k6-v2-binary,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))))))

.PHONY: sm-k6-native
sm-k6-native: $(DISTDIR)/$(HOST_OS)-$(HOST_ARCH)/k6-v1 $(DISTDIR)/$(HOST_OS)-$(HOST_ARCH)/k6-v2
