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

![sidecar process][process]

[process]: https://www.plantuml.com/plantuml/svg/fLJ1KeCm4BtdAtPwbZfUFJYErd4ywMX_88G5Cq8IDw6jV-y60gMbU94BPFUzVPkN3VS-I0fjKmjHo21pwH5McuULS1pMIZjf0gpsbkh2QLDbqkaLI0_yXYLCNalrbTj3vdM1Ib97ID-dd5BNw7zymAR3bFuqFHR2WxCKiA_aoEQuf5rQsaig4dHS2P7q8RlhUhy5POr15S3kaE3v_Urn3l5eYbuEjE5QZGpQ6Y5Lq3iPSDnJH3Dfyy0SGcKfiGHXQCca4YyRKU2CErSb_6xH1r2VgBx01qBP3183EFNMGsx4Uht-4s6cAgNdv9uqDy4UbBgQ9ljMR-7jzMCBrzarOMrb8VhmG8wXWLkHRHr7h8jk9xs307ukb7r-saA3WRJY4UBsQblv3ZlwqXiNGycyY_29aKV2T8_3mREcQ_tdvIhHjZFYqRwW6xHj4EoYw0oUdrIXVPgYnH6OOXQg_6CGbqJZugIliwaO1r8OonPsoWboDRaqdJUowJnMw7syKZbo7gCh4Bb2tFCd_pz-0G00
[PlantUML]: https://www.planttext.com/?text=RL8nZi8m4EpzYfLxyu0hJWYGQAuuNOieDhQ12ECrjX5vFXix1CAbydfcCZiQPvaondoxE24qNG9vwpF8RSG3UfI02OvrXdT-JJ7Rhj2wZE_a3vtRGZaU9hPtYiw4rXyLXYhXSrxGEDJdXZgLMcCrng8Uvlal3XJl68sjql76OkUipYtv17AqjLteWrTnYDHOOJ1ZWyc2tAoa41mDr4rzmsveOC5hzq8y-r1CzPelEEKS9d3jP8xfAtdYTODXTBDYB5tv3SROl3hyS_fa9svYQAU6hicsVx_hAVwo-6Jx8AM8b-FIUiE_nWC0
