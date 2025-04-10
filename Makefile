## This is a self-documented Makefile. For usage information, run `make help`:
##
## For more information, refer to https://www.thapaliya.com/en/writings/well-documented-makefiles/

ROOTDIR       := $(abspath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
DISTDIR       := $(abspath $(ROOTDIR)/dist)
.DEFAULT_GOAL := all

.PHONY: all
all: deps build

include $(ROOTDIR)/.gbt.mk

include $(ROOTDIR)/config.mk

include $(wildcard $(ROOTDIR)/scripts/make/[0-9][0-9][0-9]_*.mk)
