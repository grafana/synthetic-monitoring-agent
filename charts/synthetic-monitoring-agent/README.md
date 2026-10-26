# synthetic-monitoring-agent Helm chart

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
it (`browserPool.crocochrome.networkPolicy.enabled`). To scrape crocochrome metrics, which use the same port, add your
Prometheus Pods to `browserPool.crocochrome.networkPolicy.extraIngressFrom`.

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

Values set to `null` use the agent's own default, and the chart doesn't pass the flag. Refer to
[values.yaml](values.yaml) for every value and its description.

| Value | Agent flag |
| --- | --- |
| `probe.apiServerAddress` | `-api-server-address` |
| `probe.apiToken`, `probe.existingSecret`, `probe.existingSecretKey` | `API_TOKEN` environment variable |
| `probe.apiInsecure` | `-api-insecure` |
| `logging.verbose`, `logging.debug` | `-verbose`, `-debug` |
| `features` | `-features` |
| `listenPort` | `-listen-address=0.0.0.0:<port>` |
| `k6.disabled`, `k6.blockedNets`, `k6.uri`, `k6.repository` | `-disable-k6`, `-blocked-nets`, `-k6-uri`, `-k6-repository` |
| `memory.autoMemLimit`, `memory.memLimitRatio` | `-enable-auto-memlimit`, `-memlimit-ratio` |
| `cache.type`, `cache.localCapacity`, `cache.localTTL`, `cache.memcachedServers` | `-cache-type`, `-cache-local-capacity`, `-cache-local-ttl`, `-memcached-servers` |
| `usageReports.enabled` | `-disable-usage-reports` when `false` |
| `telemetry.push`, `telemetry.timeSpan`, `telemetry.metricsPushInterval` | `-experimental-push-telemetry`, `-telemetry-time-span`, `-metrics-push-interval` |
| `debugEndpoints.*` | `-enable-change-log-level`, `-enable-disconnect`, `-enable-pprof`, `-block-profile-rate`, `-mutex-profile-fraction` |
| `cluster.enabled`, `cluster.name`, `cluster.listenPort` | `-cluster-enabled`, `-cluster-name`, `-cluster-listen-port` |
| `cluster.advertiseAddress` | `-cluster-advertise-address`, the Pod IP by default |
| `cluster.waitForSize`, `cluster.waitTimeout`, `cluster.rejoinInterval` | `-cluster-wait-for-size`, `-cluster-wait-timeout`, `-cluster-rejoin-interval` |
| `browserPool.enabled` | `-browser-pool-enabled`, and `-browser-pool-addresses` set to the crocochrome headless Service |
| `extraArgs` | Any other flag |

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

```bash
helm lint charts/synthetic-monitoring-agent -f charts/synthetic-monitoring-agent/ci/cluster-browser-pool-values.yaml
helm unittest charts/synthetic-monitoring-agent
```
