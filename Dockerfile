FROM debian:stable-slim

RUN apt-get update && apt-get -y install ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY dist/synthetic-monitoring-agent /usr/local/bin/synthetic-monitoring-agent
COPY scripts/pre-stop.sh /usr/local/lib/synthetic-monitoring-agent/pre-stop.sh
ENTRYPOINT ["/usr/local/bin/synthetic-monitoring-agent"]
