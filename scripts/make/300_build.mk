##@ Building

define build_go_template
BUILD_GO_TARGETS += build-go-$(1)-$(2)-$(3)

build-go-$(1)-$(2)-$(3) : GOOS := $(1)
build-go-$(1)-$(2)-$(3) : GOARCH := $(2)
build-go-$(1)-$(2)-$(3) : GOPKG := $(3)
build-go-$(1)-$(2)-$(3) : DIST_FILENAME := $(firstword $(OUTPUT_FILE) $(DISTDIR)/$(1)-$(2)/$(notdir $(3)))

endef

$(foreach BUILD_PLATFORM,$(PLATFORMS), \
	$(foreach CMD,$(COMMANDS), \
		$(eval $(call build_go_template,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))),$(CMD)))))

BUILD_GO_NATIVE_TARGETS := $(filter build-go-$(HOST_OS)-$(HOST_ARCH)-%, $(BUILD_GO_TARGETS))

.PHONY: $(BUILD_GO_TARGETS)
$(BUILD_GO_TARGETS) : build-go-% :
	$(call build_go_command,$(GOPKG))

.PHONY: build-go
build-go: $(BUILD_GO_TARGETS) ## Build all Go binaries.
	$(S) echo Done.

.PHONY: build
build: build-go ## Build everything.

.PHONY: build-native
build-native: $(BUILD_GO_NATIVE_TARGETS) ## Build only native Go binaries
	$(S) echo Done.

VERSION_PKG := $(shell $(GO) list $(GO_BUILD_MOD_FLAGS) ./internal/version)

define build_go_command
	$(S) echo 'Building $(1) ($(GOOS)-$(GOARCH))'
	$(S) mkdir -p $(DISTDIR)/$(GOOS)-$(GOARCH)
	$(V) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build \
		$(GO_BUILD_FLAGS) \
		-o '$(DIST_FILENAME)' \
		-ldflags '-X "$(VERSION_PKG).commit=$(BUILD_COMMIT)" -X "$(VERSION_PKG).version=$(BUILD_VERSION)" -X "$(VERSION_PKG).buildstamp=$(BUILD_STAMP)"' \
		'$(1)'
	$(V) test '$(GOOS)' = '$(HOST_OS)' -a '$(GOARCH)' = '$(HOST_ARCH)' && \
		cp -a '$(DIST_FILENAME)' '$(DISTDIR)/$(notdir $(1))' || \
		true
endef
