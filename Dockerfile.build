# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS build
RUN apk --no-cache add --upgrade ca-certificates-bundle

# setcapper stage handles adding file capabilities where needed
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS setcapper
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add libcap

COPY --chown=sm:sm --chmod=0500 dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent

RUN setcap cap_net_raw=+ep /usr/local/bin/synthetic-monitoring-agent

# Base release copies the binaries, configuration and also the
# certificates from the first stage.
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH
ARG K6_V1_VERSION=v1.1.17
ARG K6_V2_VERSION=v2.0.12

RUN adduser -D -u 12345 -g 12345 sm

ADD --chown=sm:sm --chmod=0500 https://github.com/grafana/xk6-sm/releases/download/${K6_V1_VERSION}/sm-k6-${TARGETOS}-${TARGETARCH} /usr/libexec/sm-k6/k6-v1
ADD --chown=sm:sm --chmod=0500 https://github.com/grafana/xk6-sm/releases/download/${K6_V2_VERSION}/sm-k6-${TARGETOS}-${TARGETARCH} /usr/libexec/sm-k6/k6-v2
COPY --chown=sm:sm --chmod=0500 --from=setcapper /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --chown=sm:sm scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER sm
ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# Browser release copies the setup from the base agent and
# additionally installs Chromium to support browser checks.
FROM ghcr.io/grafana/chromium-swiftshader-alpine:155.0.8059.84-r0-3.24.3@sha256:df15b44385bedaf4591bb80739b40cb77e1c254e4be8ca9c65aa54c863fbeed8 AS with-browser
RUN apk --no-cache add --repository community tini

# The agent gets NET_RAW from its file capability. In the release image it's the entrypoint, so that's enough. Here
# tini starts it, and under no_new_privs (e.g. allowPrivilegeEscalation: false in Kubernetes) the agent can only keep
# capabilities tini already has. Give tini NET_RAW in its permitted set only (+p), so it can pass the capability on
# without having it active itself.
RUN apk --no-cache add --virtual .setcap libcap && \
	setcap cap_net_raw=+p "$(command -v tini)" && \
	apk del .setcap

RUN adduser -D -u 12345 -g 12345 sm

COPY --from=release --chown=sm:sm /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release --chown=sm:sm /usr/libexec/sm-k6/k6-v1 /usr/libexec/sm-k6/k6-v1
COPY --from=release --chown=sm:sm /usr/libexec/sm-k6/k6-v2 /usr/libexec/sm-k6/k6-v2
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Removing any file with setuid bit set, such as /usr/lib/chromium/chrome-sandbox,
# which is used for chromium sandboxing.
RUN find / -type f -perm -4000 -delete

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

USER sm
ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
