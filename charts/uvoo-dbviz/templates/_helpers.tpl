{{- define "uvoo-sqviz.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "uvoo-sqviz.fullname" -}}
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

{{- define "uvoo-sqviz.labels" -}}
app.kubernetes.io/name: {{ include "uvoo-sqviz.name" . }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "uvoo-sqviz.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "uvoo-sqviz.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-sqviz.clickhouseURL" -}}
{{- if .Values.config.clickhouseUrl -}}
{{- .Values.config.clickhouseUrl -}}
{{- else -}}
{{- printf "http://%s-clickhouse:8123" (include "uvoo-sqviz.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-sqviz.postgresName" -}}
{{- printf "%s-postgres" (include "uvoo-sqviz.fullname" .) -}}
{{- end -}}

{{- define "uvoo-sqviz.postgrestName" -}}
{{- printf "%s-postgrest" (include "uvoo-sqviz.fullname" .) -}}
{{- end -}}

{{- define "uvoo-sqviz.keycloakName" -}}
{{- printf "%s-keycloak" (include "uvoo-sqviz.fullname" .) -}}
{{- end -}}

{{- define "uvoo-sqviz.keycloakClientId" -}}
{{- default "uvoo-sqviz-web" .Values.config.keycloakClientId -}}
{{- end -}}

{{- define "uvoo-sqviz.otelCollectorName" -}}
{{- printf "%s-otel-collector" (include "uvoo-sqviz.fullname" .) -}}
{{- end -}}

{{- define "uvoo-sqviz.postgrestURL" -}}
{{- if .Values.config.postgrestUrl -}}
{{- .Values.config.postgrestUrl -}}
{{- else if and .Values.demo.enabled .Values.demo.postgrest.enabled -}}
{{- printf "http://%s:3000" (include "uvoo-sqviz.postgrestName" .) -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-sqviz.publicURL" -}}
{{- if .Values.config.publicUrl -}}
{{- .Values.config.publicUrl -}}
{{- else if .Values.ingress.enabled -}}
{{- printf "http%s://%s" (ternary "s" "" .Values.ingress.tls.enabled) .Values.ingress.host -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-sqviz.keycloakIssuer" -}}
{{- if .Values.config.keycloakIssuer -}}
{{- .Values.config.keycloakIssuer -}}
{{- else if and .Values.demo.enabled .Values.demo.keycloak.enabled .Values.demo.keycloak.hostname -}}
{{- printf "%s/realms/sqviz" .Values.demo.keycloak.hostname -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-sqviz.keycloakDiscoveryURL" -}}
{{- if .Values.config.keycloakDiscoveryUrl -}}
{{- .Values.config.keycloakDiscoveryUrl -}}
{{- else if and .Values.demo.enabled .Values.demo.keycloak.enabled -}}
{{- printf "http://%s:8080/realms/sqviz" (include "uvoo-sqviz.keycloakName" .) -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-sqviz.validateValues" -}}
{{- if not .Values.config.allowInsecureDefaults -}}
{{- if .Values.demo.enabled -}}
{{- fail "demo.enabled deploys insecure test dependencies; set demo.enabled=false for production or config.allowInsecureDefaults=true for a demo install" -}}
{{- end -}}
{{- if .Values.config.authDevMode -}}
{{- fail "config.authDevMode enables header-based development authentication; set config.authDevMode=false for production or config.allowInsecureDefaults=true for a demo install" -}}
{{- end -}}
{{- if and (or .Values.config.alertsEnabled .Values.config.alertLoadPersisted) (or (not .Values.config.alertWorkerKey) (eq .Values.config.alertWorkerKey "dev-alert-worker-key")) -}}
{{- fail "alerts require a non-default config.alertWorkerKey; set a strong value or config.allowInsecureDefaults=true for a demo install" -}}
{{- end -}}
{{- end -}}
{{- end -}}
