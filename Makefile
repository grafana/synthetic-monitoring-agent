## This is a self-documented Makefile. For usage information, run `make help`:
##
## For more information, refer to https://www.thapaliya.com/en/writings/well-documented-makefiles/

ROOTDIR       := $(abspath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

.DEFAULT_GOAL := all

.PHONY: all
all: deps build

include $(ROOTDIR)/scripts/make/vars.mk

include $(ROOTDIR)/config.mk

include $(ROOTDIR)/.gbt.mk

-include $(ROOTDIR)/scripts/make/local.mk

include $(ROOTDIR)/scripts/make/go.mk
include $(ROOTDIR)/scripts/make/deps.mk
include $(ROOTDIR)/scripts/make/build.mk
include $(ROOTDIR)/scripts/make/testing.mk
include $(ROOTDIR)/scripts/make/linters.mk
include $(ROOTDIR)/scripts/make/release.mk
include $(ROOTDIR)/scripts/make/helpers.mk
include $(ROOTDIR)/scripts/make/xk6.mk
