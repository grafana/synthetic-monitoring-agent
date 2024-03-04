# This file is included from the main makefile. Anything that is
# specific to this module should go in this file.

DOCKER_TAG = grafana/synthetic-monitoring-agent

PLATFORMS := $(sort $(HOST_OS)/$(HOST_ARCH) linux/amd64 linux/arm64)
