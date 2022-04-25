### Local environment tools
Follow the steps below to work with the local Synthetic Monitoring cluster, and add a development version of the SM agent in there.

First, setup a local cluster with [sm-dev](https://github.com/grafana/synthetic-monitoring-api/tree/main/scripts#sm-dev).
Once configured, go into the local Grafana instance and configure the SM Plugin. Then:
1. Go into the SM Plugin.
2. Click `Probes`. Then `New`.
3. Configure a `Probe Name`, and a `Region` name (`amer` for example). Click `Save`.
4. Copy the `Probe Authentication Token` in a config file with the following format:
```
API_TOKEN=<probe authentication code>
```
5. Save the file and let's call its path `<config file path>`.
6. Deploy and build the agent into the local cluster by running:
```bash
./scripts/sm-dev <config file path>
```

In the case you just want to deploy a k8s configuration change to the local cluster, run:
```bash
./scripts/sm-dev -n <config file path>
```
This will avoid re-building the SM agent Docker image.

Once depoyed, the agent will be reachable in the following host endpoint: `http://sm-agent.k3d.localhost:9999`. If you find
a `Could not resolve host` error, follow the steps [here](https://github.com/grafana/cloud-onboarding/tree/main/ops#curling-cluster-ingress-urls-gives-you-curl-6-could-not-resolve-host).
