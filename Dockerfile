# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS build
RUN apk --no-cache add ca-certificates-bundle

# setcapper stage handles adding file capabilities where needed
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS setcapper
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add libcap

COPY --chown=sm:sm --chmod=0500 dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent

RUN setcap cap_net_raw=+ep /usr/local/bin/synthetic-monitoring-agent

# Base release copies the binaries, configuration and also the
# certificates from the first stage.
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH
ARG K6_V1_VERSION=v1.1.15
ARG K6_V2_VERSION=v2.0.10

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
FROM ghcr.io/grafana/chromium-swiftshader-alpine:152.0.7977.82-r0-3.24.1@sha256:eea17bd211b829c26bbff240c47663577790bf353ddc63504e4a6f836376d02b AS with-browser
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
