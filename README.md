Synthetic monitoring agent
==========================

Synthetic monitoring is a black box monitoring solution that provides
users with insights into how their applications and services are
behaving from an external point of view. It complements the insights
from within the applications and services we already provide through our
existing Metrics and Logs services at the core of our Grafana Cloud
platform.

Synthetic monitoring is also a demonstration of how use-case specific applications
can be built on top of Grafana and Grafana Cloud. It is the successor to
the original [worldping
application](https://grafana.net/plugins/raintank-worldping-app). The
refreshed synthetic monitoring product focuses on reducing complexity
and taking advantage of Grafana Cloud capabilities.

This is the agent: its function is to obtain configuration information
from synthetic-monitoring-api and use it to build a configuration for
[blackbox_exporter](https://github.com/prometheus/blackbox_exporter),
which is how checks are executed. The resulting metrics and events are
pushed directly to Grafana Cloud Prometheus and Loki services.

There are other related repositories:

* [The synthetic monitoring API](https://github.com/grafana/synthetic-monitoring-api).
* [The synthetic monitoring application](https://github.com/grafana/synthetic-monitoring-app).

Architecture
------------

Please see
[synthetic-monitoring-api](https://github.com/grafana/synthetic-monitoring-api)
for an architectural description.

![agent process][process]

[process]: https://www.planttext.com/api/plantuml/svg/dLHDRy8m3BtdLqGz3wHTEKo88KsxJVi78VLA14swn47ityzE0ZHLcIQua8zdl-Tdf-k0ocFiZqAq2jLE1P3DTjE8WOwDDeEoA9lmOt4Fj5_qpXfqtjXkeGRJI1Ka_Sl_m3kmc0DuLKUGY0xoRLxMruDtFL3A61BajgrXHtV8adWXHEAHYvUaS2MrinOqIdS2Bzy-FrwdW02svTmxaCP-ETyhDCuAfT6S54BHpLWAsMuemeDsliG83nYzBGdUjwA5IUIKpyDtX81Ixq4VP40Fgh-apz2YAGEUnNAv_EFUYiwxE53nRf0alnoVXQJVbJhRotQaMpY3ZgdCXBe8BatWir8M6UwDfkOHtz5r8TsDIXn5P1dEQaZRYhwk_DP8EkeTPkDlKJErpkBci_CKF9oNpchZHbfNSeXXVx6aXYNI0hZwn8shXHPgD3suY8BPKdkdCzAQKERsxk2DCRCv7XxyUuIygJHLJbuVWB3iQ69DWAVyBjc8e7fWe8OG-C7kW6Y1RP0S9CIQblHL-WK0
[PlantUML]: https://www.planttext.com/?text=dLHDRy8m3BtdLqGz3wHTEKo88KsxJVi78VLA14swn47ityzE0ZHLcIQua8zdl-Tdf-k0ocFiZqAq2jLE1P3DTjE8WOwDDeEoA9lmOt4Fj5_qpXfqtjXkeGRJI1Ka_Sl_m3kmc0DuLKUGY0xoRLxMruDtFL3A61BajgrXHtV8adWXHEAHYvUaS2MrinOqIdS2Bzy-FrwdW02svTmxaCP-ETyhDCuAfT6S54BHpLWAsMuemeDsliG83nYzBGdUjwA5IUIKpyDtX81Ixq4VP40Fgh-apz2YAGEUnNAv_EFUYiwxE53nRf0alnoVXQJVbJhRotQaMpY3ZgdCXBe8BatWir8M6UwDfkOHtz5r8TsDIXn5P1dEQaZRYhwk_DP8EkeTPkDlKJErpkBci_CKF9oNpchZHbfNSeXXVx6aXYNI0hZwn8shXHPgD3suY8BPKdkdCzAQKERsxk2DCRCv7XxyUuIygJHLJbuVWB3iQ69DWAVyBjc8e7fWe8OG-C7kW6Y1RP0S9CIQblHL-WK0
