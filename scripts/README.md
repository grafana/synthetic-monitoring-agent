### Local environment tools
Follow the steps below to work with a local Synthetic Monitoring kubernetes cluster, and add a development version of the SM agent in there.

First, install the `Synthetic Monitoring plugin` in a `Grafana` instance, and follow these instructions on how to add a `Probe`.
1. Go into the SM Plugin.
2. First go to `Config`. You will notice under `Backend Address` a URL. This is the Synthetic Monitoring API URL. Save that for later, we will call it `<sm api url>`.
4. After that, click`Probes`. Then `New`.
5. Configure a `Probe Name`, and a `Region` name (`amer` for example). Click `Save`.
6. Copy the `Probe Authentication Token`, and the `<sm api url>` (which if not present will default to `sm-api:4031`) in a config file with the following format:
```
API_TOKEN=<probe authentication code>
SM_API_URL=<sm api url> 
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

Once , the agent will be reachable in the following host endpoint: `http://sm-agent.k3d.localhost:9999`. If you find
a `Could not resolve host` error, follow the steps in the [troubleshooting](#troubleshooting) section.

### Troubleshooting

#### Curling cluster ingress URLS gives you `curl: (6) Could not resolve host`

There appears to be a DNS issue introduced by Big Sur where `*.localhost` top level domains do not resolve by default. Browsers will likely still resolve the URLs but curl and other CLI apps will not. Known fixes for this issue are below,

1. Add all services to your /etc/hosts file (really annoying because there are quite a few services if you install apps)
2. Setup a local DNS server to route `*.localhost` -> `127.0.0.1`
    - The commands below will setup the rule using dnsmasq (Adopted from this gist https://gist.github.com/ogrrd/5831371)
   ```shell
   brew install dnsmasq

   mkdir -pv $(brew --prefix)/etc/

   echo 'address=/.localhost/127.0.0.1' >> $(brew --prefix)/etc/dnsmasq.conf

   sudo brew services start dnsmasq

   sudo mkdir -v /etc/resolver

   sudo bash -c 'echo "nameserver 127.0.0.1" > /etc/resolver/localhost'

   scutil --dns #should now show the localhost resolver
   ```
    - Retry the curl commands for integrations-api/hosted-exporters and you should hopefully get results now
