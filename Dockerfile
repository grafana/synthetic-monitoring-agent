# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099 AS build
RUN apk --no-cache add ca-certificates-bundle

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

FROM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099 AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add libcap
RUN adduser -D -u 12345 -g 12345 sm

ADD --chown=sm:sm --chmod=0500 https://github.com/grafana/xk6-sm/releases/download/v0.0.3-pre/sm-k6-${TARGETOS}-${TARGETARCH} /usr/local/bin/sm-k6
COPY --chown=sm:sm --chmod=0500 dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --chown=sm:sm scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

RUN setcap cap_net_raw=+ep /usr/local/bin/synthetic-monitoring-agent

RUN apk del -f libcap
USER sm
ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# Third stage copies the setup from the base agent and
# additionally installs Chromium to support browser checks.
FROM ghcr.io/grafana/chromium-swiftshader-alpine:131.0.6778.264-r0-3.21.2@sha256:c3394ca2a5d82eecba8b8bceff972ca3f0f925ac9dec6cb24be8b84811f4f73f AS with-browser
RUN apk --no-cache add --repository community tini
RUN adduser -D -u 12345 -g 12345 sm

COPY --from=release --chown=sm:sm /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release --chown=sm:sm /usr/local/bin/sm-k6 /usr/local/bin/sm-k6
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Removing any file with setuid bit set, such as /usr/lib/chromium/chrome-sandbox,
# which is used for chromium sandboxing.
RUN find / -type f -perm -4000 -delete

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

USER sm
ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
