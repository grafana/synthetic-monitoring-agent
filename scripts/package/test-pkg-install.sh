#!/bin/bash

# Ubuntu test pulling .deb package and installing. Ensure sidecar and blackbox_exporter
# are installed and in the path.

# Setup
sudo apt-get update
sudo apt-get install -y apt-transport-https
sudo apt-get install -y software-properties-common wget

# Add synthetic-monitoring test repo to apt
wget -q -O - https://sm-testing-repo.storage.googleapis.com/gpg.key | sudo apt-key add -
sudo add-apt-repository "deb https://sm-testing-repo.storage.googleapis.com/deb stable main"

# Try installing
sudo apt-get install synthetic-monitoring-agent

# Test if things were installed
if [ ! -x "$(which synthetic-monitoring-agent)" ] ; then
  echo "ERROR: synthetic-monitoring-agent not installed."
  exit 1
fi

echo "Success"
