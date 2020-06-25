#!/bin/bash
#
# Publish the artifacts from package.sh to GCS.
#
set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")
BUILD_DEB_DIR=${CODE_DIR}/dist/build/systemd
BUILD_RMP_DIR=${CODE_DIR}/dist/build/systemd-centos7

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

# GCS buckets, deb db not in public bucket
APTLY_DB_BUCKET=sm-testing-aptly-db
REPO_BUCKET=sm-testing-repo

APTLY_CONF_FILE=${PUBLISH_ROOT}/aptly.conf

# UNCOMMENT to use test GPG keys
#source ${BASE}/gpg-test-vars.sh
if [ -z "${GPG_PRIV_KEY}" ] || [ -z "${GPG_KEY_PASSWORD}" ] ; then
    echo "Error: GPG_PRIV_KEY and GPG_KEY_PASSWORD not set."
    exit 1
fi

# Import GPG keys 
GPG_PRIV_KEY_FILE=${BASE}/priv.key
GPG_PASSPHRASE_FILE=${BASE}/passphrase
echo $GPG_PRIV_KEY | base64 -d > ${GPG_PRIV_KEY_FILE}
echo $GPG_KEY_PASSWORD > ${GPG_PASSPHRASE_FILE}
gpg --batch --yes --no-tty --allow-secret-key-import --import ${GPG_PRIV_KEY_FILE}

### DEBIAN 

# Install aptly if not already
if [ ! -x "$(which aptly)" ] ; then
  sudo apt-key adv --keyserver pool.sks-keyservers.net --recv-keys ED75B5A4483DA07C
  wget -qO - https://www.aptly.info/pubkey.txt | sudo apt-key add -
  sudo add-apt-repository "deb http://repo.aptly.info/ squeeze main"
  sudo apt-get update
  sudo apt-get install aptly
fi

# Activate GCS service account
gcloud auth activate-service-account --key-file=/keys/gcs-key.json

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
gsutil -m rsync -r gs://${APTLY_DB_BUCKET} ${APTLY_DIR} 

# Copy packages to the repo
cp ${BUILD_DEB_DIR}/*.deb ${APTLY_STAGE}

# Add packages to deb repo
aptly -config=${APTLY_CONF_FILE} repo add -force-replace synthetic-monitoring-agent ${APTLY_STAGE}

# Update local deb repo
aptly -config=${APTLY_CONF_FILE} publish update -batch -passphrase-file=${BASE}/passphrase -force-overwrite stable filesystem:repo:synthetic-monitoring-agent

# Can set DISABLE_REPO_PUB=1 for testing to skip publishing to buckets.
if [ -z "${DISABLE_REPO_PUB}" ] ; then

  # Push binaries before the db 
  gsutil -m rsync -r ${APTLY_POOL} gs://${REPO_BUCKET}/deb/pool
  # Push the deb db
  gsutil -m rsync -r ${APTLY_REPO}/synthetic-monitoring-agent gs://${REPO_BUCKET}/deb

fi

### TODO: RPM
#sudo apt-get install -y rpm



# Done for some reason, cleanup and exit
cleanup () {
	# cleanup passphrase and key files
	rm ${GPG_PRIV_KEY_FILE}
	rm ${GPG_PASSPHRASE_FILE}

	exit 0
}
cleanup
