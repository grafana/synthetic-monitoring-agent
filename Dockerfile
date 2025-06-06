# First stage obtains the list of certificates.
FROM --platform=$BUILDPLATFORM alpine:3.22.0@sha256:8a1f59ffb675680d47db6337b49d22281a139e9d709335b492be023728e11715 AS build
RUN apk --no-cache add ca-certificates-bundle

# setcapper stage handles adding file capabilities where needed
FROM alpine:3.22.0@sha256:8a1f59ffb675680d47db6337b49d22281a139e9d709335b492be023728e11715 AS setcapper
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN apk --no-cache add libcap

COPY --chown=sm:sm --chmod=0500 dist/${HOST_DIST}/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent

RUN setcap cap_net_raw=+ep /usr/local/bin/synthetic-monitoring-agent

# Base release copies the binaries, configuration and also the
# certificates from the first stage.
FROM alpine:3.22.0@sha256:8a1f59ffb675680d47db6337b49d22281a139e9d709335b492be023728e11715 AS release
ARG TARGETOS
ARG TARGETARCH
ARG HOST_DIST=$TARGETOS-$TARGETARCH

RUN adduser -D -u 12345 -g 12345 sm

ADD --chown=sm:sm --chmod=0500 https://github.com/grafana/xk6-sm/releases/download/v0.5.5/sm-k6-${TARGETOS}-${TARGETARCH} /usr/local/bin/sm-k6
COPY --chown=sm:sm --chmod=0500 --from=setcapper /usr/local/bin/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY --chown=sm:sm scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER sm
ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]

# Browser release copies the setup from the base agent and
# additionally installs Chromium to support browser checks.
FROM ghcr.io/grafana/chromium-swiftshader-alpine:133.0.6943.141-r0-3.21.3@sha256:e1c0904d4386549840c514e94e3c8c78e04d73230f127167b423353af1b5c240 AS with-browser
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
