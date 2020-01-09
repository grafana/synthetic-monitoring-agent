Worldping blackbox exporter sidecar
===================================

Worldping is a black box monitoring solution that provides users with
insights into how their applications and services are behaving from an
external point of view. It complements the insights from within the
applications and services we already provide through our existing
Metrics and Logs services at the core of our Grafana Cloud platform.

Worldping is also a demonstration of how use-case specific applications
can be built on top of Grafana and Grafana Cloud. It is the successor to
the original [worldping
application](https://grafana.net/plugins/raintank-worldping-app). The
refreshed worldping product focuses on reducing complexity and taking
advantage of Grafana Cloud capabilities.

This is the blackbox exporter sidecar: its function is to obtain
configuration information from worldping-api and use it to create a
configuration for
[blackbox_exporter](https://github.com/prometheus/blackbox_exporter),
which does the actual work of executing checks. This program connects to
the metrics endpoint in blackbox exporter and converts that into metrics
and events that are pushed to worldping-forwarder (or directly to Cortex
and Loki).

There are other related repositories:

* [The worldping API](https://github.com/grafana/worldping-api).
* [The worldping application](https://github.com/grafana/worldping-app).
* [The worldping forwarder](https://github.com/grafana/worldping-forwarder).

Architecture
------------

Please see [worldping-api](https://github.com/grafana/worldping-api) for
an architectural description.
