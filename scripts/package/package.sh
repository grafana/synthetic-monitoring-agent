#!/bin/bash

# Script to package the synthetic-monitoring-agent

set -x

BASE=$(dirname "$0")
CODE_DIR=$(readlink -e "$BASE/../../")

VERSION="$("${CODE_DIR}/scripts/version")"
## trim "v" prefix of version
VERSION="${VERSION#?}"
CONTACT="Grafana Labs <hello@grafana.com>"
VENDOR="grafana.com"
LICENSE="Apache2.0"

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
	SUDO="sudo"
fi

# Update apt's package list (security packages might have changed updated)
$SUDO apt-get update

# Install fpm if needed
if [ ! -x "$(which fpm)" ]; then
	$SUDO apt-get install -y ruby ruby-dev rubygems build-essential
	$SUDO gem install --no-document fpm -v 1.13.1
fi

# Install rpm if needed
if [ ! -x "$(which rpm)" ]; then
	$SUDO apt-get install -y rpm
fi

CONFIG_DIR=$BASE/config/systemd

# Copy config files in
copy_files_into_pkg() {
	local BUILD=$1
	local BUILD_OUTPUT=$2

	# Setup dirs
	mkdir -p "${BUILD}/usr/bin"
	mkdir -p "${BUILD}/lib/systemd/system/"
	mkdir -p "${BUILD}/etc/synthetic-monitoring"

	cp "${BUILD_OUTPUT}/synthetic-monitoring-agent" "${BUILD}/usr/bin/"

	# Copy config files in
	cp "${CONFIG_DIR}/synthetic-monitoring-agent.conf" "${BUILD}/etc/synthetic-monitoring"
	cp "${CONFIG_DIR}/synthetic-monitoring-agent.service" "${BUILD}/lib/systemd/system"
}

# Create Debian package
create_deb_package() {
	local ARCH=$1
	local BUILD=$2
	local BUILD_OUTPUT=$3

	PACKAGE_NAME="${BUILD_OUTPUT}/synthetic-monitoring-agent-${VERSION}_${ARCH}.deb"

	[ -e "${PACKAGE_NAME}" ] && rm "${PACKAGE_NAME}"

	fpm \
		-s dir \
		-t deb \
		-v "${VERSION}" -n synthetic-monitoring-agent \
		-a "${ARCH}" \
		--description "synthetic monitoring agent" \
		--deb-systemd "${CONFIG_DIR}/synthetic-monitoring-agent.service" \
		-m "$CONTACT" \
		--vendor "$VENDOR" \
		--license "$LICENSE" \
		-C "${BUILD}" \
		-p "${PACKAGE_NAME}" \
		.
}

# Create RPM package
create_rpm_package() {
	local ARCH=$1
	local BUILD=$2
	local BUILD_OUTPUT=$3

	case "${ARCH}" in
	amd64)
		rpm_arch=x86_64
		;;
	*)
		rpm_arch=${ARCH}
		;;
	esac

	PACKAGE_NAME="${BUILD_OUTPUT}/synthetic-monitoring-agent-${VERSION}.el7.${rpm_arch}.rpm"
	[ -e "${PACKAGE_NAME}" ] && rm "${PACKAGE_NAME}"

	fpm \
		-s dir \
		-t rpm \
		-v "${VERSION}" \
		-n synthetic-monitoring-agent \
		-a "${rpm_arch}" \
		--description "synthetic monitoring agent" \
		--config-files /etc/synthetic-monitoring/ \
		-m "$CONTACT" \
		--vendor "$VENDOR" \
		--license "$LICENSE" \
		-C "${BUILD}" \
		-p "${PACKAGE_NAME}" \
		.
}

for pkg_arch in amd64 arm arm64 ; do
	BUILD_OUTPUT="${CODE_DIR}/dist/linux-${pkg_arch}"
	BUILD_ROOT="${BUILD_OUTPUT}/build"

	mkdir -p "${BUILD_ROOT}"

	copy_files_into_pkg "${BUILD_ROOT}" "${BUILD_OUTPUT}"

	## ubuntu 16.04, 18.04, Debian 8
	create_deb_package "${pkg_arch}" "${BUILD_ROOT}" "${BUILD_OUTPUT}"

	## CentOS 7
	create_rpm_package "${pkg_arch}" "${BUILD_ROOT}" "${BUILD_OUTPUT}"
done
