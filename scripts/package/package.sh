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
#TODO: Once a version is tagged, remove 0.0.9
VERSION=0.0.9-$(git describe --long --always)
CONTACT="Grafana Labs <hello@grafana.com>"
VENDOR="grafana.com"
LICENSE="Apache2.0"


## ubuntu 16.04, 18.04, Debian 8

## download the blackbox-exporter deb package if not present
BB_EXPORTER_URL=https://github.com/prometheus/blackbox_exporter/releases/download/v0.16.0/
BB_EXPORTER_TGZ=blackbox_exporter-0.16.0.linux-amd64.tar.gz
BB_EXPORTER_TGZ_PATH=${BUILD_ROOT}/${BB_EXPORTER_TGZ}
BB_EXPORTER_DIR=${BUILD_ROOT}/bb_exporter_files
mkdir -p ${BB_EXPORTER_DIR}
if [ ! -f ${BB_EXPORTER_TGZ_PATH} ]; then
    wget -O ${BB_EXPORTER_TGZ_PATH} ${BB_EXPORTER_URL}${BB_EXPORTER_TGZ}
fi 

# Extract to a temp directory without folders
tar xzf ${BB_EXPORTER_TGZ_PATH} -C ${BB_EXPORTER_DIR} --strip-components=1


## Setup for the package name
BUILD=${BUILD_ROOT}/systemd
CONFIG_DIR=$BASE/config/systemd
PACKAGE_NAME="${BUILD}/worldping-blackbox-sidecar-${VERSION}_${ARCH}.deb"

# Copy config files in
copy_files_into_pkg () {
  # Setup dirs
  mkdir -p ${BUILD}/usr/bin
  mkdir -p ${BUILD}/lib/systemd/system/
  mkdir -p ${BUILD}/etc/worldping

  # Copy the blackbox_exporter files in
  cp ${BB_EXPORTER_DIR}/blackbox_exporter ${BUILD}/usr/bin
  cp ${BUILD_OUTPUT}/worldping-blackbox-sidecar ${BUILD}/usr/bin/

  # Copy config files in
  cp ${CONFIG_DIR}/worldping.conf ${BUILD}/etc/worldping
  cp ${CONFIG_DIR}/blackbox.yml ${BUILD}/etc/worldping
  cp ${CONFIG_DIR}/worldping-blackbox-exporter.service ${BUILD}/lib/systemd/system
  cp ${CONFIG_DIR}/worldping-blackbox-sidecar.service ${BUILD}/lib/systemd/system
}
copy_files_into_pkg

fpm -s dir -t deb \
  -v ${VERSION} -n worldping-blackbox-sidecar -a ${ARCH} --description "worldPing blackbox_exporter sidecar agent" \
  --deb-systemd ${CONFIG_DIR}/worldping-blackbox-sidecar.service \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .


## CentOS 7
BUILD=${BUILD_ROOT}/systemd-centos7

sudo apt-get install -y rpm

# Setup configs
copy_files_into_pkg

cp ${BB_EXPORTER_DIR}/blackbox_exporter ${BUILD}/usr/bin
cp ${BUILD_ROOT}/../worldping-blackbox-sidecar ${BUILD}/usr/bin/

PACKAGE_NAME="${BUILD}/worldping-blackbox-sidecar-${VERSION}.el7.${ARCH}.rpm"

fpm -s dir -t rpm \
  -v ${VERSION} -n worldping-blackbox-sidecar -a ${ARCH} --description "worldPing blackbox_exporter sidecar agent" \
  --config-files /etc/worldping/ \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .

