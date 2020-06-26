#!/bin/bash

# Ubuntu test pulling .deb package and installing. Ensure sidecar and blackbox_exporter
# are installed and in the path.

# Repo url's for prod and test
PROD_REPO="https://packages-sm.grafana.com"
TEST_REPO="https://sm-testing-repo.storage.googleapis.com"

# Set repo to test
REPO_URL=${TEST_REPO}

# Setup
sudo apt-get update
sudo apt-get install -y apt-transport-https
sudo apt-get install -y software-properties-common wget

# Add synthetic-monitoring test repo to apt
wget -q -O - ${REPO_URL}/gpg.key | sudo apt-key add -
sudo add-apt-repository "deb ${REPO_URL}/deb stable main"

# Try installing
sudo apt-get install synthetic-monitoring-agent

# Test if things were installed
if [ ! -x "$(which synthetic-monitoring-agent)" ] ; then
  echo "ERROR: synthetic-monitoring-agent not installed."
  exit 1
fi

echo "Success"
