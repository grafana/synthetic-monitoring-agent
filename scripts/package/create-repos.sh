#!/bin/bash
#
# WARNING: This is NOT a script that should be used often and only when starting 
# repos from scratch. Sync with GCS has been commented out to keep from inadvertently
# overwriting production repos.
#
set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")

PUBLISH_ROOT=${CODE_DIR}/dist/publish
mkdir -p ${PUBLISH_ROOT}
APTLY_DIR=${PUBLISH_ROOT}/deb
mkdir -p ${APTLY_DIR}
APTLY_DB=${PUBLISH_ROOT}/deb/db
mkdir -p ${APTLY_DB}
APTLY_REPO=${PUBLISH_ROOT}/deb/repo
mkdir -p ${APTLY_REPO}

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

ARCH="$(uname -m)"

APTLY_CONF_FILE=${PUBLISH_ROOT}/aptly.conf

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

# Activate GCS service account
gcloud auth activate-service-account --key-file=/keys/gcs-key.json

# Install aptly if not already
if [ ! -x "$(which aptly)" ] ; then
  sudo apt-key adv --keyserver pool.sks-keyservers.net --recv-keys ED75B5A4483DA07C
  wget -qO - https://www.aptly.info/pubkey.txt | sudo apt-key add -
  sudo add-apt-repository "deb http://repo.aptly.info/ squeeze main"
  sudo apt-get update
  sudo apt-get install aptly
fi

# write the aptly.conf file, will be overwritten if exists
cat << EOF > ${APTLY_CONF_FILE}
{
  "rootDir": "${APTLY_DIR}",
  "downloadConcurrency": 4,
  "downloadSpeedLimit": 0,
  "architectures": ["amd64", "arm64", "armhf", "i386"],
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


# Create Debian repo
aptly -config=${APTLY_CONF_FILE} repo create -distribution="stable" synthetic-monitoring

# Publish blank repo
aptly -config=${APTLY_CONF_FILE} publish repo -batch -force-overwrite synthetic-monitoring filesystem:repo:synthetic-monitoring

# UNCOMMENT: Commented out to keep from inadvertently overwriting the published
# repo. Uncomment if a new repo really needs to be sync'd.
# Sync to GCS
#gsutil -m rsync -r ${APTLY_DIR} gs://${APTLY_DB_BUCKET}


#TODO: RPM Repo creation
#sudo apt-get install -y rpm


# Done, cleanup and exit
cleanup () {
	rm ${GPG_PRIV_KEY_FILE}
	exit 0
}
cleanup
