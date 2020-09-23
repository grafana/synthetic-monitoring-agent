# Kubernetes

Steps:
- Get API key for your private probe, [see steps](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/)
- Replace `YOUR_TOKEN_HERE` in [secret.yaml](secret.yaml) with your Probe Key
- Apply [namespace.yaml](namespace.yaml) to create `synthetic-monitoring` namespace
- Apply [secret.yaml](secret.yaml) to create `sm-agent-1` secret
- Now we will apply [deployment.yaml](deployment.yaml) to deploy out agent

Here is what these steps will like as commands:

```bash
kubectl apply -f namespace.yaml
kubectl apply -f secret.yaml
kubectl apply -f deployment.yaml
```

Now you should have agent reporting as private probe.
