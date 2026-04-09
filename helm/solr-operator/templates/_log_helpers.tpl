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
log file
*/}}

{{- define "solr-operator.logging.fileMountDir" -}}
/var/logs/operator
{{- end -}}

{{- define "solr-operator.logging.file" -}}
{{- if .Values.logging.file.enabled -}}
{{ include "solr-operator.logging.fileMountDir" . }}/{{ .Values.logging.file.path }}
{{- end -}}
{{- end -}}


{{/*
log file
*/}}
{{- define "solr-operator.logging.volumes" -}}
{{- if .Values.logging.file.enabled -}}
- {{ .Values.logging.file.volume | toYaml | nindent 2 }}
{{ include "solr-operator.logging.rotation.volume" . }}
{{- end -}}
{{- end -}}

{{/*
log volume mounts
*/}}
{{- define "solr-operator.logging.volumeMounts" -}}
{{- if .Values.logging.file.enabled -}}
- name: {{ .Values.logging.file.volume.name }}
  mountPath: {{ include "solr-operator.logging.fileMountDir" . }}
  readOnly: false
{{ include "solr-operator.logging.rotation.volumeMount" . }}
{{- end -}}
{{- end -}}

{{/*
log rotation
*/}}
{{- define "solr-operator.logging.rotation.configMapName" -}}
log-rotate-conf
{{- end -}}

{{- define "solr-operator.logging.rotation.volumeMount" -}}
{{- if .Values.logging.file.enabled | and .Values.logging.file.rotation.enabled -}}
- name: {{ include "solr-operator.logging.rotation.configMapName" . }}
  mountPath: /etc/logrotate.d/
  readOnly: true
{{- end -}}

{{- end -}}
{{- define "solr-operator.logging.rotation.volume" -}}
{{- if .Values.logging.file.enabled | and .Values.logging.file.rotation.enabled -}}
- name: {{ include "solr-operator.logging.rotation.configMapName" . }}
  configMap:
    name: {{ include "solr-operator.logging.rotation.configMapName" . }}
{{- end -}}
{{- end -}}
