# First stage copies the binaries, configuration and installs the
# certificates for the base agent.
ARG TARGETPLATFORM

FROM --platform=$TARGETPLATFORM alpine:3.20.3 as release
RUN apk --no-cache add ca-certificates

ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY dist/${HOST_DIST}/k6 /usr/local/bin/sm-k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# Second stage copies the setup from the base agent and
# additionally installs Chromium to support browser checks.
FROM alpine:3.20.3 as with-browser

RUN apk --no-cache add tini
RUN apk --no-cache add chromium-swiftshader

COPY --from=release /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release /usr/local/bin/sm-k6 /usr/local/bin/sm-k6
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
