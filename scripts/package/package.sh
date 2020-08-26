#!/bin/bash

# Script to package the synthetic-monitoring-agent and the prometheus 
# blackbox-exporter together.

set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")

SUDO=""
if [ $(id -u) -gt 0 ]; then
  SUDO="sudo"
fi

# Update apt's package list (security packages might have changed updated)
$SUDO apt-get update

# Install fpm if needed
if [ ! -x "$(which fpm)" ] ; then
  $SUDO apt-get install -y ruby ruby-dev rubygems build-essential
  $SUDO gem install --no-document fpm
fi

BUILD_OUTPUT=${CODE_DIR}/dist
BUILD_ROOT=$CODE_DIR/dist/build
mkdir -p ${BUILD_ROOT}

ARCH="$(uname -m)"
VERSION="$(${CODE_DIR}/scripts/version)"
## trim "v" prefix of version
VERSION=${VERSION#?}
CONTACT="Grafana Labs <hello@grafana.com>"
VENDOR="grafana.com"
LICENSE="Apache2.0"


## ubuntu 16.04, 18.04, Debian 8

## Setup for the package name
BUILD=${BUILD_ROOT}/systemd
CONFIG_DIR=$BASE/config/systemd
PACKAGE_NAME="${BUILD_OUTPUT}/synthetic-monitoring-agent-${VERSION}_${ARCH}.deb"
[ -e ${PACKAGE_NAME} ] && rm ${PACKAGE_NAME}

# Copy config files in
copy_files_into_pkg () {
  # Setup dirs
  mkdir -p ${BUILD}/usr/bin
  mkdir -p ${BUILD}/lib/systemd/system/
  mkdir -p ${BUILD}/etc/synthetic-monitoring

  cp ${BUILD_OUTPUT}/synthetic-monitoring-agent ${BUILD}/usr/bin/

  # Copy config files in
  cp ${CONFIG_DIR}/synthetic-monitoring-agent.conf ${BUILD}/etc/synthetic-monitoring
  cp ${CONFIG_DIR}/synthetic-monitoring-agent.service ${BUILD}/lib/systemd/system
}
copy_files_into_pkg

fpm -s dir -t deb \
  -v ${VERSION} -n synthetic-monitoring-agent -a ${ARCH} --description "synthetic monitoring agent" \
  --deb-systemd ${CONFIG_DIR}/synthetic-monitoring-agent.service \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .


## CentOS 7
$SUDO apt-get install -y rpm

PACKAGE_NAME="${BUILD_OUTPUT}/synthetic-monitoring-agent-${VERSION}.el7.${ARCH}.rpm"
[ -e ${PACKAGE_NAME} ] && rm ${PACKAGE_NAME}

fpm -s dir -t rpm \
  -v ${VERSION} -n synthetic-monitoring-agent -a ${ARCH} --description "synthetic monitoring agent" \
  --config-files /etc/synthetic-monitoring/ \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .

