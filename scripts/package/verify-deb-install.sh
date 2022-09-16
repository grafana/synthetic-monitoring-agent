#!/usr/bin/env sh

set -euxo pipefail

docker ps
image="$(docker ps --filter ancestor=jrei/systemd-debian:12 --latest --format "{{.ID}}")"
echo "Running on container: ${image}"

dir="."
if [ -n "${CI}" ]; then
    dir="/drone/src"
fi
echo "Running on directory: ${dir}"

cat <<EOF | docker exec --interactive "${image}" sh
    set -x

    # Install the agent and check the files are in the right place
    dpkg -i ${dir}/dist/synthetic-monitoring-agent*amd64.deb

    if [ ! -x "\$(command -v synthetic-monitoring-agent)" ]; then
        echo "ERROR: synthetic-monitoring-agent not installed."
        exit 1
    fi

    if [ ! -f "/etc/synthetic-monitoring/synthetic-monitoring-agent.conf" ]; then
        echo "ERROR: synthetic-monitoring-agent config file not installed."
        exit 1
    fi
EOF