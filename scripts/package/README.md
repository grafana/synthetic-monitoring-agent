# Packaging Synthetic Monitoring Agent

Use the `make release` target to publish to Github or `make release-snapshot` to test the packaging.

The following scripts are used for packaging `synthetic-monitoring-agent`:

Script | Usage
------ | -----
`goreleaser-build` | Used by `goreleaser` to build the binaries
`verify-rpm-install.sh` | Script triggered by CI to verify that the RPM package installs correctly and contains the expected files
`verify-deb-install.sh` | Script triggered by CI to verify that the Debian package installs correctly and contains the expected files
