{{- define "uvoo-dbviz.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "uvoo-dbviz.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-dbviz.labels" -}}
app.kubernetes.io/name: {{ include "uvoo-dbviz.name" . }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "uvoo-dbviz.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "uvoo-dbviz.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-dbviz.clickhouseURL" -}}
{{- if .Values.config.clickhouseUrl -}}
{{- .Values.config.clickhouseUrl -}}
{{- else -}}
{{- printf "http://%s-clickhouse:8123" (include "uvoo-dbviz.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-dbviz.validateValues" -}}
{{- if not .Values.config.allowInsecureDefaults -}}
{{- if .Values.config.authDevMode -}}
{{- fail "config.authDevMode enables header-based development authentication; set config.authDevMode=false for production or config.allowInsecureDefaults=true for a demo install" -}}
{{- end -}}
{{- if and (or .Values.config.alertsEnabled .Values.config.alertLoadPersisted) (or (not .Values.config.alertWorkerKey) (eq .Values.config.alertWorkerKey "dev-alert-worker-key")) -}}
{{- fail "alerts require a non-default config.alertWorkerKey; set a strong value or config.allowInsecureDefaults=true for a demo install" -}}
{{- end -}}
{{- end -}}
{{- end -}}
