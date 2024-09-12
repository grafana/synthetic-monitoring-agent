# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM debian:stable-slim@sha256:64bc71feaa7ec2ac758a6a3a37c0f0d6ebccf0a45e3f5af1f1d3b5d4cb316b29 AS build
RUN apt-get update && apt-get -y install ca-certificates

# Second stage copies the binaries, configuration and also the
# certificates from the first stage. This Dockerfile also installs
# chromium in the agent image.

ARG TARGETPLATFORM

FROM --platform=$TARGETPLATFORM alpine:3.20.3 as release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add tini
RUN apk --no-cache add chromium-swiftshader

COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY dist/${HOST_DIST}/k6 /usr/local/bin/sm-k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]
