# synthetic-monitoring-agent Helm chart

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.66.1](https://img.shields.io/badge/AppVersion-v0.66.1-informational?style=flat-square)

Deploys the [Grafana Synthetic Monitoring agent](https://github.com/grafana/synthetic-monitoring-agent) to run a
[private probe](https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/set-up/set-up-private-probes/) on
Kubernetes. Optionally:

- **Clustering** (experimental): several agents share the same probe and split its checks.
- **Browser pool** (experimental): browser checks run on a fleet of [crocochrome](https://github.com/grafana/crocochrome)
  instances instead of a Chromium inside the agent. The chart can deploy the fleet.

## Install

1. [Add a private probe](https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/set-up/set-up-private-probes/#add-a-new-probe-in-your-grafana-instance)
   in Grafana and copy its **Probe Authentication Token** and the
   [API server URL](https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/set-up/set-up-private-probes/#probe-api-server-url)
   for your region.

1. Store the token in a Secret:

   ```bash
   kubectl create namespace synthetic-monitoring
   kubectl create secret generic sm-agent -n synthetic-monitoring --from-literal=api-token=<TOKEN>
   ```

1. Install the chart:

   ```bash
   helm install sm-agent oci://ghcr.io/grafana/helm-charts/synthetic-monitoring-agent \
     --version 2.0.0 \
     -n synthetic-monitoring \
     --set probe.apiServerAddress=synthetic-monitoring-grpc-eu-west-3.grafana.net:443 \
     --set probe.existingSecret=sm-agent
   ```

   The chart is also in the `grafana` Helm repository:

   ```bash
   helm repo add grafana https://grafana.github.io/helm-charts
   helm install sm-agent grafana/synthetic-monitoring-agent --version 2.0.0 -n synthetic-monitoring ...
   ```

The probe appears as online on the **Synthetics > Probes** page once the agent connects.

### Versions

The chart version is independent of the agent version. The chart's `appVersion` is the agent release it was tested
with, and the default image tag. To run a different agent release, set `image.tag`.

## Run a cluster

Clustering requires:

- An agent release with the `-cluster-*` flags ([#2022](https://github.com/grafana/synthetic-monitoring-agent/pull/2022)).
  It isn't released yet. Older images exit on startup with an unknown flag error.
- Synthetic Monitoring API support for clusters: every agent in the ring must receive the probe's full set of checks.

```yaml
replicas: 3
cluster:
  enabled: true
  # Keep it below replicas, so agents still start when one Pod is unavailable.
  waitForSize: 2
```

The chart also creates a PodDisruptionBudget, so that node drains and cluster upgrades evict one agent at a time. Set
`podDisruptionBudget.enabled: false` to turn it off.

The chart sets the join addresses to the release's headless Service, the advertise address to the Pod IP, and the
cluster name to the release name. Every agent in the release uses the same probe token. `replicas` above 1 requires
`cluster.enabled`, because without clustering every agent runs every check of the probe.

### Cluster security

Cluster traffic is plaintext HTTP/2, and agents don't authenticate each other: `cluster.name` only prevents accidental
merges. To restrict cluster traffic, you can enable the chart's NetworkPolicy (`networkPolicy.enabled`, off by
default). It only allows cluster traffic from the agents of this release, and needs a CNI that enforces NetworkPolicies.

The same policy allows `listenPort`, which serves `/metrics` and the debug endpoints, from any source. To limit it to
your Prometheus, set `networkPolicy.extraIngressFrom`.

### Autoscale the cluster

The chart can add a HorizontalPodAutoscaler for the agents. It requires `cluster.enabled`, and it's off by default:

```yaml
cluster:
  enabled: true
  # At most autoscaling.minReplicas.
  waitForSize: 1
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 70
```

The HPA owns the number of agents, so the chart ignores `replicas`. It scales on whichever target needs more agents.
Set a target to `null` to remove it.

Agent load comes from the probe's checks, not from traffic, so an autoscaler mostly helps probes whose number of checks
grows over time. Keep in mind:

- **Every scale event moves checks.** When agents join or leave, the cluster moves some checks to other agents, which
  run them again straight away. The chart scales down slowly by default: after 10 minutes of lower usage, and one agent
  every 5 minutes at most. Set `autoscaling.behavior` to change it.
- **Size the minimum for peak load.** Checks with long frequencies run in bursts that are over before the HPA reacts.
- **Targets are a percentage of requests,** not limits. Set `resources.requests` to what an agent really uses, or the
  targets don't mean much.

#### Browser checks and autoscaling

Without a browser pool, every browser check starts a Chromium inside the agent that owns it. Chromium uses far more CPU
and memory than any other check type, so browser checks dominate an agent's resource use and make it hard to autoscale
well:

- **Uneven load.** The cluster balances the number of checks across agents, not their cost. An agent that owns several
  browser checks can use much more CPU and memory than the others.
- **Out-of-memory kills the HPA doesn't prevent.** The HPA scales on the average across agents. A busy agent can reach
  its memory limit and be killed while the average stays below the target. When it restarts, its checks move to other
  agents and run again.
- **Spiky metrics.** Chromium only runs for the length of a check, so CPU and memory jump during browser checks and drop
  right after. The HPA can scale up after a burst has ended, and the slow scale-down keeps the extra agents for a while.
- **Larger requests and limits.** Every agent has to be sized for the browser checks it might own. All agents cost more,
  and targets measured against those requests mean less.

If the probe runs more than a few browser checks, use the [browser pool](#use-a-browser-pool). Chromium then runs on
crocochrome, and the agents only run the browser check scripts. Agent load becomes even and predictable, the HPA's CPU
and memory targets reflect the checks the agents run, and agents can use smaller requests and limits and the standard
image. The crocochrome fleet scales separately; refer to [Autoscale the fleet](#autoscale-the-fleet).

If you run only a few browser checks without the pool, size the memory limit for the heaviest agent, not the average,
and expect some scale events around bursts of browser checks.

## Use a browser pool

The browser pool requires an agent release with the `-browser-pool-*` flags
([#2027](https://github.com/grafana/synthetic-monitoring-agent/pull/2027)), and crocochrome v0.13.0 or later. The agent
flags aren't released yet. Older agent images exit on startup with an unknown flag error.

```yaml
browserPool:
  enabled: true
  crocochrome:
    # One browser session per instance. Run at least as many as the peak number of concurrent browser checks.
    replicas: 3
```

The chart deploys crocochrome as a Deployment with a headless Service, and points the agents to it. The agents can use
the standard image (`image.variant: standard`).

The default instances can all be scheduled on the same node. To spread them across nodes:

```yaml
browserPool:
  crocochrome:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: synthetic-monitoring-agent
            app.kubernetes.io/instance: <release name>
            app.kubernetes.io/component: crocochrome
```

### Security

crocochrome has two images, and `browserPool.crocochrome.unprivileged` chooses between them. The chart sets the image
and the security context it requires:

| | Hardened (default) | Unprivileged (`browserPool.crocochrome.unprivileged: true`) |
|---|---|---|
| Image tag | `<tag>` | `<tag>-unprivileged` |
| Chromium runs as | A separate user (`nobody`) | The same user as crocochrome |
| Security context | User 6666, `allowPrivilegeEscalation: true`, and the `SETUID`, `SETGID`, `KILL`, `CHOWN`, `DAC_OVERRIDE`, and `FOWNER` capabilities | The image's non-root user (6666), or the UID the platform assigns, `allowPrivilegeEscalation: false`, no capabilities |
| Pod Security profile | `baseline` | `restricted` |

The hardened image keeps Chromium apart from crocochrome through the user boundary. In the unprivileged image, a
compromised Chromium, for example through a malicious page, can kill or trace crocochrome, which restarts the container,
and leave files in `/chromium-tmp` for later sessions on the same instance. Use the unprivileged image only when you
trust both the check scripts and the content of the pages they load, including third-party scripts. Otherwise, keep the
hardened image. Refer to the
[crocochrome documentation](https://github.com/grafana/crocochrome/blob/main/doc/capabilities.md).

```yaml
browserPool:
  crocochrome:
    unprivileged: true
```

The chart doesn't set a user for the unprivileged image, so it also runs with the UID that platforms such as the
OpenShift `restricted-v2` SecurityContextConstraints assign.

The crocochrome API doesn't have authentication. The chart deploys a NetworkPolicy that only allows the agents to reach
it (`browserPool.crocochrome.networkPolicy.enabled`). It applies to any scraper, whether it uses the ServiceMonitor or
discovers the Pods on its own. To scrape crocochrome metrics, which use the same port, add your scraper's Pods to
`browserPool.crocochrome.networkPolicy.extraIngressFrom`, for example:

```yaml
browserPool:
  crocochrome:
    networkPolicy:
      extraIngressFrom:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
```

crocochrome runs with its own ServiceAccount (`browserPool.crocochrome.serviceAccount`), without a token, so that
identities given to the agent's ServiceAccount don't reach the browsers.

### Autoscale the fleet

The chart doesn't include an autoscaler for crocochrome. `avg(sm_crocochrome_session_active)` is the share of the
fleet in use, and you can autoscale on it with your own HorizontalPodAutoscaler or with [KEDA](https://keda.sh/). Set
`browserPool.crocochrome.replicas: null`, so that `helm upgrade` doesn't reset the number of instances the autoscaler
chose, and add the autoscaler with `extraObjects`. For example, with KEDA:

```yaml
browserPool:
  crocochrome:
    replicas: null
extraObjects:
  - apiVersion: keda.sh/v1alpha1
    kind: ScaledObject
    metadata:
      name: '{{ include "sm-agent.crocochromeName" . }}'
    spec:
      scaleTargetRef:
        name: '{{ include "sm-agent.crocochromeName" . }}'
      minReplicaCount: 3
      maxReplicaCount: 20
      triggers:
        - type: prometheus
          metadata:
            serverAddress: http://prometheus.monitoring.svc:9090
            query: avg(sm_crocochrome_session_active{namespace="{{ .Release.Namespace }}"})
            threshold: "0.7"
```

Size the minimum for bursts: browser checks with long frequencies tend to start at the same time, and autoscalers react
slowly.

## Security contexts

The chart sets the security contexts that the agent and crocochrome images require, so they aren't values:

- **Agent:** runs as user 12345 with a read-only root filesystem, no privilege escalation, and only the `NET_RAW`
  capability, which ping and traceroute checks need. `baseline` doesn't allow `NET_RAW`, so it needs the Pod Security
  `privileged` profile. The `seccompProfile` defaults to `RuntimeDefault`, and the Pod `fsGroup` to 12345.
- **crocochrome:** depends on the image, refer to [Security](#security). The `seccompProfile` defaults to
  `RuntimeDefault`.

To add fields, such as `seLinuxOptions` or a different `seccompProfile`, use `extraPodSecurityContext` and
`extraSecurityContext`, or `browserPool.crocochrome.extraPodSecurityContext` and
`browserPool.crocochrome.extraSecurityContext`. They can't change the fields the images require: the user, the root
filesystem, privilege escalation, and capabilities. The unprivileged crocochrome image doesn't require a user, so
`browserPool.crocochrome.extraSecurityContext` can set one.

## Feature flags

`features` accepts the agent's optional feature flags.
The chart passes them as they are: the agent ignores unknown names without a warning. `k6` and `traceroute` are
always enabled.

## Values

Values set to `null` use the agent's own default, and the chart doesn't pass the flag. Most descriptions name the agent
flag a value sets.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Agent affinity. |
| autoscaling | object | `{"behavior":{},"enabled":false,"maxReplicas":5,"minReplicas":2,"targetCPUUtilizationPercentage":70,"targetMemoryUtilizationPercentage":70}` | HorizontalPodAutoscaler for the agents. Requires cluster.enabled. The HPA owns the number of agents, and replicas is ignored. Every scale event moves checks between agents, and moved checks run again straight away. Refer to the README before enabling it, especially if the probe runs browser checks without the browser pool. |
| autoscaling.behavior | object | `{}` | HPA behavior. When empty, the chart scales down slowly: after 10 minutes of lower usage, and one agent every 5 minutes at most. Scale-up uses the Kubernetes default. |
| autoscaling.enabled | bool | `false` | Create the HorizontalPodAutoscaler. |
| autoscaling.maxReplicas | int | `5` | Maximum number of agents. |
| autoscaling.minReplicas | int | `2` | Size the minimum for peak load: the HPA reacts too slowly to absorb bursts of checks. |
| autoscaling.targetCPUUtilizationPercentage | int | `70` | Utilization targets, as a percentage of resources.requests. The HPA uses whichever needs more agents. Set a target to null to remove it. |
| browserPool | object | See the browserPool.* values. | Run browser checks on a crocochrome browser pool instead of a local Chromium (experimental). Refer to https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/set-up/set-up-private-probes/#run-browser-checks-on-a-browser-pool |
| browserPool.crocochrome | object | See the browserPool.crocochrome.* values. | crocochrome fleet deployed with the release. |
| browserPool.crocochrome.affinity | object | `{}` | crocochrome affinity. |
| browserPool.crocochrome.extraArgs | list | `[]` | Extra crocochrome arguments. |
| browserPool.crocochrome.extraPodSecurityContext | object | `{}` | The chart sets the security context the crocochrome image requires, depending on unprivileged. The seccompProfile defaults to RuntimeDefault. Use these to add fields, such as seLinuxOptions. They can't change the required fields: the root filesystem, privilege escalation, capabilities, and, for the hardened image, the user. |
| browserPool.crocochrome.image.pullPolicy | string | `"IfNotPresent"` | crocochrome image pull policy. |
| browserPool.crocochrome.image.tag | string | `"v0.13.0"` | crocochrome image tag. |
| browserPool.crocochrome.livenessProbe | object | `{}` | Liveness probe for crocochrome. Disabled when empty. |
| browserPool.crocochrome.networkPolicy | object | `{"egress":[],"enabled":true,"extraIngressFrom":[]}` | The crocochrome API doesn't have authentication. This NetworkPolicy only allows the agents to reach it. |
| browserPool.crocochrome.networkPolicy.egress | list | `[]` | Egress rules. When empty, the policy doesn't restrict egress. |
| browserPool.crocochrome.networkPolicy.enabled | bool | `true` | Create the crocochrome NetworkPolicy. |
| browserPool.crocochrome.networkPolicy.extraIngressFrom | list | `[]` | Additional ingress sources for port 8080, for example Prometheus. The same port serves the crocochrome API. |
| browserPool.crocochrome.nodeSelector | object | `{}` | crocochrome node selector. |
| browserPool.crocochrome.podAnnotations | object | `{}` | Annotations for the crocochrome Pods. |
| browserPool.crocochrome.podDisruptionBudget.enabled | bool | `false` | Create a PodDisruptionBudget for crocochrome. |
| browserPool.crocochrome.podDisruptionBudget.maxUnavailable | int | `1` | Maximum number of crocochrome instances evicted at a time. |
| browserPool.crocochrome.podLabels | object | `{}` | Can't set app.kubernetes.io/name, app.kubernetes.io/instance or app.kubernetes.io/component: the chart uses them in selectors. |
| browserPool.crocochrome.portName | string | `"http"` | Name of the crocochrome container port. The same port serves the session API and /metrics. |
| browserPool.crocochrome.priorityClassName | string | `""` | crocochrome priority class. |
| browserPool.crocochrome.readinessProbe | object | `{"periodSeconds":5,"tcpSocket":{"port":"{{ .Values.browserPool.crocochrome.portName }}"}}` | crocochrome has no health endpoint, so readiness checks that its port accepts connections. The default probe follows portName. |
| browserPool.crocochrome.replicas | int | `3` | Each instance runs one browser session at a time. Run at least as many as the peak number of browser checks running at the same time. Set to null when an autoscaler, such as KEDA, manages the number of instances, so that helm upgrade doesn't reset it. |
| browserPool.crocochrome.resources | object | `{"limits":{"cpu":"1000m","memory":"1000Mi"},"requests":{"cpu":"500m","memory":"1000Mi"}}` | crocochrome resource requests and limits. |
| browserPool.crocochrome.serviceAccount | object | `{"annotations":{},"create":true,"name":""}` | ServiceAccount for the crocochrome Pods, separate from the agent's, so that identities given to the agent, such as cloud IAM annotations, don't reach the browsers running check scripts. Its token is never mounted. |
| browserPool.crocochrome.serviceAccount.annotations | object | `{}` | Annotations for the crocochrome ServiceAccount. |
| browserPool.crocochrome.serviceAccount.create | bool | `true` | Create a ServiceAccount for crocochrome. |
| browserPool.crocochrome.serviceAccount.name | string | `""` | Defaults to the crocochrome full name when create is true, and to "default" otherwise. |
| browserPool.crocochrome.serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor for crocochrome. |
| browserPool.crocochrome.serviceMonitor.interval | string | `""` | Scrape interval. Defaults to the Prometheus one. |
| browserPool.crocochrome.serviceMonitor.labels | object | `{}` | Extra labels for the ServiceMonitor. |
| browserPool.crocochrome.serviceMonitor.namespace | string | `""` | Defaults to the release namespace. |
| browserPool.crocochrome.serviceMonitor.scrapeTimeout | string | `""` | Scrape timeout. Defaults to the Prometheus one. |
| browserPool.crocochrome.terminationGracePeriodSeconds | int | `320` | crocochrome waits for its active session to finish on shutdown, up to its 5 minute session timeout. |
| browserPool.crocochrome.tmpSizeLimit | string | `""` | Chromium writes temporary files to an in-memory volume, which counts toward the memory limit. |
| browserPool.crocochrome.tolerations | list | `[]` | crocochrome tolerations. |
| browserPool.crocochrome.topologySpreadConstraints | list | `[]` | crocochrome topology spread constraints. |
| browserPool.crocochrome.unprivileged | bool | `false` | Use the unprivileged crocochrome image (tag suffix -unprivileged) instead of the hardened one. The hardened image runs Chromium as a different user, so a compromised Chromium can't interfere with crocochrome. It needs allowPrivilegeEscalation and six capabilities, and meets Pod Security "baseline", not "restricted". The unprivileged image needs no capabilities or privilege escalation, and meets "restricted". Chromium then runs as the same user as crocochrome, so a compromised Chromium can kill or trace crocochrome and leave files for later sessions. Use it only if you trust the check scripts and the pages they load. Refer to https://github.com/grafana/crocochrome/blob/main/doc/capabilities.md |
| browserPool.enabled | bool | `false` | Pass the crocochrome headless Service to the agents (-browser-pool-enabled and -browser-pool-addresses) and deploy the fleet. |
| cache.localCapacity | string | `nil` | Maximum number of items in the local cache (-cache-local-capacity). |
| cache.localTTL | string | `nil` | Default TTL of local cache items, for example 5m (-cache-local-ttl). |
| cache.memcachedServers | list | `[]` | memcached servers, as host:port (-memcached-servers). |
| cache.type | string | `nil` | auto, memcached, local, or noop (-cache-type). Agent default: auto. |
| cluster | object | `{"advertiseAddress":"","enabled":false,"listenPort":7946,"listenPortName":"gossip","name":"","rejoinInterval":null,"waitForSize":0,"waitTimeout":null}` | Run several agents for the same probe as a cluster (experimental). Every agent uses the same token. Refer to https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/set-up/set-up-private-probes/#run-private-probes-in-a-cluster |
| cluster.advertiseAddress | string | `""` | Address other agents use to reach this one (-cluster-advertise-address). Defaults to the Pod IP. |
| cluster.enabled | bool | `false` | Run the agents as a cluster (-cluster-enabled). |
| cluster.listenPort | int | `7946` | Port for cluster traffic (-cluster-listen-port). |
| cluster.listenPortName | string | `"gossip"` | Name of the container port for cluster traffic. Must differ from listenPortName. |
| cluster.name | string | `""` | Cluster name (-cluster-name). Agents with a different name can't join. Defaults to the release full name. |
| cluster.rejoinInterval | string | `nil` | How often agents rejoin the peers they find (-cluster-rejoin-interval). Agent default: 60s. |
| cluster.waitForSize | int | `0` | Number of agents the cluster must reach before an agent runs checks (-cluster-wait-for-size). Keep it below replicas. 0 disables waiting. |
| cluster.waitTimeout | string | `nil` | Maximum time to wait for waitForSize (-cluster-wait-timeout). Agent default: 60s. |
| clusterDomain | string | `"cluster.local"` | Kubernetes cluster domain, used to build the DNS names of the headless Services. |
| debugEndpoints | object | `{"blockProfileRate":null,"changeLogLevel":false,"disconnect":false,"mutexProfileFraction":null,"pprof":false}` | Debug endpoints served on listenPort. Only enable them on trusted networks. |
| debugEndpoints.blockProfileRate | string | `nil` | Block profile rate (-block-profile-rate), used only with pprof. |
| debugEndpoints.changeLogLevel | bool | `false` | /logger, to change the log level at runtime (-enable-change-log-level). |
| debugEndpoints.disconnect | bool | `false` | /disconnect, to disconnect the agent from the API (-enable-disconnect). |
| debugEndpoints.mutexProfileFraction | string | `nil` | Mutex profile fraction (-mutex-profile-fraction), used only with pprof. |
| debugEndpoints.pprof | bool | `false` | /debug/pprof (-enable-pprof). |
| extraArgs | list | `[]` | Any other agent flag. |
| extraEnv | list | `[]` | Extra environment variables for the agent container. |
| extraObjects | list | `[]` | Extra manifests to deploy with the release. Strings are rendered with tpl. |
| extraPodSecurityContext | object | `{}` | The chart sets the security context the agent image requires: non-root user 12345, read-only root filesystem, no privilege escalation, and only the NET_RAW capability, needed for ping and traceroute checks. "baseline" doesn't allow NET_RAW, so it needs the Pod Security "privileged" profile. Use these to add fields, such as seLinuxOptions or a seccompProfile. They can't change the required fields. |
| extraSecurityContext | object | `{}` |  |
| extraVolumeMounts | list | `[]` | Extra volume mounts for the agent container. |
| extraVolumes | list | `[]` | Extra volumes for the agent Pods. |
| features | list | `[]` | Optional agent feature flags (-features). The agent ignores unknown names without a warning. k6 and traceroute are always enabled. |
| fullnameOverride | string | `""` | Overrides the full resource name prefix. |
| hostAliases | list | `[]` | Agent Pod host aliases. |
| image.pullPolicy | string | `"IfNotPresent"` | Agent image pull policy. |
| image.repository | string | `"grafana/synthetic-monitoring-agent"` | Agent image repository. |
| image.tag | string | `""` | Defaults to the chart appVersion. |
| image.variant | string | `"standard"` | standard, or browser to use the image with Chromium (tag suffix -browser). Not needed with the browser pool. |
| imagePullSecrets | list | `[]` | Secrets used to pull the images. |
| k6.blockedNets | string | `nil` | CIDR ranges that k6 checks can't reach (-blocked-nets). The agent default is 10.0.0.0/8. Set to "" to allow every range. |
| k6.disabled | bool | `false` | Disable k6 checks (scripted, multi-step and browser checks) on this probe (-disable-k6). |
| k6.repository | string | `nil` | Directory containing k6 binaries (-k6-repository). |
| k6.uri | string | `nil` | Path or URI of a k6 binary or a remote k6 runner (-k6-uri). Not compatible with the browser pool when it's an http(s) URI. |
| lifecycle | object | `{}` | Container lifecycle hooks, for example a preStop hook. |
| listenPort | int | `4050` | Port for the agent's HTTP server, which serves /metrics, /ready and the optional debug endpoints. The chart makes the agent listen on all interfaces so that probes and metrics scraping work. |
| listenPortName | string | `"http-metrics"` | Name of the container port for listenPort. The Service, the ServiceMonitor, the NetworkPolicy and the default probes use it. |
| livenessProbe | object | `{"httpGet":{"path":"/","port":"{{ .Values.listenPortName }}"}}` | Agent liveness probe. |
| logging.debug | bool | `false` | Enable debug logging (-debug), which also enables verbose logging. |
| logging.verbose | bool | `false` | Enable verbose logging (-verbose). |
| memory.autoMemLimit | string | `nil` | Set GOMEMLIMIT from the container memory limit (-enable-auto-memlimit). Agent default: true. |
| memory.memLimitRatio | string | `nil` | Fraction of the memory limit used for GOMEMLIMIT (-memlimit-ratio). Agent default: 0.9. |
| nameOverride | string | `""` | Overrides the chart name in resource names. |
| networkPolicy | object | `{"enabled":false,"extraIngressFrom":[]}` | Optional NetworkPolicy for the agent Pods. Cluster traffic is plaintext HTTP/2 without authentication: cluster.name only prevents accidental merges. The policy allows cluster traffic only from the agents of this release, and metrics scraping from extraIngressFrom. It needs a CNI that enforces NetworkPolicies. |
| networkPolicy.enabled | bool | `false` | Create the NetworkPolicy. |
| networkPolicy.extraIngressFrom | list | `[]` | Sources allowed to reach listenPort, which serves /metrics and the debug endpoints. Any source when empty. |
| nodeSelector | object | `{}` | Agent node selector. |
| podAnnotations | object | `{}` | Annotations for the agent Pods. |
| podDisruptionBudget.enabled | bool | `true` | Limits voluntary evictions, such as node drains, to maxUnavailable agents at a time. Only applies with cluster.enabled: a single agent can't be protected. |
| podDisruptionBudget.maxUnavailable | int | `1` | Maximum number of agents evicted at a time. |
| podLabels | object | `{}` | Can't set app.kubernetes.io/name, app.kubernetes.io/instance or app.kubernetes.io/component: the chart uses them in selectors. |
| priorityClassName | string | `""` | Agent priority class. |
| probe.apiInsecure | bool | `false` | Don't use TLS for the connection to the API server (-api-insecure). |
| probe.apiServerAddress | string | `""` | Synthetic Monitoring API server address, as host:port, for example synthetic-monitoring-grpc-eu-west-3.grafana.net:443. Refer to https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/set-up/set-up-private-probes/#probe-api-server-url Required. |
| probe.apiToken | string | `""` | Probe Authentication Token. The chart stores it in a Secret. Prefer existingSecret in production. |
| probe.existingSecret | string | `""` | Name of an existing Secret that holds the Probe Authentication Token. Takes precedence over apiToken. |
| probe.existingSecretKey | string | `"api-token"` | Key of the token in existingSecret. |
| readinessProbe | object | `{"httpGet":{"path":"/ready","port":"{{ .Values.listenPortName }}"}}` | Agent readiness probe. |
| replicas | int | `1` | Number of agents. With clustering disabled, keep it at 1: every agent runs every check of the probe. |
| resources | object | `{"limits":{"memory":"1Gi"},"requests":{"cpu":"200m","memory":"500Mi"}}` | Agent resource requests and limits. The agent sets GOMEMLIMIT from the memory limit (memory.autoMemLimit), and from the node's memory when there is no limit. |
| service.annotations | object | `{}` | Annotations for the Service. |
| service.type | string | `"ClusterIP"` | Service type for the agents. |
| serviceAccount.annotations | object | `{}` | Annotations for the ServiceAccount, for example cloud IAM roles. |
| serviceAccount.automountServiceAccountToken | bool | `false` | Mount the ServiceAccount token in the agent Pods. The agent doesn't use the Kubernetes API. |
| serviceAccount.create | bool | `true` | Create a ServiceAccount for the agents. |
| serviceAccount.name | string | `""` | ServiceAccount name. Defaults to the release full name when create is true, and to "default" otherwise. |
| serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor for the agents. |
| serviceMonitor.interval | string | `""` | Scrape interval. Defaults to the Prometheus one. |
| serviceMonitor.labels | object | `{}` | Extra labels for the ServiceMonitor. |
| serviceMonitor.metricRelabelings | list | `[]` | Relabeling rules applied to the scraped metrics. |
| serviceMonitor.namespace | string | `""` | ServiceMonitor namespace. Defaults to the release namespace. |
| serviceMonitor.relabelings | list | `[]` | Relabeling rules applied to the targets. |
| serviceMonitor.scrapeTimeout | string | `""` | Scrape timeout. Defaults to the Prometheus one. |
| telemetry.metricsPushInterval | string | `nil` | Interval between internal metrics pushes, for example 1m (-metrics-push-interval). |
| telemetry.push | bool | `false` | Push telemetry to the probe's tenant databases (-experimental-push-telemetry). |
| telemetry.timeSpan | string | `nil` | Minutes between telemetry pushes per tenant (-telemetry-time-span). |
| terminationGracePeriodSeconds | int | `30` | With clustering, keep it above 5 seconds so an agent can hand over its checks before it stops. |
| tolerations | list | `[]` | Agent tolerations. |
| topologySpreadConstraints | list | `[]` | Agent topology spread constraints. |
| usageReports.enabled | bool | `true` | Send anonymous usage reports. Set to false to pass -disable-usage-reports. |

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| grafana/synthetic-monitoring-be |  | <https://github.com/orgs/grafana/teams/synthetic-monitoring-be> |

## Source Code

* <https://github.com/grafana/synthetic-monitoring-agent>
* <https://github.com/grafana/crocochrome>

## Upgrading

### 2.0.0

Version 2.0.0 is a new chart. It replaces the `synthetic-monitoring-agent` 1.x charts: the deprecated one in
[grafana/helm-charts](https://github.com/grafana/helm-charts) (last version 1.2.1) and the one in
[grafana-community/helm-charts](https://github.com/grafana-community/helm-charts).

- **What's breaking:** most values are renamed, and the autoscaler requires clustering.
- **Who is affected:** every 1.x release.
- **How to proceed:** uninstall the 1.x release, then install 2.0.0 with the values mapped as below. The probe token
  doesn't change. The probe is offline between the two steps.

| 1.x chart | 2.0.0 |
| --- | --- |
| `agent.apiServerAddress` | `probe.apiServerAddress` |
| `agent.apiToken` | `probe.apiToken` |
| `secret.existingSecret` | `probe.existingSecret` |
| `agent.debug`, `agent.verbose` | `logging.debug`, `logging.verbose` |
| `agent.enableDisconnect` | `debugEndpoints.disconnect` |
| `replicaCount` | `replicas`, with `cluster.enabled` above 1 |
| `autoscaling.*` | `autoscaling.*`, now only with `cluster.enabled` and `minReplicas` of at least 2. Size a fixed cluster with `replicas` |
| `service.port` | `listenPort` |
| `securityContext`, `podSecurityContext` | Set by the chart. Add fields with `extraSecurityContext`, `extraPodSecurityContext` |
| `extraArgs`, `extraEnv`, `extraObjects` | Unchanged |

## Develop

Every change to the chart needs a new `version` in [Chart.yaml](Chart.yaml): a release is published when a new version
reaches `main`. Bump `appVersion` after testing the chart with a new agent release.

This README is generated by [helm-docs](https://github.com/norwoodj/helm-docs) from [README.md.gotmpl](README.md.gotmpl),
`Chart.yaml` and `values.yaml`: don't edit `README.md` directly. Describe each value in `values.yaml` with a `# --`
comment above it, edit the prose in the template, and run `make chart-docs`. CI fails if the README is stale.

The checks CI runs are also make targets. They need [helm](https://helm.sh), the
[helm-unittest](https://github.com/helm-unittest/helm-unittest) plugin, [kubeconform](https://github.com/yannh/kubeconform),
[yq](https://github.com/mikefarah/yq) and Docker:

```bash
make chart                # all of the below
make chart-lint           # helm lint and kubeconform, for every values file in ci/
make chart-test           # helm unittest
make chart-check-flags    # the appVersion agent image supports every flag the chart passes
make chart-check-flags AGENT_IMAGE=<image>   # check against another agent image, such as a local build
make chart-check-docs     # README.md is up to date
make chart-docs           # regenerate README.md
```
