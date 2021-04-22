[![Build Status](https://drone.grafana.net/api/badges/grafana/synthetic-monitoring-agent/status.svg)](https://drone.grafana.net/grafana/synthetic-monitoring-agent)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/grafana/synthetic-monitoring-agent.svg)](https://github.com/grafana/synthetic-monitoring-agent)
[![Go Report Card](https://goreportcard.com/badge/github.com/grafana/synthetic-monitoring-agent)](https://goreportcard.com/report/github.com/grafana/synthetic-monitoring-agent)
[![Go Reference](https://pkg.go.dev/badge/github.com/grafana/synthetic-monitoring-agent.svg)](https://pkg.go.dev/github.com/grafana/synthetic-monitoring-agent)
[![License](https://img.shields.io/github/license/grafana/synthetic-monitoring-agent)](https://opensource.org/licenses/Apache-2.0)

<img src="img/logo.svg" width="100" />

Synthetic Monitoring Agent
==========================
This is the 'worker' for Grafana's [Synthetic Monitoring application](https://github.com/grafana/synthetic-monitoring-app). The agent provides probe functionality and executes network [checks](https://github.com/grafana/synthetic-monitoring-app/blob/main/README.md#check-types) for monitoring remote targets. 

Please [install](https://grafana.com/grafana/plugins/grafana-synthetic-monitoring-app/installation) Synthetic Monitoring 
in your Grafana Cloud or local Grafana instance before setting up your own private probe. You may need to generate a [new API key](https://grafana.com/profile/api-keys) to initialize the app.


Probes
------
Probes run [checks](https://github.com/grafana/synthetic-monitoring-app/blob/main/README.md#check-types) from 
distributed locations around the world and send the resulting metrics and events directly to 
[Grafana Cloud](https://grafana.com/products/cloud/) Prometheus and Loki services. 

You can select 1 or more **public** probes to run checks from or [run your own **private** probes](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/)
from any environment you choose.


To run your own probe
---------------------
![Add Probe](img/screenshot-probes.png)
### Add a new probe in your Grafana instance
* Navigate to **Synthetic Monitoring -> Probes**.
* Click **New**
* Enter a **Probe Name**, **Latitude**, **Longitude**, and **Region**.
* Optionally enter up to 3 custom labels to identify your probe.
* Click **Save**
* Copy the "Probe Authentication Token" and save for installing the agent.

### Install the agent on Debian based systems

* Add package repo GPG key

`wget -q -O - https://packages-sm.grafana.com/gpg.key | sudo apt-key add -`

* Add Debian package repo

`sudo add-apt-repository "deb https://packages-sm.grafana.com/deb stable main"`

* Install synthetic-monitoring-agent package


`sudo apt-get install synthetic-monitoring-agent`

* Edit `/etc/synthetic-monitoring/synthetic-monitoring-agent.conf` and add the token retrieved from Grafana

```
# Enter API token retrieved from grafana.com here
API_TOKEN='YOUR TOKEN HERE'
```

* Restart the agent

`sudo service synthetic-monitoring-agent restart`

Once the service is running, you will be able to select your new probe exactly the same as any public probe. You will need to manually add the new probe to any previously created checks.

#### Deploy it using Docker
We publish a docker image on [docker hub](https://hub.docker.com/r/grafana/synthetic-monitoring-agent)

Steps:
- Get an Authentication Token for your private probe, [see here for the steps](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/private-probes/)

```bash
# pull image
docker pull grafana/synthetic-monitoring-agent:latest
# export configs
# replace YOUR_TOKEN_HERE with your Authentication Token
export API_TOKEN=YOUR_TOKEN_HERE
export API_SERVER="synthetic-monitoring-grpc.grafana.net:443"
# run
docker run grafana/synthetic-monitoring-agent --api-server-address=${API_SERVER} --api-token=${API_TOKEN} --verbose=true
```

Now you should have the agent reporting as private probe, and running checks (if you have created some) in the logs.

#### Deploy it using Kubernetes
See [examples/kubernetes](./examples/kubernetes) for the documentation and example yaml files
