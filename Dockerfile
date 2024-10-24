# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.20.3 AS build
RUN apk --no-cache add ca-certificates-bundle

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

ARG TARGETPLATFORM

FROM --platform=$TARGETPLATFORM alpine:3.20.3 AS release
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
FROM --platform=$TARGETPLATFORM alpine:3.20.3 AS with-browser

# Renovate updates the pinned packages below.
# The --repository arg is required for renovate to know which alpine repo it should look for updates in.
# To keep the renovate regex simple, only keep one package installation per line.
RUN apk --no-cache add --repository community tini=0.19.0-r3 && \
  apk --no-cache add --repository community chromium-swiftshader=130.0.6723.58-r0

COPY --from=release /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release /usr/local/bin/sm-k6 /usr/local/bin/sm-k6
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
