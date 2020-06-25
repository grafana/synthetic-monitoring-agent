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
pushed directly to Prometheus and Loki.

There are other related repositories:

* [The synthetic monitoring API](https://github.com/grafana/synthetic-monitoring-api).
* [The synthetic monitoring application](https://github.com/grafana/synthetic-monitoring-app).

Architecture
------------

Please see
[synthetic-monitoring-api](https://github.com/grafana/synthetic-monitoring-api)
for an architectural description.

![agent process][process]

[process]: https://www.plantuml.com/plantuml/svg/fLJBRjim4BppAnO-rmvwzI484XT1JWtQ_015hMp24fSbgSJv-omVaYrRwYM-M5pEx8n6Ipxu85tekrQ8MWPPIO-msZskXEMoLjfA4s3rGQwjhJRxjRHw1T83_yCIfcgbEbPqMdjTev8k4ShpbDFe5lsd3zWbJEEdssCZF5bo0NCdwwZ2AP1B7OO3zdv0bEKKrj8nkuyFGXHBiBvFhxC5HSQW2a3lwE3vp-lJBSIZgRC3qAOXrycWoGYfWdwN0SVNZ6WcxHwPur2HAopXCFJEb1OlEr7Z3VTMrU6_7dq0TK1r11ySocwG6C35MuRy59lDvhy8SwdIUDxyS9fDS0QDtbzkPgjRUFtzzmtkrdSEMvAroEM174i3T--ejrmX2vnGqJi9uDzCs-TVRy1Vosd5KyNsMjhxX1rpoS75KWbl5duHv9cGhP1HUNbbOHhkUMgur578NtZapOSvXrnKY6FtZTvScmbnyBm5s_l3aCqrC4aNo1XPd94htDb2q1rI7qHKJTDqEvQrzkN8BCxx9MQXopTEtP9eN4nyNIKxdZvXOi99kK1-vCiXnIAxD4iAlO-tHeKiZJ4GY3GX7lYHhyul
[PlantUML]: https://www.planttext.com/?text=fLJBRjim4BppAnO-rmvwzI484XT1JWtQ_015hMp24fSbgSJv-omVaYrRwYM-M5pEx8n6Ipxu85tekrQ8MWPPIO-msZskXEMoLjfA4s3rGQwjhJRxjRHw1T83_yCIfcgbEbPqMdjTev8k4ShpbDFe5lsd3zWbJEEdssCZF5bo0NCdwwZ2AP1B7OO3zdv0bEKKrj8nkuyFGXHBiBvFhxC5HSQW2a3lwE3vp-lJBSIZgRC3qAOXrycWoGYfWdwN0SVNZ6WcxHwPur2HAopXCFJEb1OlEr7Z3VTMrU6_7dq0TK1r11ySocwG6C35MuRy59lDvhy8SwdIUDxyS9fDS0QDtbzkPgjRUFtzzmtkrdSEMvAroEM174i3T--ejrmX2vnGqJi9uDzCs-TVRy1Vosd5KyNsMjhxX1rpoS75KWbl5duHv9cGhP1HUNbbOHhkUMgur578NtZapOSvXrnKY6FtZTvScmbnyBm5s_l3aCqrC4aNo1XPd94htDb2q1rI7qHKJTDqEvQrzkN8BCxx9MQXopTEtP9eN4nyNIKxdZvXOi99kK1-vCiXnIAxD4iAlO-tHeKiZJ4GY3GX7lYHhyul
