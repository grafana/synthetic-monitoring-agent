# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM debian:stable-slim@sha256:64bc71feaa7ec2ac758a6a3a37c0f0d6ebccf0a45e3f5af1f1d3b5d4cb316b29 AS build
RUN apt-get update && apt-get -y install ca-certificates

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

ARG TARGETPLATFORM

FROM --platform=$TARGETPLATFORM debian:stable-slim@sha256:64bc71feaa7ec2ac758a6a3a37c0f0d6ebccf0a45e3f5af1f1d3b5d4cb316b29 as release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY dist/${HOST_DIST}/k6 /usr/local/bin/sm-k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]
