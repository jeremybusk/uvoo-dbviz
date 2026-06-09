# uvoo-dbviz

Uvoo DBViz is a tenant-aware ClickHouse visualizer for observability data. It keeps
the control plane relational in PostgreSQL/PostgREST, keeps analytics in
ClickHouse, and keeps the API small: the Go backend validates OIDC tokens,
enforces tenant scope, and turns UI requests into constrained ClickHouse queries.

## Stack

- Frontend: React, Vite, Apache ECharts rendered to canvas
- API: Go, standard-library OIDC/JWKS validation, constrained ClickHouse query API
- State: PostgreSQL schema with RLS policies for tenants, users, dashboards, charts, alert rules, and contact endpoints
- State API: PostgREST-compatible schema and JWT tenant claim policies
- Telemetry store: ClickHouse over HTTP
- Deployment: Docker Compose, Helm, GHCR workflow, CI, and license checks

Google and Microsoft OIDC providers are present and enabled in the public config
by default. Add `DBVIZ_OIDC_GOOGLE_CLIENT_ID` and/or
`DBVIZ_OIDC_MICROSOFT_CLIENT_ID` to activate browser sign-in. Keycloak and other
private IdPs can be added with the Keycloak env vars or
`DBVIZ_OIDC_PROVIDERS_JSON`.

## Local Testing

```sh
cp .env.example .env
docker compose up -d --build
```

For browser testing from another machine on an isolated LAN, set the host name or
IP address clients will use before starting Compose:

```sh
DBVIZ_BIND_ADDR=0.0.0.0
DBVIZ_PUBLIC_HOST=192.168.1.50
```

Then open `http://192.168.1.50:8080`. Keycloak will advertise the same public
host in OIDC issuer and redirect metadata, while the Go backend still discovers
Keycloak through the internal Docker service URL.

Compose starts:

- `uvoo-dbviz` on <http://localhost:8080>
- ClickHouse on <http://localhost:8123> with seeded `dev` tenant data
- PostgreSQL on `localhost:5432`
- PostgREST on <http://localhost:3000>
- Keycloak on <http://localhost:8089> with realm `dbviz`
- OpenTelemetry Collector on `localhost:4317` and `localhost:4318`

The OpenTelemetry Collector exports received OTLP logs, traces, and metrics to
ClickHouse using collector-managed raw tables named `otelcol_*`. The current UI
datasets query the normalized demo tables `otel_logs`, `otel_traces`, and
`otel_metrics`; those are created by the ClickHouse migration and populated by
the sample telemetry script. Compose also includes an `otel-normalizer` helper
that creates ClickHouse materialized views from compatible collector raw tables
into the normalized UI tables. If the collector has not created raw tables yet,
rerun `docker compose run --rm otel-normalizer` after telemetry arrives.

Development auth is enabled by default in Compose. The seeded Keycloak users are:

- `alice` / `password`, tenant `dev`
- `bob` / `password`, tenant `example.com`

The UI can also use dev auth when `DBVIZ_AUTH_DEV_MODE=true`. Browser state
operations go through the Go API, which validates bearer tokens and forwards the
verified tenant/user context to PostgREST. PostgREST still enforces RLS with
that request context or `X-Dev-Tenant: dev` in local development.

In the default Compose profile, the Go API validates OIDC tokens and does not
forward the browser bearer token to PostgREST
(`DBVIZ_POSTGREST_FORWARD_BEARER=false`). This avoids PostgREST rejecting local
Keycloak tokens when it is not configured with Keycloak's signing keys. Enable
bearer forwarding only when PostgREST has a real JWT verifier configured.

On successful sign-in, the UI calls `POST /api/session/sync`. The backend records
or updates the current tenant/user in PostgreSQL, making later role and invite
rules explicit instead of keeping identity only in browser state.

Write operations for dashboards, saved queries, data sources, alert rules, and
contact endpoints are checked against the synced PostgreSQL role. `owner`,
`admin`, and `editor` can write; `viewer` can read. Tenant invite, member
deactivation, and audit event access is limited to `owner` and `admin`.

Users can belong to a tenant even when their public IdP does not emit that
tenant as a claim. Owners and admins create an invite, the invited user signs in
with the matching email address, and `POST /api/invites/accept` attaches that
identity to the invited tenant. The UI then sends `X-DBViz-Tenant` for the
selected active tenant; the Go API forwards it to PostgREST with the verified
subject/provider headers, and PostgreSQL only resolves the tenant when a
matching membership exists.

Admins can deactivate members without deleting historical ownership metadata.
Disabled users no longer satisfy tenant membership or role checks, and owner
changes/deactivation guard against removing the last active owner.

Run the OTel sample emitter after the stack is up:

```sh
docker compose run --rm otel-sample
```

It sends OTLP JSON to the collector and inserts matching normalized sample rows
into ClickHouse so the default UI datasets can chart the data immediately. It
also attempts to create the raw-to-normalized materialized views for future OTLP
traffic.

## Build And Verify

```sh
make test
make web
make build
make license-check
helm lint charts/uvoo-dbviz
docker compose config
make compose-smoke
```

Production safety checks are enabled by setting `DBVIZ_ENV=production` or
`DBVIZ_REQUIRE_PRODUCTION_SAFE=true`. The process then rejects development auth,
localhost service URLs, default alert worker keys, demo PostgREST JWT secrets,
and missing usable OIDC provider configuration unless
`DBVIZ_ALLOW_INSECURE_DEFAULTS=true` is explicitly set.

## Data Model

PostgreSQL migrations live in `migrations/postgres`. ClickHouse table setup and
sample observability rows live in `migrations/clickhouse` and
`scripts/seed-clickhouse.sh`.

The default query API supports `logs`, `traces`, and `metrics` datasets. Each
dataset has explicit table, time, tenant, filter, filter operator, dimension,
measure, aggregation, max-lookback, and max-row allow-lists so a user request
cannot bypass tenant scoping with arbitrary SQL.

Tenants can also keep ClickHouse connection metadata in `data_sources`. The
application stores non-secret connection fields and a `passwordSecretRef`; raw
passwords are rejected by the Go API and stripped by PostgreSQL RPCs. At query
time, a selected `sourceId` is loaded through tenant-scoped RLS and converted
into a ClickHouse client. The `passwordSecretRef` value maps to an environment
variable by uppercasing and replacing non-alphanumerics with `_`, prefixed with
`DBVIZ_SECRET_`; for example, `clickhouse-default` resolves from
`DBVIZ_SECRET_CLICKHOUSE_DEFAULT`.

## Dashboards

The frontend saves and opens dashboards through Go API endpoints backed by
PostgREST RPCs. Dashboard layouts are JSONB documents with a `version` and a
`charts` array. Each chart stores a title, visualization config, and a full
validated query payload, so dashboards can carry multiple reusable panels
without schema churn.

- `GET /api/dashboards`
- `POST /api/dashboards`
- `list_dashboards()`
- `save_dashboard(dashboard_id uuid, dashboard_name text, dashboard_layout jsonb)`

Saved queries use the same tenant-scoped control-plane path and validate query
payloads against configured datasets before persisting them:

- `GET /api/saved-queries`
- `POST /api/saved-queries`
- `list_saved_queries()`
- `save_saved_query(saved_query_id uuid, saved_query_name text, saved_query_description text, saved_query_payload jsonb)`

Those functions derive tenant context from JWT claims such as `tenant_id`,
`tenant_slug`, Google `hd`, Microsoft `tid`, or the local `X-Dev-Tenant` header.

Alert rule and contact management follows the same pattern. Builder queries can
use allow-listed structured filters, and dashboard panels carry layout metadata
so saved dashboards can be opened as a responsive panel grid.

- `GET /api/data-sources`
- `POST /api/data-sources`
- `POST /api/data-sources/test`
- `GET /api/query/history`
- `POST /api/sql`
- `GET /api/alerts/rules`
- `POST /api/alerts/rules`
- `POST /api/alerts/test`
- `GET /api/alerts/contacts`
- `POST /api/alerts/contacts`
- `GET /api/alerts/incidents`
- `GET /api/alerts/notifications`
- `GET /api/session/profile`
- `GET /api/session/memberships`
- `GET /api/audit/events`
- `GET /api/members`
- `POST /api/members/role`
- `POST /api/members/deactivate`
- `GET /api/invites`
- `POST /api/invites`
- `POST /api/invites/accept`

## Custom SQL

The UI supports a SQL mode for advanced ClickHouse exploration through
`POST /api/sql`. SQL queries are still tenant- and time-bounded by required
ClickHouse parameters:

```sql
SELECT service_name, severity, count() AS value
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND timestamp >= {from:DateTime}
  AND timestamp < {to:DateTime}
GROUP BY service_name, severity
ORDER BY value DESC
```

The server binds `tenant`, `from`, `to`, and `limit`, runs ClickHouse in
read-only mode, appends a controlled limit, and rejects multi-statement SQL,
comments, DDL/DML/admin statements, custom `FORMAT`/`SETTINGS`, and risky table
functions such as `url()`, `file()`, `s3()`, and `remote()`.

Custom SQL alert rules use the same safe execution path with stricter
validation: the query must return a numeric column named `value`. The alert
condition compares the maximum returned `value` against the configured
operator/threshold. `POST /api/alerts/test` previews that value before saving.

## Alerts

The alert worker is disabled by default. Enable it with:

```sh
DBVIZ_ALERTS_ENABLED=true
DBVIZ_ALERT_RULES_JSON='[{"id":"log-volume","name":"High log volume","tenantId":"dev","enabled":true,"query":{"dataset":"logs","groupBy":"service_name","aggregation":"count"},"condition":{"operator":"gt","threshold":100},"intervalSeconds":60,"contacts":[{"kind":"webhook","target":"http://example-webhook:8080/alerts","config":{}}]}]'
```

The worker evaluates rules through the same constrained ClickHouse query builder
used by the UI. Alert conditions can include a Go duration string in
`condition.for`, such as `5m`, to require the threshold to remain true before an
incident is recorded. Contact kinds are `webhook`, `pagerduty`, and `email`.
Email delivery is enabled when SMTP settings are configured:

```sh
DBVIZ_ALERT_SMTP_HOST=smtp.example.com
DBVIZ_ALERT_SMTP_PORT=587
DBVIZ_ALERT_SMTP_USER=alerts@example.com
DBVIZ_ALERT_SMTP_PASSWORD=...
DBVIZ_ALERT_SMTP_FROM=alerts@example.com
```

Persisted alert rules saved through `POST /api/alerts/rules` are loaded by the
worker when `DBVIZ_ALERT_LOAD_PERSISTED=true`. The worker uses
`DBVIZ_ALERT_WORKER_KEY` with the `list_enabled_alert_rules_for_worker()` RPC.
For production, set the database setting `app.alert_worker_key` to the same
secret value and do not use the dev default.

When a rule fires, the worker records an `alert_incidents` row through
`record_alert_incident_for_worker()`. Open firing incidents are deduped by a
stable fingerprint, `occurrence_count` and `last_seen_at` are updated on repeat
fires, and contact delivery is suppressed until `DBVIZ_ALERT_DEDUPE_SECONDS`
passes. When the condition clears, the worker marks the open incident
`resolved`; operators can also resolve incidents with
`POST /api/alerts/incidents/resolve`. Every contact delivery attempt is recorded
in `alert_notifications` with status, HTTP status code, contact target, and
error text. Notification failures are also recorded as `notify_failed` incidents
with the failed contact and error in the payload.
