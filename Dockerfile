# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.21.0@sha256:21dc6063fd678b478f57c0e13f47560d0ea4eeba26dfc947b2a4f81f686b9f45 AS build
RUN apk --no-cache add ca-certificates-bundle

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

FROM alpine:3.21.0@sha256:21dc6063fd678b478f57c0e13f47560d0ea4eeba26dfc947b2a4f81f686b9f45 AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY dist/${HOST_DIST}/k6 /usr/local/bin/sm-k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# Third stage copies the setup from the base agent and
# additionally installs Chromium to support browser checks.
FROM ghcr.io/grafana/chromium-swiftshader-alpine:131.0.6778.85-r0-3.20.3 AS with-browser

RUN apk --no-cache add --repository community tini

COPY --from=release /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release /usr/local/bin/sm-k6 /usr/local/bin/sm-k6
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
