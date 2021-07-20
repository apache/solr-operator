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

{{/*
Create the name of the service account to use for ZK
*/}}
{{- define "solr.serviceAccountName.zk" -}}
{{- .Values.zk.provided.zookeeperPodPolicy.serviceAccountName | default (include "solr.serviceAccountName.global" .) -}}
{{- end -}}

{{/*
Print all ZK Pod Options.
This is tricky because we want to default the serviceAccountName if not specified, but want to keep it empty if nothing is provided.
*/}}
{{- define "solr.zk.zookeeperPodPolicy" -}}
{{- if (include "solr.serviceAccountName.zk" .) -}}
{{ merge .Values.zk.provided.zookeeperPodPolicy (dict "serviceAccountName" (include "solr.serviceAccountName.zk" .)) | toYaml }}
{{- else if .Values.zk.provided.zookeeperPodPolicy -}}
{{ toYaml .Values.zk.provided.zookeeperPodPolicy }}
{{- end -}}
{{- end -}}

{{/*
Print all ZK Pod Options
*/}}
{{- define "solr.zk.zookeeperPodPolicy.serviceAccountName.tmp" -}}
{{- if .Values.zk.provided.zookeeperPodPolicy.serviceAccountName -}}
serviceAccountName: {{ .Values.zk.provided.zookeeperPodPolicy.serviceAccountName }}
{{- else if (include "solr.serviceAccountName.zk" .) -}}
serviceAccountName: {{ include "solr.serviceAccountName.zk" . }}
{{- end -}}
{{- end -}}

{{/*
Print all ZK Pod Options
*/}}
{{- define "solr.zk.zookeeperPodPolicy.serviceAccountName" -}}
{{- if .Values.zk.provided.zookeeperPodPolicy.serviceAccountName -}}
{{- .Values.zk.provided.zookeeperPodPolicy.serviceAccountName -}}
{{- else if (include "solr.serviceAccountName.zk" .) -}}
{{- include "solr.serviceAccountName.zk" . -}}
{{- end -}}
{{- end -}}

{{/*
ZK ChRoot
*/}}
{{ define "solr.zk.chroot" }}
/{{ printf "%s%s" (trimSuffix "/" .Values.zk.chroot) (ternary (printf "/%s/%s" .Release.Namespace (include "solr.fullname" .)) "" .Values.zk.uniqueChroot) | trimPrefix "/" }}
{{ end }}
