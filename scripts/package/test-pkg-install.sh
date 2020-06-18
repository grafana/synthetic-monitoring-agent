#!/bin/bash

# Ubuntu test pulling .deb package and installing. Ensure sidecar and blackbox_exporter
# are installed and in the path.

# Setup
sudo apt-get update
sudo apt-get install -y apt-transport-https
sudo apt-get install -y software-properties-common wget

# Add worldping test repo to apt
wget -q -O - https://wp-testing-repo.storage.googleapis.com/gpg.key | sudo apt-key add -
sudo add-apt-repository "deb https://wp-testing-repo.storage.googleapis.com/deb stable main"

# Try installing
sudo apt-get install worldping-blackbox-sidecar

# Test if things were installed
if [ ! -x "$(which worldping-blackbox-sidecar)" ] ; then
  echo "ERROR: worldping-blackbox-sidecar not installed."
  exit 1
fi

if [ ! -x "$(which blackbox_exporter)" ] ; then
  echo "ERROR: prometheus-blackbox-exporter not installed."
  exit 1
fi

echo "Success"
