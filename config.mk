# This file is included from the main makefile. Anything that is
# specific to this module should go in this file.

DOCKER_TAG = grafana/synthetic-monitoring-agent

GO_TOOLS_IMAGE := us.gcr.io/kubernetes-dev/go-tools:2023-10-10-v384623-f09f9eb30

PLATFORMS := $(sort $(HOST_OS)/$(HOST_ARCH) linux/amd64 linux/arm64 linux/arm)
