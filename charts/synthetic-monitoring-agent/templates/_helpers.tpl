{{/*
Chart name.
*/}}
{{- define "sm-agent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Release full name, used as the base of every resource name.
*/}}
{{- define "sm-agent.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "sm-agent.headlessName" -}}
{{- printf "%s-headless" (include "sm-agent.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "sm-agent.crocochromeName" -}}
{{- printf "%s-crocochrome" (include "sm-agent.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "sm-agent.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels. Pass a dict with the root context as "ctx" and the component name as "component". An optional "version"
replaces the chart's appVersion, for components that don't run the agent image.
*/}}
{{- define "sm-agent.labels" -}}
helm.sh/chart: {{ include "sm-agent.chart" .ctx }}
{{ include "sm-agent.selectorLabels" . }}
app.kubernetes.io/version: {{ default .ctx.Chart.AppVersion .version | trunc 63 | trimSuffix "-" | quote }}
app.kubernetes.io/managed-by: {{ .ctx.Release.Service }}
{{- end }}

{{- define "sm-agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sm-agent.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "sm-agent.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "sm-agent.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "sm-agent.crocochromeServiceAccountName" -}}
{{- if .Values.browserPool.crocochrome.serviceAccount.create }}
{{- default (include "sm-agent.crocochromeName" .) .Values.browserPool.crocochrome.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.browserPool.crocochrome.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "sm-agent.secretName" -}}
{{- default (include "sm-agent.fullname" .) .Values.probe.existingSecret }}
{{- end }}

{{- define "sm-agent.secretKey" -}}
{{- if .Values.probe.existingSecret }}
{{- .Values.probe.existingSecretKey }}
{{- else }}
{{- "api-token" }}
{{- end }}
{{- end }}

{{- define "sm-agent.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- if eq .Values.image.variant "browser" }}
{{- $tag = printf "%s-browser" $tag }}
{{- end }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{- define "sm-agent.crocochromeImage" -}}
{{- $tag := .Values.browserPool.crocochrome.image.tag }}
{{- if .Values.browserPool.crocochrome.unprivileged }}
{{- $tag = printf "%s-unprivileged" $tag }}
{{- end }}
{{- printf "%s:%s" .Values.browserPool.crocochrome.image.repository $tag }}
{{- end }}

{{/*
Security contexts. Defaults can be overridden by the extra* values, required fields can't: the images depend on them.
*/}}
{{- define "sm-agent.podSecurityContext" -}}
{{- $defaults := dict "fsGroup" 12345 }}
{{- toYaml (mergeOverwrite $defaults (deepCopy .Values.extraPodSecurityContext)) }}
{{- end }}

{{- define "sm-agent.securityContext" -}}
{{- $defaults := dict "seccompProfile" (dict "type" "RuntimeDefault") }}
{{- $required := dict
  "runAsNonRoot" true
  "runAsUser" 12345
  "readOnlyRootFilesystem" true
  "allowPrivilegeEscalation" false
  "capabilities" (dict "add" (list "NET_RAW") "drop" (list "ALL"))
}}
{{- toYaml (mergeOverwrite $defaults (deepCopy .Values.extraSecurityContext) $required) }}
{{- end }}

{{- define "sm-agent.crocochromePodSecurityContext" -}}
{{- with .Values.browserPool.crocochrome.extraPodSecurityContext }}
{{- toYaml . }}
{{- end }}
{{- end }}

{{- define "sm-agent.crocochromeSecurityContext" -}}
{{- $defaults := dict "seccompProfile" (dict "type" "RuntimeDefault") }}
{{- $required := dict }}
{{- if .Values.browserPool.crocochrome.unprivileged }}
{{- $required = dict
  "runAsNonRoot" true
  "readOnlyRootFilesystem" true
  "allowPrivilegeEscalation" false
  "capabilities" (dict "drop" (list "ALL"))
}}
{{- else }}
{{- $required = dict
  "runAsNonRoot" true
  "runAsUser" 6666
  "runAsGroup" 6666
  "readOnlyRootFilesystem" true
  "allowPrivilegeEscalation" true
  "capabilities" (dict "add" (list "SETUID" "SETGID" "KILL" "CHOWN" "DAC_OVERRIDE" "FOWNER") "drop" (list "ALL"))
}}
{{- end }}
{{- $securityContext := mergeOverwrite $defaults (deepCopy .Values.browserPool.crocochrome.extraSecurityContext) $required }}
{{- /* Replaced, not merged: the unprivileged image must not get capabilities added from extraSecurityContext. */}}
{{- $_ := set $securityContext "capabilities" $required.capabilities }}
{{- toYaml $securityContext }}
{{- end }}

{{- define "sm-agent.clusterName" -}}
{{- default (include "sm-agent.fullname" .) .Values.cluster.name }}
{{- end }}

{{- define "sm-agent.browserPoolEnabled" -}}
{{- if .Values.browserPool.enabled }}true{{ end }}
{{- end }}

{{- define "sm-agent.crocochromeEnabled" -}}
{{- if and .Values.browserPool.enabled .Values.browserPool.crocochrome.enabled }}true{{ end }}
{{- end }}

{{/*
Agent container args. Every chart value that maps to an agent flag is translated here.
*/}}
{{- define "sm-agent.args" -}}
- --api-server-address={{ .Values.probe.apiServerAddress }}
- --listen-address=0.0.0.0:{{ .Values.listenPort }}
{{- if .Values.probe.apiInsecure }}
- --api-insecure
{{- end }}
{{- if .Values.logging.verbose }}
- --verbose
{{- end }}
{{- if .Values.logging.debug }}
- --debug
{{- end }}
{{- with .Values.features }}
- --features={{ join "," . }}
{{- end }}
{{- if .Values.k6.disabled }}
- --disable-k6
{{- end }}
{{- if not (kindIs "invalid" .Values.k6.blockedNets) }}
- --blocked-nets={{ .Values.k6.blockedNets }}
{{- end }}
{{- with .Values.k6.uri }}
- --k6-uri={{ . }}
{{- end }}
{{- with .Values.k6.repository }}
- --k6-repository={{ . }}
{{- end }}
{{- if not (kindIs "invalid" .Values.memory.autoMemLimit) }}
- --enable-auto-memlimit={{ .Values.memory.autoMemLimit }}
{{- end }}
{{- if not (kindIs "invalid" .Values.memory.memLimitRatio) }}
- --memlimit-ratio={{ .Values.memory.memLimitRatio }}
{{- end }}
{{- with .Values.cache.type }}
- --cache-type={{ . }}
{{- end }}
{{- if not (kindIs "invalid" .Values.cache.localCapacity) }}
- --cache-local-capacity={{ .Values.cache.localCapacity }}
{{- end }}
{{- with .Values.cache.localTTL }}
- --cache-local-ttl={{ . }}
{{- end }}
{{- with .Values.cache.memcachedServers }}
- --memcached-servers={{ join "," . }}
{{- end }}
{{- if not .Values.usageReports.enabled }}
- --disable-usage-reports
{{- end }}
{{- if .Values.telemetry.push }}
- --experimental-push-telemetry
{{- end }}
{{- if not (kindIs "invalid" .Values.telemetry.timeSpan) }}
- --telemetry-time-span={{ .Values.telemetry.timeSpan }}
{{- end }}
{{- with .Values.telemetry.metricsPushInterval }}
- --metrics-push-interval={{ . }}
{{- end }}
{{- if .Values.debugEndpoints.changeLogLevel }}
- --enable-change-log-level
{{- end }}
{{- if .Values.debugEndpoints.disconnect }}
- --enable-disconnect
{{- end }}
{{- if .Values.debugEndpoints.pprof }}
- --enable-pprof
{{- end }}
{{- if not (kindIs "invalid" .Values.debugEndpoints.blockProfileRate) }}
- --block-profile-rate={{ .Values.debugEndpoints.blockProfileRate }}
{{- end }}
{{- if not (kindIs "invalid" .Values.debugEndpoints.mutexProfileFraction) }}
- --mutex-profile-fraction={{ .Values.debugEndpoints.mutexProfileFraction }}
{{- end }}
{{- if .Values.cluster.enabled }}
- --cluster-enabled
- --cluster-name={{ include "sm-agent.clusterName" . }}
- --cluster-listen-port={{ .Values.cluster.listenPort }}
{{- $join := list (printf "%s.%s.svc.%s" (include "sm-agent.headlessName" .) .Release.Namespace .Values.clusterDomain) }}
- --cluster-join-addresses={{ join "," (concat $join .Values.cluster.extraJoinAddresses) }}
- --cluster-advertise-address={{ default "$(POD_IP)" .Values.cluster.advertiseAddress }}
{{- if .Values.cluster.waitForSize }}
- --cluster-wait-for-size={{ .Values.cluster.waitForSize }}
{{- end }}
{{- with .Values.cluster.waitTimeout }}
- --cluster-wait-timeout={{ . }}
{{- end }}
{{- with .Values.cluster.rejoinInterval }}
- --cluster-rejoin-interval={{ . }}
{{- end }}
{{- end }}
{{- if .Values.browserPool.enabled }}
- --browser-pool-enabled
{{- if .Values.browserPool.crocochrome.enabled }}
- --browser-pool-addresses={{ printf "%s.%s.svc.%s:8080" (include "sm-agent.crocochromeName" .) .Release.Namespace .Values.clusterDomain }}
{{- else }}
- --browser-pool-addresses={{ join "," .Values.browserPool.externalAddresses }}
{{- end }}
{{- end }}
{{- range .Values.extraArgs }}
- {{ . | quote }}
{{- end }}
{{- end }}

{{/*
Validate values that the agent would reject or silently ignore.
*/}}
{{- define "sm-agent.validate" -}}
{{- if not .Values.probe.apiServerAddress }}
{{- fail "probe.apiServerAddress is required, for example synthetic-monitoring-grpc-eu-west-3.grafana.net:443" }}
{{- end }}
{{- if not (contains ":" .Values.probe.apiServerAddress) }}
{{- fail "probe.apiServerAddress must include a port, for example synthetic-monitoring-grpc-eu-west-3.grafana.net:443" }}
{{- end }}
{{- if and (not .Values.probe.apiToken) (not .Values.probe.existingSecret) }}
{{- fail "set probe.apiToken or probe.existingSecret" }}
{{- end }}
{{- if not (has .Values.image.variant (list "standard" "browser")) }}
{{- fail "image.variant must be standard or browser" }}
{{- end }}
{{- range $value, $labels := dict "podLabels" .Values.podLabels "browserPool.crocochrome.podLabels" .Values.browserPool.crocochrome.podLabels }}
{{- range $key := list "app.kubernetes.io/name" "app.kubernetes.io/instance" "app.kubernetes.io/component" }}
{{- if hasKey $labels $key }}
{{- fail (printf "%s can't set %s: the chart uses it in selectors" $value $key) }}
{{- end }}
{{- end }}
{{- end }}
{{- if and (not .Values.cluster.enabled) (gt (int .Values.replicas) 1) }}
{{- fail "replicas above 1 require cluster.enabled: without clustering, every agent runs every check of the probe" }}
{{- end }}
{{- if .Values.autoscaling.enabled }}
{{- if not .Values.cluster.enabled }}
{{- fail "autoscaling.enabled requires cluster.enabled: without clustering, every agent runs every check of the probe" }}
{{- end }}
{{- if lt (int .Values.autoscaling.minReplicas) 2 }}
{{- fail "autoscaling.minReplicas must be at least 2" }}
{{- end }}
{{- if lt (int .Values.autoscaling.maxReplicas) (int .Values.autoscaling.minReplicas) }}
{{- fail "autoscaling.maxReplicas can't be less than autoscaling.minReplicas" }}
{{- end }}
{{- if and (not .Values.autoscaling.targetCPUUtilizationPercentage) (not .Values.autoscaling.targetMemoryUtilizationPercentage) }}
{{- fail "autoscaling requires targetCPUUtilizationPercentage or targetMemoryUtilizationPercentage" }}
{{- end }}
{{- end }}
{{- if .Values.cluster.enabled }}
{{- $size := dict "name" "replicas" "value" .Values.replicas }}
{{- if .Values.autoscaling.enabled }}
{{- $size = dict "name" "autoscaling.minReplicas" "value" .Values.autoscaling.minReplicas }}
{{- end }}
{{- if gt (int .Values.cluster.waitForSize) (int $size.value) }}
{{- fail (printf "cluster.waitForSize can't be greater than %s, or agents never run checks until cluster.waitTimeout" $size.name) }}
{{- end }}
{{- if le (int .Values.terminationGracePeriodSeconds) 5 }}
{{- fail "terminationGracePeriodSeconds must be above 5 with clustering, so agents can hand over their checks" }}
{{- end }}
{{- end }}
{{- if .Values.browserPool.enabled }}
{{- if .Values.k6.disabled }}
{{- fail "browserPool.enabled requires k6: set k6.disabled to false" }}
{{- end }}
{{- if and .Values.k6.uri (hasPrefix "http" (toString .Values.k6.uri)) }}
{{- fail "browserPool.enabled doesn't work with a remote k6 runner (k6.uri)" }}
{{- end }}
{{- if and (not .Values.browserPool.crocochrome.enabled) (empty .Values.browserPool.externalAddresses) }}
{{- fail "browserPool.enabled requires browserPool.crocochrome.enabled or browserPool.externalAddresses" }}
{{- end }}
{{- end }}
{{- end }}
