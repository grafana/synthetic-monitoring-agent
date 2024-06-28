XK6_PKG_DIR := $(ROOTDIR)/xk6/sm
XK6_OUT_DIR := $(DISTDIR)/$(HOST_OS)-$(HOST_ARCH)
K6_BIN      := $(XK6_OUT_DIR)/k6
K6_VERSION  := v0.52.0

LOCAL_GOPATH ?= $(shell go env GOPATH)

ifeq ($(origin XK6),undefined)
XK6 ?= $(ROOTDIR)/scripts/docker-run xk6
endif

define build_xk6_template
BUILD_XK6_TARGETS += build-xk6-$(1)-$(2)

build-xk6-$(1)-$(2) : GOOS := $(1)
build-xk6-$(1)-$(2) : GOARCH := $(2)
build-xk6-$(1)-$(2) : DIST_FILENAME := $(dir $(firstword $(OUTPUT_FILE) $(DISTDIR)/$(1)-$(2)/))k6
$(DISTDIR)/$(1)-$(2)/k6) : $(wildcard $(ROOTDIR)/xk6/sm/*.go $(ROOTDIR)/xk6/sm/go.mod)

endef

define build_dummy_xk6_template
BUILD_DUMMY_XK6_TARGETS += build-dummy-xk6-$(1)-$(2)

build-dummy-xk6-$(1)-$(2) : GOOS := $(1)
build-dummy-xk6-$(1)-$(2) : GOARCH := $(2)
build-dummy-xk6-$(1)-$(2) : DIST_FILENAME := $(dir $(firstword $(OUTPUT_FILE) $(DISTDIR)/$(1)-$(2)/))k6
$(DISTDIR)/$(1)-$(2)/k6) :

endef

# TODO(mem): xk6 does not build on linux/arm yet
DUMMY_XK6_PLATFORMS := $(filter linux/arm,$(PLATFORMS))

XK6_PLATFORMS := $(filter-out $(DUMMY_XK6_PLATFORMS),$(PLATFORMS))

$(foreach BUILD_PLATFORM,$(XK6_PLATFORMS), \
	$(eval $(call build_xk6_template,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))))))

$(foreach BUILD_PLATFORM,$(DUMMY_XK6_PLATFORMS), \
	$(eval $(call build_dummy_xk6_template,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))))))

BUILD_XK6_NATIVE_TARGETS := $(filter build-xk6-$(HOST_OS)-$(HOST_ARCH), $(BUILD_XK6_TARGETS))

define build_xk6_command
	$(S) echo 'Building $(notdir $(DIST_FILENAME)) ($(GOOS)-$(GOARCH))'
	$(S) mkdir -p $(DISTDIR)/$(GOOS)-$(GOARCH)
	$(V) GOOS=$(GOOS) GOARCH=$(GOARCH) $(XK6) \
		build $(K6_VERSION) \
		--with xk6-sm='$(XK6_PKG_DIR)' \
		--output '$(DIST_FILENAME)'
	$(V) test '$(GOOS)' = '$(HOST_OS)' -a '$(GOARCH)' = '$(HOST_ARCH)' && \
		cp -a '$(DIST_FILENAME)' '$(DISTDIR)/$(notdir $(DIST_FILENAME))' || \
		true
endef

ifneq ($(strip $(BUILD_XK6_TARGETS)),)
.PHONY: $(BUILD_XK6_TARGETS)
$(BUILD_XK6_TARGETS) : build-xk6-% :
	$(call build_xk6_command)

build: $(BUILD_XK6_TARGETS)
endif

define build_dummy_xk6_command
	$(S) echo 'Building $(notdir $(DIST_FILENAME)) ($(GOOS)-$(GOARCH))'
	$(S) mkdir -p $(DISTDIR)/$(GOOS)-$(GOARCH)
	$(V) install -m 0755 $(ROOTDIR)/scripts/dummy-k6.sh '$(DIST_FILENAME)'
	$(V) test '$(GOOS)' = '$(HOST_OS)' -a '$(GOARCH)' = '$(HOST_ARCH)' && \
		cp -a '$(DIST_FILENAME)' '$(DISTDIR)/$(notdir $(DIST_FILENAME))' || \
		true
endef

ifneq ($(strip $(BUILD_DUMMY_XK6_TARGETS)),)
.PHONY: $(BUILD_DUMMY_XK6_TARGETS)
$(BUILD_DUMMY_XK6_TARGETS) : build-dummy-xk6-% :
	$(call build_dummy_xk6_command)

build: $(BUILD_DUMMY_XK6_TARGETS)
endif

.PHONY: native-k6
native-k6: build-xk6-$(HOST_OS)-$(HOST_ARCH)
