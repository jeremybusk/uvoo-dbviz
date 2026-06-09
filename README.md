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

Compose starts:

- `uvoo-dbviz` on <http://localhost:8080>
- ClickHouse on <http://localhost:8123> with seeded `dev` tenant data
- PostgreSQL on `localhost:5432`
- PostgREST on <http://localhost:3000>
- Keycloak on <http://localhost:8089> with realm `dbviz`
- OpenTelemetry Collector on `localhost:4317` and `localhost:4318`

Development auth is enabled by default in Compose. The seeded Keycloak users are:

- `alice` / `password`, tenant `dev`
- `bob` / `password`, tenant `example.com`

The UI can also use dev auth when `DBVIZ_AUTH_DEV_MODE=true`. Browser state
operations go through the Go API, which forwards bearer tokens or local dev
tenant headers to PostgREST. PostgREST still enforces RLS with JWT tenant claims
or `X-Dev-Tenant: dev` in local development.

On successful sign-in, the UI calls `POST /api/session/sync`. The backend records
or updates the current tenant/user in PostgreSQL, making later role and invite
rules explicit instead of keeping identity only in browser state.

Write operations for dashboards, alert rules, and contact endpoints are checked
against the synced PostgreSQL role. `owner`, `admin`, and `editor` can write;
`viewer` can read. Tenant invite management is limited to `owner` and `admin`.

Users can belong to a tenant even when their public IdP does not emit that
tenant as a claim. Owners and admins create an invite, the invited user signs in
with the matching email address, and `POST /api/invites/accept` attaches that
identity to the invited tenant. The UI then sends `X-DBViz-Tenant` for the
selected active tenant; the Go API forwards it to PostgREST with the verified
subject/provider headers, and PostgreSQL only resolves the tenant when a
matching membership exists.

Run the OTel sample emitter after the stack is up:

```sh
docker compose run --rm otel-sample
```

It sends OTLP JSON to the collector and inserts matching normalized sample rows
into ClickHouse so the default UI datasets can chart the data immediately.

## Build And Verify

```sh
make test
make web
make build
make license-check
helm lint charts/uvoo-dbviz
docker compose config
```

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
PostgREST RPCs:

- `GET /api/dashboards`
- `POST /api/dashboards`
- `list_dashboards()`
- `save_dashboard(dashboard_id uuid, dashboard_name text, dashboard_layout jsonb)`

Those functions derive tenant context from JWT claims such as `tenant_id`,
`tenant_slug`, Google `hd`, Microsoft `tid`, or the local `X-Dev-Tenant` header.

Alert rule and contact management follows the same pattern:

- `GET /api/data-sources`
- `POST /api/data-sources`
- `POST /api/data-sources/test`
- `GET /api/query/history`
- `GET /api/alerts/rules`
- `POST /api/alerts/rules`
- `GET /api/alerts/contacts`
- `POST /api/alerts/contacts`
- `GET /api/alerts/incidents`
- `GET /api/session/profile`
- `GET /api/session/memberships`
- `GET /api/members`
- `POST /api/members/role`
- `GET /api/invites`
- `POST /api/invites`
- `POST /api/invites/accept`

## Alerts

The alert worker is disabled by default. Enable it with:

```sh
DBVIZ_ALERTS_ENABLED=true
DBVIZ_ALERT_RULES_JSON='[{"id":"log-volume","name":"High log volume","tenantId":"dev","enabled":true,"query":{"dataset":"logs","groupBy":"service_name","aggregation":"count"},"condition":{"operator":"gt","threshold":100},"intervalSeconds":60,"contacts":[{"kind":"webhook","target":"http://example-webhook:8080/alerts","config":{}}]}]'
```

The worker evaluates rules through the same constrained ClickHouse query builder
used by the UI. Contact kinds are `webhook`, `pagerduty`, and `email`; email is
currently logged until SMTP configuration is added.

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
`POST /api/alerts/incidents/resolve`. Notification failures are recorded as
`notify_failed` incidents with the failed contact and error in the payload.
