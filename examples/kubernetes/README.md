# Kubernetes

Steps:
- Get an Authentication Token for your private probe, [see here for steps](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/)
- Get Probe API Server URL from [Private Probe docs](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/)
- Replace `YOUR_TOKEN_HERE` in [secret.yaml](secret.yaml) with your Authentication Token
- Replace `PROBE_API_SERVER_URL` in [deployment.yaml](deployment.yaml) with your API server URL
- Apply [namespace.yaml](namespace.yaml) to create the `synthetic-monitoring` namespace
- Apply [secret.yaml](secret.yaml) to create the `sm-agent-1` secret
- Apply [deployment.yaml](deployment.yaml) to deploy your agent

Here are these steps as commands:

```bash
kubectl apply -f namespace.yaml
kubectl apply -f secret.yaml
kubectl apply -f deployment.yaml
```

Now you should have the agent reporting as private probe.

### Production Deployment

If you are running it in production, you should pin the image to an specific version instead of `latest`,
see all tags on [docker hub](https://hub.docker.com/r/grafana/synthetic-monitoring-agent)

The process exposes Prometheus-style metrics on an HTTP server running on port 4050 (/metrics endpoint).
You can scrape and monitor your private probe in this way using either Prometheus or the [Grafana Cloud Agent](https://github.com/grafana/agent).

Checkout [Private Probe docs](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/) for more details.
