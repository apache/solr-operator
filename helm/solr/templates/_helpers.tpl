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
Expand the name of the chart.
*/}}
{{- define "solr.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "solr.fullname" -}}
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

{{/*
Provides the name of the solrcloud object (the fullname without '-solrcloud' appended)
*/}}
{{- define "solr.fullname-no-suffix" -}}
{{ include "solr.fullname" . | trimSuffix "-solrcloud" }}
{{- end }}

{{/*
Provides the name of the solrcloud metrics exporter object
*/}}
{{- define "solr.metrics.fullname" -}}
{{ include "solr.fullname-no-suffix" . }}-solr-metrics
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "solr.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "solr.labels" -}}
helm.sh/chart: {{ include "solr.chart" . }}
{{ include "solr.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "solr.selectorLabels" -}}
app.kubernetes.io/name: {{ include "solr.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Common Metrics labels
*/}}
{{- define "solr.metrics.labels" -}}
helm.sh/chart: {{ include "solr.chart" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Abstract out the sdr.appname for the metrics
*/}}
{{- define "solr.metrics.sdr-appname" -}}
{{- default (printf "%s-metrics" (include "solr.sdr-appname" .)) .Values.metrics.sdrAppname }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "solr.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "solr.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Abstract out the Zookeeper Chroot
*/}}
{{- define "solr.zkChroot" -}}
/{{ printf "%s%s" (trimSuffix "/" .Values.zk.chroot) (ternary (printf "/%s/%s" .Release.Namespace (include "solr.fullname-no-suffix" .)) "" .Values.zk.uniqueChroot) | trimPrefix "/" }}
{{- end }}