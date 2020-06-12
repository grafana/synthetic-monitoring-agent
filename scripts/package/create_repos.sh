#!/bin/bash
#
# This is NOT a script that should be used often and only when starting a repo from scratch.
#

set -x
BASE=$(dirname $0)
CODE_DIR=$(readlink -e "$BASE/../../")
echo "Base: $BASE"
echo "Coe: $CODE_DIR"

#TODO: Put this back for RPM
#sudo apt-get install rpm

PUBLISH_ROOT=${CODE_DIR}/dist/publish
mkdir -p ${PUBLISH_ROOT}
APTLY_DIR=${PUBLISH_ROOT}/deb
mkdir -p ${APTLY_DB}
APTLY_DB=${PUBLISH_ROOT}/deb/db
mkdir -p ${APTLY_DB}
APTLY_REPO=${PUBLISH_ROOT}/deb/repo
mkdir -p ${APTLY_REPO}

APTLY_DB_BUCKET=wp-testing-aptly-db
REPO_BUCKET=wp-testing-repo

ARCH="$(uname -m)"

APTLY_CONF_FILE=${PUBLISH_ROOT}/aptly.conf

# Will need the GPG keys imported
GPG_PRIV_KEY_FILE=${BASE}/priv.key
GPG_PASSPHRASE_FILE=${BASE}/passphrase
echo $GPG_PRIV_KEY | base64 -d > ${GPG_PRIV_KEY_FILE}
echo $GPG_KEY_PASSWORD > ${GPG_PASSPHRASE_FILE}
gpg --batch --yes --no-tty --allow-secret-key-import --import ${GPG_PRIV_KEY_FILE}


# Install aptly if not already
if [ ! -x "$(which aptly)" ] ; then
  sudo apt-key adv --keyserver pool.sks-keyservers.net --recv-keys ED75B5A4483DA07C
  wget -qO - https://www.aptly.info/pubkey.txt | sudo apt-key add -
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
  "gpgProvider": "gpg",
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
aptly -config=${APTLY_CONF_FILE} repo create -distribution="stable" worldping

# Publish blank repo
aptly -config=${APTLY_CONF_FILE} publish repo -batch -passphrase-file=./scripts/package/passphrase -force-overwrite worldping filesystem:repo:worldping

# Sync to GCS
gsutil -m rsync -r ${APTLY_DIR} gs://${APTLY_DB_BUCKET}


#TODO: RPM Repo creation

