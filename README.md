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

Architecture
------------

Please see [worldping-api](https://github.com/grafana/worldping-api) for
an architectural description.

![sidecar process][process]

[process]: https://www.plantuml.com/plantuml/svg/fLJBRjim4BppAnO-rmvwzI484XT1JWtQ_015hMp24fSbgSJv-omVaYrRwYM-M5pEx8n6Ipxu85tekrQ8MWPPIO-msZskXEMoLjfA4s3rGQwjhJRxjRHw1T83_yCIfcgbEbPqMdjTev8k4ShpbDFe5lsd3zWbJEEdssCZF5bo0NCdwwZ2AP1B7OO3zdv0bEKKrj8nkuyFGXHBiBvFhxC5HSQW2a3lwE3vp-lJBSIZgRC3qAOXrycWoGYfWdwN0SVNZ6WcxHwPur2HAopXCFJEb1OlEr7Z3VTMrU6_7dq0TK1r11ySocwG6C35MuRy59lDvhy8SwdIUDxyS9fDS0QDtbzkPgjRUFtzzmtkrdSEMvAroEM174i3T--ejrmX2vnGqJi9uDzCs-TVRy1Vosd5KyNsMjhxX1rpoS75KWbl5duHv9cGhP1HUNbbOHhkUMgur578NtZapOSvXrnKY6FtZTvScmbnyBm5s_l3aCqrC4aNo1XPd94htDb2q1rI7qHKJTDqEvQrzkN8BCxx9MQXopTEtP9eN4nyNIKxdZvXOi99kK1-vCiXnIAxD4iAlO-tHeKiZJ4GY3GX7lYHhyul
[PlantUML]: https://www.planttext.com/?text=fLJBRjim4BppAnO-rmvwzI484XT1JWtQ_015hMp24fSbgSJv-omVaYrRwYM-M5pEx8n6Ipxu85tekrQ8MWPPIO-msZskXEMoLjfA4s3rGQwjhJRxjRHw1T83_yCIfcgbEbPqMdjTev8k4ShpbDFe5lsd3zWbJEEdssCZF5bo0NCdwwZ2AP1B7OO3zdv0bEKKrj8nkuyFGXHBiBvFhxC5HSQW2a3lwE3vp-lJBSIZgRC3qAOXrycWoGYfWdwN0SVNZ6WcxHwPur2HAopXCFJEb1OlEr7Z3VTMrU6_7dq0TK1r11ySocwG6C35MuRy59lDvhy8SwdIUDxyS9fDS0QDtbzkPgjRUFtzzmtkrdSEMvAroEM174i3T--ejrmX2vnGqJi9uDzCs-TVRy1Vosd5KyNsMjhxX1rpoS75KWbl5duHv9cGhP1HUNbbOHhkUMgur578NtZapOSvXrnKY6FtZTvScmbnyBm5s_l3aCqrC4aNo1XPd94htDb2q1rI7qHKJTDqEvQrzkN8BCxx9MQXopTEtP9eN4nyNIKxdZvXOi99kK1-vCiXnIAxD4iAlO-tHeKiZJ4GY3GX7lYHhyul
