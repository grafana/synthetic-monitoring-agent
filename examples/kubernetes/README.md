# Kubernetes

Steps:
- Get Probe Authentication Token for your private probe, [see steps](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/)
- Replace `YOUR_TOKEN_HERE` in [secret.yaml](secret.yaml) with your Probe Authentication Token
- Apply [namespace.yaml](namespace.yaml) to create `synthetic-monitoring` namespace
- Apply [secret.yaml](secret.yaml) to create `sm-agent-1` secret
- Now we will apply [deployment.yaml](deployment.yaml) to deploy out agent

Here are these steps as commands:

```bash
kubectl apply -f namespace.yaml
kubectl apply -f secret.yaml
kubectl apply -f deployment.yaml
```

Now you should have agent reporting as private probe.

### Production Deployment

If you are running it in production, you should pin the image version instead of `latest`,
see all tags on [docker hub](https://hub.docker.com/r/grafana/synthetic-monitoring-agent)

We expose Prometheus style metrics (/metrics endpoint) on port 4050 of process(and pod),
you can scrape and monitor your private probe
