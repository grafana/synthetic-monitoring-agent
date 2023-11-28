# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM debian:stable-slim AS build
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apt-get update && apt-get -y install ca-certificates libcap2-bin

# Second stage copies the binaries, configuration and also the
# certificates from the first stage.

COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
RUN setcap 'cap_net_raw=ep' /usr/local/bin/synthetic-monitoring-agent

ARG TARGETPLATFORM

FROM --platform=$TARGETPLATFORM debian:stable-slim
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

COPY dist/${HOST_DIST}/k6 /usr/local/bin/k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]
