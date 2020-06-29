#!/bin/bash
#
# Publish the artifacts from package.sh to GCS.
#
set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")
BUILD_DEB_DIR=${CODE_DIR}/dist
BUILD_RMP_DIR=${CODE_DIR}/dist
GCS_KEY_DIR=${GCS_KEY_DIR:-/keys}

SUDO=""
if [ $(id -u) -gt 0 ]; then
  SUDO="sudo"
fi

# Setup directories 
PUBLISH_ROOT=${CODE_DIR}/dist/publish
mkdir -p ${PUBLISH_ROOT}
APTLY_REPO=${PUBLISH_ROOT}/deb/repo
mkdir -p ${APTLY_REPO}
APTLY_DIR=${PUBLISH_ROOT}/deb
mkdir -p ${APTLY_DIR}
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
set +x

# UNCOMMENT to use test GPG keys
#source ${BASE}/gpg-test-vars.sh
if [ -z "${GPG_PRIV_KEY}" ] ; then
    echo "Error: GPG_PRIV_KEY not set."
    exit 1
fi

# Import GPG keys 
GPG_PRIV_KEY_FILE=${BASE}/priv.key
echo $GPG_PRIV_KEY | base64 -d > ${GPG_PRIV_KEY_FILE}
gpg --batch --yes --no-tty --allow-secret-key-import --import ${GPG_PRIV_KEY_FILE}

set -x

# Activate GCS service account
gcloud auth activate-service-account --key-file=${GCS_KEY_DIR}/gcs-key.json

### DEBIAN 

# Install aptly if not already
if [ ! -x "$(which aptly)" ] ; then
  $SUDO apt-key adv --keyserver pool.sks-keyservers.net --recv-keys ED75B5A4483DA07C
  wget -qO - https://www.aptly.info/pubkey.txt | $SUDO apt-key add -
  $SUDO add-apt-repository "deb http://repo.aptly.info/ squeeze main"
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

### TODO: RPM
#$SUDO apt-get install -y rpm


# Done, cleanup and exit
cleanup () {
	rm ${GPG_PRIV_KEY_FILE}
	exit 0
}
cleanup
