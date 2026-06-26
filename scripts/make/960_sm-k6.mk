XK6_PLATFORMS := $(filter-out linux/arm,$(PLATFORMS)) darwin/arm64 darwin/amd64

K6_V1_VERSION=v1.1.7
K6_V2_VERSION=v2.0.1

.PHONY: sm-k6
sm-k6:
	@true

.PHONY: sm-k6-native
sm-k6-native:
	@true

# Args:
# 1: OS
# 2: Arch
# 3: Tag
# 4: Version
define sm-k6-binary
$(DISTDIR)/$(1)-$(2)/k6-$(3):
	mkdir -p "$(DISTDIR)/$(1)-$(2)"
	curl -fsSL https://github.com/grafana/xk6-sm/releases/download/$(4)/sm-k6-$(1)-$(2) -o "$$@"
	chmod +x "$$@"

sm-k6: $(DISTDIR)/$(1)-$(2)/k6-$(3)

ifeq ($(HOST_OS)-$(HOST_ARCH),$(1)-$(2))
sm-k6-native: $(DISTDIR)/$(1)-$(2)/k6-$(3)
endif
endef

$(foreach BUILD_PLATFORM,$(XK6_PLATFORMS), \
	$(eval $(call sm-k6-binary,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))),v1,$(K6_V1_VERSION))))

$(foreach BUILD_PLATFORM,$(XK6_PLATFORMS), \
	$(eval $(call sm-k6-binary,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))),v2,$(K6_V2_VERSION))))
