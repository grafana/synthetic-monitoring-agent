#!/bin/bash

# Script to package the worldping-blackbox-sidecar and the prometheus 
# blackbox-exporter together.

set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")

# Install fpm if needed
if [ ! -x "$(which fpm)" ] ; then
  sudo apt-get install -y ruby ruby-dev rubygems build-essential
  gem install --no-document fpm
fi

BUILD_OUTPUT=${CODE_DIR}/dist
BUILD_ROOT=$CODE_DIR/dist/build
mkdir -p $(BUILD_ROOT)

ARCH="$(uname -m)"
VERSION=$(git describe --long --always)
## trim "v" prefix of version
VERSION=${VERSION#?}
CONTACT="Grafana Labs <hello@grafana.com>"
VENDOR="grafana.com"
LICENSE="Apache2.0"


## ubuntu 16.04, 18.04, Debian 8

## Setup for the package name
BUILD=${BUILD_ROOT}/systemd
CONFIG_DIR=$BASE/config/systemd
PACKAGE_NAME="${BUILD}/worldping-blackbox-sidecar-${VERSION}_${ARCH}.deb"
[ -e ${PACKAGE_NAME} ] && rm ${PACKAGE_NAME}

# Copy config files in
copy_files_into_pkg () {
  # Setup dirs
  mkdir -p ${BUILD}/usr/bin
  mkdir -p ${BUILD}/lib/systemd/system/
  mkdir -p ${BUILD}/etc/worldping

  cp ${BUILD_OUTPUT}/worldping-blackbox-sidecar ${BUILD}/usr/bin/

  # Copy config files in
  cp ${CONFIG_DIR}/worldping.conf ${BUILD}/etc/worldping
  cp ${CONFIG_DIR}/worldping-blackbox-sidecar.service ${BUILD}/lib/systemd/system
}
copy_files_into_pkg

fpm -s dir -t deb \
  -v ${VERSION} -n worldping-blackbox-sidecar -a ${ARCH} --description "worldPing blackbox_exporter sidecar agent" \
  --deb-systemd ${CONFIG_DIR}/worldping-blackbox-sidecar.service \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .


## CentOS 7
sudo apt-get install -y rpm

PACKAGE_NAME="${BUILD}/worldping-blackbox-sidecar-${VERSION}.el7.${ARCH}.rpm"
[ -e ${PACKAGE_NAME} ] && rm ${PACKAGE_NAME}

fpm -s dir -t rpm \
  -v ${VERSION} -n worldping-blackbox-sidecar -a ${ARCH} --description "worldPing blackbox_exporter sidecar agent" \
  --config-files /etc/worldping/ \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .

