# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS build
RUN apk --no-cache add ca-certificates-bundle

# setcapper stage handles adding file capabilities where needed
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS setcapper
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add libcap

COPY --chown=sm:sm --chmod=0500 dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent

RUN setcap cap_net_raw=+ep /usr/local/bin/synthetic-monitoring-agent

# Base release copies the binaries, configuration and also the
# certificates from the first stage.
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH
ARG K6_V1_VERSION=v1.1.9
ARG K6_V2_VERSION=v2.0.4

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
FROM ghcr.io/grafana/chromium-swiftshader-alpine:149.0.7827.53-r0-3.23.4@sha256:35312b0c6824db3fa2976ef421ce13799dcc080e724f601e16c1d48febd119a4 AS with-browser
RUN apk --no-cache add --repository community tini
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
