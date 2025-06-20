{{/*
   * Licensed to the Apache Software Foundation (ASF) under one or more
   * contributor license agreements.  See the NOTICE file distributed with
   * this work for additional information regarding copyright ownership.
   * The ASF licenses this file to You under the Apache License, Version 2.0
   * (the "License"); you may not use this file except in compliance with
   * the License.  You may obtain a copy of the License at
   *
   *     http://www.apache.org/licenses/LICENSE-2.0
   *
   * Unless required by applicable law or agreed to in writing, software
   * distributed under the License is distributed on an "AS IS" BASIS,
   * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   * See the License for the specific language governing permissions and
   * limitations under the License.
   */}}

{{/* vim: set filetype=mustache: */}}

{{/*
The values within Pod Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.pod.filler" -}}
{{- if .Values.podOptions.labels -}}
labels:
  {{- toYaml .Values.podOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.annotations -}}
annotations:
  {{- toYaml .Values.podOptions.annotations | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.resources -}}
resources:
  {{- toYaml .Values.podOptions.resources | nindent 2 }}
{{ end }}
{{- if (include "solr.serviceAccountName.solr" .) -}}
serviceAccountName: {{ include "solr.serviceAccountName.solr" . }}
{{ end }}
{{- if .Values.podOptions.shareProcessNamespace -}}
shareProcessNamespace: {{ .Values.podOptions.shareProcessNamespace }}
{{ end }}
{{- if .Values.podOptions.enableServiceLinks -}}
enableServiceLinks: {{ .Values.podOptions.enableServiceLinks }}
{{ end }}
{{- if .Values.podOptions.priorityClassName -}}
priorityClassName: {{ .Values.podOptions.priorityClassName }}
{{ end }}
{{- if .Values.podOptions.envVars -}}
envVars:
  {{- toYaml .Values.podOptions.envVars | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.affinity -}}
affinity:
  {{- toYaml .Values.podOptions.affinity | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.tolerations -}}
tolerations:
  {{- toYaml .Values.podOptions.tolerations | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.nodeSelector -}}
nodeSelector:
  {{- toYaml .Values.podOptions.nodeSelector | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.podSecurityContext -}}
podSecurityContext:
  {{- toYaml .Values.podOptions.podSecurityContext | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.containerSecurityContext -}}
containerSecurityContext:
  {{- toYaml .Values.podOptions.containerSecurityContext | nindent 2 }}
{{ end }}
{{- if (or .Values.podOptions.imagePullSecrets .Values.global.imagePullSecrets) -}}
imagePullSecrets:
  {{- toYaml (append .Values.podOptions.imagePullSecrets .Values.global.imagePullSecrets) | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.volumes -}}
volumes:
  {{- toYaml .Values.podOptions.volumes | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.terminationGracePeriodSeconds -}}
terminationGracePeriodSeconds: {{ .Values.podOptions.terminationGracePeriodSeconds }}
{{ end }}
{{- if .Values.podOptions.livenessProbe -}}
livenessProbe:
  {{- toYaml .Values.podOptions.livenessProbe | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.readinessProbe -}}
readinessProbe:
  {{- toYaml .Values.podOptions.readinessProbe | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.startupProbe -}}
startupProbe:
  {{- toYaml .Values.podOptions.startupProbe | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.lifecycle -}}
lifecycle:
  {{- toYaml .Values.podOptions.lifecycle | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.sidecarContainers -}}
sidecarContainers:
  {{- toYaml .Values.podOptions.sidecarContainers | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.initContainers -}}
initContainers:
  {{- toYaml .Values.podOptions.initContainers | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.topologySpreadConstraints -}}
topologySpreadConstraints:
  {{- toYaml .Values.podOptions.topologySpreadConstraints | nindent 2 }}
{{ end }}
{{- if .Values.podOptions.defaultInitContainerResources -}}
defaultInitContainerResources:
  {{- toYaml .Values.podOptions.defaultInitContainerResources | nindent 2 }}
{{ end }}
{{- end -}}

{{/*
The values within StatefulSet Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.stateful-set.filler" }}
{{- if .Values.statefulSetOptions.labels -}}
labels:
  {{- toYaml .Values.statefulSetOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.statefulSetOptions.annotations -}}
annotations:
  {{- toYaml .Values.statefulSetOptions.annotations | nindent 2 }}
{{ end }}
{{- if .Values.statefulSetOptions.podManagementPolicy -}}
podManagementPolicy: {{ .Values.statefulSetOptions.podManagementPolicy }}
{{ end }}
{{- end -}}

{{/*
The values within Ingress Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.ingress.filler" -}}
{{- if .Values.ingressOptions.labels -}}
labels:
  {{- toYaml .Values.ingressOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.ingressOptions.annotations -}}
annotations:
  {{- toYaml .Values.ingressOptions.annotations | nindent 2 }}
{{ end }}
{{- if .Values.ingressOptions.ingressClassName -}}
ingressClassName: {{ .Values.ingressOptions.ingressClassName }}
{{ end }}
{{- end -}}

{{/*
The values within ConfigMap Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.config-map.filler" -}}
{{- if .Values.configMapOptions.labels -}}
labels:
  {{- toYaml .Values.configMapOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.configMapOptions.annotations -}}
annotations:
  {{- toYaml .Values.configMapOptions.annotations | nindent 2 }}
{{ end }}
{{- if .Values.configMapOptions.providedConfigMap -}}
providedConfigMap: {{ .Values.configMapOptions.providedConfigMap }}
{{ end }}
{{- end -}}

{{/*
The values within CommonService Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.common-service.filler" -}}
{{- if .Values.commonServiceOptions.labels -}}
labels:
  {{- toYaml .Values.commonServiceOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.commonServiceOptions.annotations -}}
annotations:
  {{- toYaml .Values.commonServiceOptions.annotations | nindent 2 }}
{{ end }}
{{- end -}}

{{/*
The values within HeadlessService Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.headless-service.filler" -}}
{{- if .Values.headlessServiceOptions.labels -}}
labels:
  {{- toYaml .Values.headlessServiceOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.headlessServiceOptions.annotations -}}
annotations:
  {{- toYaml .Values.headlessServiceOptions.annotations | nindent 2 }}
{{ end }}
{{- end -}}

{{/*
The values within NodeService Options for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.node-service.filler" -}}
{{- if .Values.nodeServiceOptions.labels -}}
labels:
  {{- toYaml .Values.nodeServiceOptions.labels | nindent 2 }}
{{ end }}
{{- if .Values.nodeServiceOptions.annotations -}}
annotations:
  {{- toYaml .Values.nodeServiceOptions.annotations | nindent 2 }}
{{ end }}
{{- end -}}

{{/*
Provides all customKubeOptions values for a SolrCloud
*/}}
{{- define "solr.custom-kube-options.filler" -}}
{{- with (include "solr.custom-kube-options.pod.filler" .) -}}
{{- if . -}}
podOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- with (include "solr.custom-kube-options.stateful-set.filler" .) -}}
{{- if . -}}
statefulSetOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- with (include "solr.custom-kube-options.common-service.filler" .) -}}
{{- if . -}}
commonServiceOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- with (include "solr.custom-kube-options.headless-service.filler" .) -}}
{{- if . -}}
headlessServiceOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- with (include "solr.custom-kube-options.node-service.filler" .) -}}
{{- if . -}}
nodeServiceOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- with (include "solr.custom-kube-options.config-map.filler" .) -}}
{{- if . -}}
configMapOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- with (include "solr.custom-kube-options.ingress.filler" .) -}}
{{- if . -}}
ingressOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- end -}}

{{/*
Provides the nodeServiceOptions for the SolrCloud, if any are given
*/}}
{{- define "solr.custom-kube-options" -}}
{{- with (include "solr.custom-kube-options.filler" .) -}}
{{- if . -}}
customSolrKubeOptions:
  {{- . | nindent 2 -}}
{{ end }}
{{ end }}
{{- end -}}
