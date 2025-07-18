##@ Packaging
GOPATH := $(shell go env GOPATH)
GORELEASER := $(GOPATH)/bin/goreleaser
# TODO: Upgrade goreleaser when Go version is upgraded past 1.17. Newer versions require Go 1.18 or 1.19
$(GORELEASER):
	go install github.com/goreleaser/goreleaser@v1.21.2

.PHONY: release
release: $(GORELEASER) ## Build a release and publish it to Github.
	$(S) echo "Building and publishing release files..."
	$(GORELEASER) release --rm-dist $(GORELEASER_FLAGS)

.PHONY: release-snapshot
release-snapshot: $(GORELEASER) ## Build a snapshot release for testing (not published).
	$(S) echo "Building release files..."
	$(GORELEASER) release --snapshot --rm-dist $(GORELEASER_FLAGS)

define package_template
PACKAGE_TARGETS += package-$(1)-$(2)

$(DISTDIR)/$(1)-$(2)/nfpm-input.yaml: $(DISTDIR)/version
	'$(ROOTDIR)/scripts/package/create-parameters-file' '$$@' '$(1)' '$(2)' '$(BUILD_VERSION)' '$(ROOTDIR)' '$(DISTDIR)'

$(DISTDIR)/$(1)-$(2)/nfpm.yaml : $(DISTDIR)/$(1)-$(2)/nfpm-input.yaml
$(DISTDIR)/$(1)-$(2)/nfpm.yaml : $(ROOTDIR)/scripts/package/nfpm.yaml.template
$(DISTDIR)/$(1)-$(2)/nfpm.yaml :
	$(S) mkdir -p $(DISTDIR)/$(1)-$(2)
	$(S) gomplate \
		--context '.=file://$(DISTDIR)/$(1)-$(2)/nfpm-input.yaml' \
		--file '$(ROOTDIR)/scripts/package/nfpm.yaml.template' \
		> '$$@'

package-deb-$(1)-$(2) : $(DISTDIR)/$(1)-$(2)/nfpm.yaml $(DISTDIR)/changelog.yaml
	$(S) nfpm package \
		--config $(DISTDIR)/$(1)-$(2)/nfpm.yaml \
		--packager deb \
		--target $(DISTDIR)/$(1)-$(2)/

package-rpm-$(1)-$(2) : $(DISTDIR)/$(1)-$(2)/nfpm.yaml $(DISTDIR)/changelog.yaml
	$(S) nfpm package \
		--config $(DISTDIR)/$(1)-$(2)/nfpm.yaml \
		--packager rpm \
		--target $(DISTDIR)/$(1)-$(2)/

package-tgz-$(1)-$(2) : $(DISTDIR)/$(1)-$(2)/sm-k6 $(DISTDIR)/$(1)-$(2)/synthetic-monitoring-agent $(ROOTDIR)/CHANGELOG.md $(ROOTDIR)/README.md $(ROOTDIR)/LICENSE
	# Create a tarball including the binaries, changelog, and readme. --transform is used to place all files at the root
	# of the tarball. The built-in make variable dollar-caret is escaped with two dollars so it survives package_template.
	$(S) tar --transform 's|.*/||' -zcf $(DISTDIR)/$(1)-$(2)/synthetic-monitoring-agent-$(firstword $(subst -, ,$(BUILD_VERSION)))-$(1)-$(2).tar.gz $$^

package-$(1)-$(2) : package-deb-$(1)-$(2) package-rpm-$(1)-$(2) package-tgz-$(1)-$(2)
	@true

package : package-$(1)-$(2)
endef

$(foreach BUILD_PLATFORM,$(PLATFORMS), \
	$(eval $(call package_template,$(word 1,$(subst /, ,$(BUILD_PLATFORM))),$(word 2,$(subst /, ,$(BUILD_PLATFORM))))))

$(DISTDIR)/changelog.yaml: $(DISTDIR)/version
	$(S) chglog init \
		--conventional-commits \
		--exclude-merge-commits \
		--owner 'Grafana Labs <support@grafana.com>' \
		--config-file '$(ROOTDIR)/scripts/package/chglog.yaml' \
		--output '$@'

.PHONY: package
package:
	@true

package-native : package-$(HOST_OS)-$(HOST_ARCH)
	@true
