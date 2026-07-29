{{- define "kubetask.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "kubetask.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{- define "kubetask.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "kubetask.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "kubetask.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
{{- .Values.serviceAccount.name | default "default" }}
{{- end -}}
{{- end -}}
