FROM debian:stable-slim

RUN apt-get update && apt-get -y install ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY dist/worldping-blackbox-sidecar /usr/local/bin/worldping-blackbox-sidecar
ENTRYPOINT ["/usr/local/bin/worldping-blackbox-sidecar"]
