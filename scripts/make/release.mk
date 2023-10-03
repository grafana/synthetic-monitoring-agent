##@ Packaging
GOPATH := $(shell go env GOPATH)
GORELEASER := $(GOPATH)/bin/goreleaser
# TODO: Upgrade goreleaser when Go version is upgraded past 1.17. Newer versions require Go 1.18 or 1.19
$(GORELEASER):
	go install github.com/goreleaser/goreleaser@v1.6.3 

.PHONY: release
release: $(GORELEASER) ## Build a release and publish it to Github.
	$(S) echo "Building and publishing release files..."		
	$(GORELEASER) release --rm-dist

.PHONY: release-snapshot
release-snapshot: $(GORELEASER) ## Build a snapshot release for testing (not published).
	$(S) echo "Building release files..."		
	$(GORELEASER) release --snapshot --rm-dist
