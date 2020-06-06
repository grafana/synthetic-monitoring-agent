#!/bin/bash
set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")

sudo apt-get install rpm

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
BB_EXPORTER_URL=http://ftp.us.debian.org/debian/pool/main/p/prometheus-blackbox-exporter/
BB_EXPORTER_FILE=prometheus-blackbox-exporter_0.16.0+ds-1_amd64.deb
BB_EXPORTER_FILE_PATH=${BUILD_ROOT}/${BB_EXPORTER_FILE}
BB_EXPORTER_DIR=${BUILD_ROOT}/deb_bb_exporter_files
mkdir -p ${BB_EXPORTER_DIR}
if [ ! -f ${BB_EXPORTER_FILE_PATH} ]; then
    curl -o ${BB_EXPORTER_FILE_PATH} ${BB_EXPORTER_URL}${BB_EXPORTER_FILE}
fi 
dpkg-deb -R ${BB_EXPORTER_FILE_PATH} ${BB_EXPORTER_DIR}

## Setup for the package name
BUILD=${BUILD_ROOT}/systemd
PACKAGE_NAME="${BUILD}/worldping-blackbox-sidecar-${VERSION}_${ARCH}.deb"
mkdir -p ${BUILD}/usr/bin
mkdir -p ${BUILD}/lib/systemd/system/
mkdir -p ${BUILD}/etc/worldping

## Copy the blackbox_exporter files in
mkdir -p ${BUILD}/usr/share/doc
mkdir -p ${BUILD}/etc/prometheus
mkdir -p ${BUILD}/etc/default
cp -r ${BB_EXPORTER_DIR}/usr/* ${BUILD}/usr
cp -r ${BB_EXPORTER_DIR}/lib/* ${BUILD}/lib
cp -r ${BB_EXPORTER_DIR}/etc/prometheus/* ${BUILD}/etc/prometheus
cp -r ${BB_EXPORTER_DIR}/etc/default/* ${BUILD}/etc/default

cp ${BUILD_ROOT}/../worldping-blackbox-sidecar ${BUILD}/usr/bin/

fpm -s dir -t deb \
  -v ${VERSION} -n worldping-blackbox-sidecar -a ${ARCH} --description "worldPing blackbox_exporter sidecar agent" \
  --deb-systemd ${BASE}/config/systemd/worldping-blackbox-sidecar.service \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .


## CentOS 7
BUILD=${BUILD_ROOT}/systemd-centos7

mkdir -p ${BUILD}/usr/bin
mkdir -p ${BUILD}/lib/systemd/system/
mkdir -p ${BUILD}/etc/worldping

## Copy the blackbox_exporter files in
mkdir -p ${BUILD}/etc/prometheus
mkdir -p ${BUILD}/etc/default
cp -r ${BB_EXPORTER_DIR}/usr/* ${BUILD}/usr
cp -r ${BB_EXPORTER_DIR}/lib/* ${BUILD}/lib
cp -r ${BB_EXPORTER_DIR}/etc/prometheus/* ${BUILD}/etc/prometheus
cp -r ${BB_EXPORTER_DIR}/etc/default/* ${BUILD}/etc/default

cp ${BUILD_ROOT}/../worldping-blackbox-sidecar ${BUILD}/usr/bin/
cp ${BASE}/config/systemd/worldping-blackbox-sidecar.service $BUILD/lib/systemd/system

PACKAGE_NAME="${BUILD}/worldping-blackbox-sidecar-${VERSION}.el7.${ARCH}.rpm"

fpm -s dir -t rpm \
  -v ${VERSION} -n worldping-blackbox-sidecar -a ${ARCH} --description "worldPing blackbox_exporter sidecar agent" \
  --config-files /etc/worldping/ \
  -m "$CONTACT" --vendor "$VENDOR" --license "$LICENSE" \
  -C ${BUILD} -p ${PACKAGE_NAME} .

