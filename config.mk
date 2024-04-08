# This file is included from the main makefile. Anything that is
# specific to this module should go in this file.

DOCKER_TAG = grafana/synthetic-monitoring-agent

GO_TOOLS_IMAGE := ghcr.io/grafana/grafana-build-tools:v0.10.0

PLATFORMS := $(sort $(HOST_OS)/$(HOST_ARCH) linux/amd64 linux/arm64)
