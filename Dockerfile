# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM debian:stable-slim AS build
RUN apt-get update && apt-get -y install ca-certificates

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

ARG TARGETPLATFORM

FROM --platform=$TARGETPLATFORM debian:stable-slim as release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY dist/${HOST_DIST}/k6 /usr/local/bin/sm-k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# third stage with alpine base for better access to chromium
FROM alpine:3.18 as with-browser

RUN apk --no-cache add chromium-swiftshader

ENV SM_CHROME_BIN=/usr/bin/chromium-browser
ENV SM_CHROME_PATH=/usr/lib/chromium/

COPY --from=release /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --from=release /usr/local/bin/sm-k6 /usr/local/bin/sm-k6
COPY --from=release /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=release /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]
