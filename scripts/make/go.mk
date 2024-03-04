GO_VERSION            := $(shell go list -m -f '{{.GoVersion}}')
GO                    := GO111MODULE=on CGO_ENABLED=0 go
GO_VENDOR             := $(if $(realpath $(ROOTDIR)/vendor/modules.txt),true,false)
GO_BUILD_COMMON_FLAGS := -trimpath
GO_MODULE_NAME        := $(shell $(GO) list -m)

ifeq ($(GO_VENDOR),true)
	GO_BUILD_MOD_FLAGS := -mod=vendor
	GOLANGCI_LINT_MOD_FLAGS := --modules-download-mode=vendor
else
	GO_BUILD_MOD_FLAGS := -mod=readonly
	GOLANGCI_LINT_MOD_FLAGS := --modules-download-mode=readonly
endif

GO_BUILD_FLAGS := $(GO_BUILD_MOD_FLAGS) $(GO_BUILD_COMMON_FLAGS)

GO_PKGS ?= ./...

COMMANDS := $(shell $(GO) list $(GO_BUILD_MOD_FLAGS) -f '{{if (eq .Name "main")}}{{.ImportPath}}{{end}}' ./cmd/...)

# This probably shouldn't be here, but it has to come after getting the Go module name
ifeq ($(origin GH_REPO_NAME),undefined)
	GH_REPO_NAME := $(GO_MODULE_NAME:github.com/%=%)
endif

ifeq ($(origin ENUMER),undefined)
ifneq ($(LOCAL_ENUMER),yes)
ENUMER ?= docker run \
		 --rm \
		 -v '$(ROOTDIR):/mnt' \
		 -v '$(HOME)/.cache/go-build:/root/.cache/go-build' \
		 --env GOFLAGS=-buildvcs=false \
		 --workdir /mnt \
		 '$(GBT_IMAGE)' \
		 enumer
endif
endif

ifeq ($(LOCAL_ENUMER),yes)
ENUMER ?= $(ROOTDIR)/scripts/go/bin/enumer
$(ENUMER): scripts/go/go.mod scripts/go/go.sum
	$(S) cd scripts/go && \
		$(GO) mod download && \
		$(GO) build -o $(ENUMER) github.com/dmarkham/enumer
endif
