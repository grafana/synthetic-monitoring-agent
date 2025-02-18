# This file is used by the makefile in order to obtain the version of the
# Grafana Build Tools image to use. This is *also* used by scripts/docker-run
# to obtain the same information. That means this file must be both a Makefile
# and a shell script. This is achieved by using the `VAR=value` syntax, which
# is valid in both Makefile and shell.

GBT_IMAGE=ghcr.io/grafana/grafana-build-tools:v0.40.1@sha256:547fe54d723dfe9e045305cf1bcb9d1dbc99f797e5ddd2d4acb7082d81e1f0e7
