DISTDIR       := $(abspath $(ROOTDIR)/dist)
HOST_OS       := $(shell go env GOOS)
HOST_ARCH     := $(shell go env GOARCH)
PLATFORMS     ?= $(sort $(HOST_OS)/$(HOST_ARCH) linux/amd64 linux/arm linux/arm64)

BUILD_VERSION := $(shell $(ROOTDIR)/scripts/version)
BUILD_COMMIT  := $(shell git rev-parse HEAD^{commit})
BUILD_STAMP   := $(shell date -u '+%Y-%m-%d %H:%M:%S+00:00')

S := @
V :=

ifneq ($(strip $(S)),)
.SILENT:
endif

GO_TOOLS_IMAGE := $(GBT_IMAGE)

HAS_PROTO := $(shell $(ROOTDIR)/scripts/list-proto -e && echo true)
