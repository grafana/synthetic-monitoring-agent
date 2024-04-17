#!/usr/bin/env bash
# shellcheck disable=SC3044
#
# Generate all protobuf bindings.
# Run from repository root.
set -e
set -u

if test ! -e "scripts/genproto.sh" ; then
	echo "must be run from repository root"
	exit 255
fi

DIRS="pkg/pb/synthetic_monitoring"

for dir in ${DIRS}; do
	echo "Generating Protocol Buffers code in ${dir}"
	pushd "${dir}" > /dev/null

	# Check that are no breaking changes
	buf breaking . --against ./checks.binpb

	# Generate new code
	buf generate
	goimports -w .

	# Capture the new data to validate future changes against
	buf build --output checks.binpb
	popd > /dev/null
done
