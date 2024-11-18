# First stage obtains the list of certificates.
FROM alpine:3.20.3 AS base
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add ca-certificates-bundle
COPY dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY dist/${HOST_DIST}/k6 /usr/local/bin/sm-k6
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh

ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

FROM base AS browser

# Renovate updates the pinned packages below.
# The --repository arg is required for renovate to know which alpine repo it should look for updates in.
# To keep the renovate regex simple, only keep one package installation per line.
RUN apk --no-cache add --repository community tini=0.19.0-r3
RUN apk --no-cache add --repository community chromium-swiftshader=131.0.6778.69-r0

ENV K6_BROWSER_ARGS=no-sandbox,disable-dev-shm-usage

ENTRYPOINT ["tini", "--", "/usr/local/bin/synthetic-monitoring-agent"]

FROM base
# This empty stage effectively makes `base` the default target.
