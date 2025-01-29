# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099 AS build
RUN apk --no-cache add ca-certificates-bundle

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

FROM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099 AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

ADD --chmod=0555 https://github.com/grafana/xk6-sm/releases/download/v0.0.3-pre/sm-k6-${TARGETOS}-${TARGETARCH} /usr/local/bin/sm-k6
COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# Third stage copies the setup from the base agent and
# additionally installs Chromium to support browser checks.
FROM ghcr.io/grafana/chromium-swiftshader-alpine:132.0.6834.83-r0-3.21.2@sha256:af189c0ad456d43bbfd38f21398f9d457d57b0455b6fbf986235a9b49080df44 AS with-browser

RUN apk --no-cache add --repository community tini

COPY --from=release /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release /usr/local/bin/sm-k6 /usr/local/bin/sm-k6
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
