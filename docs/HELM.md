# Helm

The chart lives in `charts/uvoo-dbviz` and installs the `uvoo-sqviz`
application. Defaults are conservative: the app and ClickHouse are enabled, and
the Docker Compose-like dependencies are behind `demo.enabled`.

## Lint And Render

```sh
make helm-lint
helm template sqviz charts/uvoo-dbviz
```

## Demo Install

This mode is for Kubernetes testing, not production. It deploys PostgreSQL,
PostgREST, ClickHouse, Keycloak, and the OTel collector with local defaults.

```sh
helm upgrade --install sqviz charts/uvoo-dbviz \
  --namespace sqviz \
  --create-namespace \
  --set demo.enabled=true \
  --set config.allowInsecureDefaults=true \
  --set config.authDevMode=true \
  --set secret.secretsEncryptionKey=dev-local-test-secret-key
```

Port-forward the app:

```sh
kubectl -n sqviz port-forward svc/sqviz-uvoo-sqviz 8080:80
```

Open <http://localhost:8080>.

## Ingress

Enable app ingress when your cluster has an ingress controller:

```sh
helm upgrade --install sqviz charts/uvoo-dbviz \
  --namespace sqviz \
  --create-namespace \
  --set demo.enabled=true \
  --set config.allowInsecureDefaults=true \
  --set config.authDevMode=true \
  --set secret.secretsEncryptionKey=dev-local-test-secret-key \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.host=sqviz.local
```

For TLS:

```sh
--set ingress.tls.enabled=true \
--set ingress.tls.secretName=sqviz-tls
```

## Keycloak Ingress

Development auth is the simplest demo path. To test browser OIDC through the
bundled Keycloak, expose Keycloak separately and set the public hostname:

```sh
--set demo.keycloak.ingress.enabled=true \
--set demo.keycloak.ingress.host=keycloak.sqviz.local \
--set demo.keycloak.hostname=http://keycloak.sqviz.local \
--set config.keycloakClientId=uvoo-sqviz-web
```

The backend discovers Keycloak through the in-cluster service by default.

## External Services

For a real deployment, keep `demo.enabled=false` and point the app at managed or
separately operated services:

```sh
helm upgrade --install sqviz charts/uvoo-dbviz \
  --namespace sqviz \
  --create-namespace \
  --set image.tag=v0.1.0 \
  --set config.environment=production \
  --set config.requireProductionSafe=true \
  --set config.publicUrl=https://sqviz.example.com \
  --set config.postgrestUrl=https://postgrest.example.com \
  --set config.clickhouseUrl=https://clickhouse.example.com:8123 \
  --set config.keycloakIssuer=https://keycloak.example.com/realms/sqviz \
  --set config.keycloakClientId=uvoo-sqviz-web \
  --set secret.existingSecret=sqviz-secrets
```

When `secret.existingSecret` is set, provide these keys in that Kubernetes
Secret:

- `SQVIZ_CLICKHOUSE_PASSWORD`
- `SQVIZ_SECRET_CLICKHOUSE_DEFAULT`
- `SQVIZ_POSTGRES_PASSWORD`
- `SQVIZ_POSTGREST_JWT_SECRET`
- `SQVIZ_KEYCLOAK_ADMIN_PASSWORD`
- `SQVIZ_OIDC_KEYCLOAK_CLIENT_SECRET`
- `SQVIZ_ALERT_SMTP_PASSWORD`
- `SQVIZ_SECRETS_ENCRYPTION_KEY`
- `PGRST_DB_URI`, only needed when bundled demo PostgREST is enabled
