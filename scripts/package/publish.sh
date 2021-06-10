#!/bin/bash
#
# Publish the artifacts from package.sh to GCS.
#
set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")
BUILD_DEB_DIR=${CODE_DIR}/dist
BUILD_RPM_DIR=${CODE_DIR}/dist
GCS_KEY_DIR=${GCS_KEY_DIR:-/keys}

SUDO=""
if [ $(id -u) -gt 0 ]; then
  SUDO="sudo"
fi

PUBLISH_ROOT=${CODE_DIR}/dist/publish
mkdir -p ${PUBLISH_ROOT}

### Start deb handling

# Setup directories 
APTLY_DIR=${PUBLISH_ROOT}/deb
mkdir -p ${APTLY_DIR}
APTLY_REPO=${PUBLISH_ROOT}/deb/repo
mkdir -p ${APTLY_REPO}
APTLY_DB=${PUBLISH_ROOT}/deb/db
mkdir -p ${APTLY_DB}
APTLY_POOL=${PUBLISH_ROOT}/deb/pool
mkdir -p ${APTLY_POOL}
APTLY_STAGE=${PUBLISH_ROOT}/tmp
mkdir -p ${APTLY_STAGE} 
ARCH="$(uname -m)"

# Only publish to prod if env explicitly set.
if [ -n "${PUBLISH_PROD_PKGS}" ]; then
  # Production GCS buckets
  APTLY_DB_BUCKET=sm-aptly-db
  REPO_BUCKET=packages-sm.grafana.com
else
  # Testing GCS buckets
  APTLY_DB_BUCKET=sm-testing-aptly-db
  REPO_BUCKET=sm-testing-repo
fi

APTLY_CONF_FILE=${PUBLISH_ROOT}/aptly.conf

# avoid printing our gpg key to stdout
#set +x

# UNCOMMENT to use test GPG keys
#source ${BASE}/gpg-test-vars.sh
if [ -z "${GPG_PRIV_KEY}" ] ; then
    echo "Error: GPG_PRIV_KEY not set."
    exit 1
fi

if [ ! -x "$(which gpg2)" ] ; then
  $SUDO apt-get install -y gnupg2
fi

# Import GPG keys 
GPG_PRIV_KEY_FILE=${BASE}/priv.key
echo "$GPG_PRIV_KEY" | base64 -d > ${GPG_PRIV_KEY_FILE}
gpg2 --batch --yes --no-tty --allow-secret-key-import --import ${GPG_PRIV_KEY_FILE}

#set -x

if [ ! -x "$(which gcloud)" ] ; then
  # Download gcloud package
  curl https://dl.google.com/dl/cloudsdk/release/google-cloud-sdk.tar.gz > /tmp/google-cloud-sdk.tar.gz
  # Install the gcloud package
  $SUDO mkdir -p /usr/local/gcloud && \
    $SUDO tar -C /usr/local/gcloud -xf /tmp/google-cloud-sdk.tar.gz && \
    $SUDO /usr/local/gcloud/google-cloud-sdk/install.sh --quiet

  # Add gcloud to the path
  PATH=$PATH:/usr/local/gcloud/google-cloud-sdk/bin
fi

# Activate GCS service account
gcloud auth activate-service-account --key-file=${GCS_KEY_DIR}/gcs-key.json

### DEBIAN 

# Install aptly if not already
if [ ! -x "$(which aptly)" ] ; then
  $SUDO apt-key adv --keyserver pool.sks-keyservers.net --recv-keys ED75B5A4483DA07C
  wget -qO - https://www.aptly.info/pubkey.txt | $SUDO apt-key add -
  $SUDO sh -c 'echo "deb http://repo.aptly.info/ squeeze main" > /etc/apt/sources.list.d/aptly.list'
  $SUDO apt-get update
  $SUDO apt-get install aptly
fi

# write the aptly.conf file, will be rewritten if exists
cat << EOF > ${APTLY_CONF_FILE}
{
  "rootDir": "${APTLY_DIR}",
  "downloadConcurrency": 4,
  "downloadSpeedLimit": 0,
  "architectures": [],
  "dependencyFollowSuggests": false,
  "dependencyFollowRecommends": false,
  "dependencyFollowAllVariants": false,
  "dependencyFollowSource": false,
  "dependencyVerboseResolve": false,
  "gpgDisableSign": false,
  "gpgDisableVerify": false,
  "gpgProvider": "gpg2",
  "downloadSourcePackages": false,
  "skipLegacyPool": true,
  "ppaDistributorID": "ubuntu",
  "ppaCodename": "",
  "skipContentsPublishing": false,
  "FileSystemPublishEndpoints": {
    "repo": {
        "rootDir": "${APTLY_REPO}",
        "linkMethod": "copy"
    }
  },
  "S3PublishEndpoints": {},
  "SwiftPublishEndpoints": {}
}
EOF

# Pull deb database
gsutil -m rsync -d -r gs://${APTLY_DB_BUCKET} ${APTLY_DIR} 

# Copy packages to the repo
cp ${BUILD_DEB_DIR}/*.deb ${APTLY_STAGE}

# Add packages to deb repo
aptly -config=${APTLY_CONF_FILE} repo add -force-replace synthetic-monitoring ${APTLY_STAGE}

# Update local deb repo
aptly -config=${APTLY_CONF_FILE} publish update -batch -force-overwrite stable filesystem:repo:synthetic-monitoring

# Can set DISABLE_REPO_PUB=1 for testing to skip publishing to buckets.
if [ -z "${DISABLE_REPO_PUB}" ] ; then

  # Push binaries before the db 
  gsutil -m rsync -r ${APTLY_POOL} gs://${REPO_BUCKET}/deb/pool
  # Push the deb db
  gsutil -m rsync -r ${APTLY_REPO}/synthetic-monitoring gs://${REPO_BUCKET}/deb

fi

### End deb handling

### Start rpm handling

$SUDO apt-get install -y createrepo

# Setup directories 
RPM_REPO_DIR=${PUBLISH_ROOT}/rpm
mkdir -p "${RPM_REPO_DIR}"
RPM_DATA_DIR=${PUBLISH_ROOT}/rpm/repodata
mkdir -p "${RPM_DATA_DIR}"
RPM_POOL_DIR=${PUBLISH_ROOT}/rpm/pool
mkdir -p "${RPM_POOL_DIR}"

for rpm in "${BUILD_RPM_DIR}"/*.rpm ; do
	rpm_hash=$(sha256sum "${rpm}" | cut -d' ' -f1)
	rpm_dir="${RPM_POOL_DIR}"/$(echo "${rpm_hash}" | cut -c1-2)/$(echo "${rpm_hash}" | cut -c3-4)/$(echo "${rpm_hash}" | cut -c5-)
	mkdir -p "${rpm_dir}"
	cp "${rpm}" "${rpm_dir}"
done

createrepo "${RPM_REPO_DIR}"

if [ -z "${DISABLE_REPO_PUB}" ] ; then
  # Push binaries before the db
  gsutil -m rsync -r ${RPM_POOL_DIR} gs://${REPO_BUCKET}/rpm/pool
  gsutil -m rsync -r ${RPM_DATA_DIR} gs://${REPO_BUCKET}/rpm/repodata
fi

### End rpm handling

# Done, cleanup and exit
cleanup () {
	rm ${GPG_PRIV_KEY_FILE}
	exit 0
}
cleanup
