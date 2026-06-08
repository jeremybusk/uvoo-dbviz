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

- `GET /api/alerts/rules`
- `POST /api/alerts/rules`
- `GET /api/alerts/contacts`
- `POST /api/alerts/contacts`

## Alerts

The alert worker is disabled by default. Enable it with:

```sh
DBVIZ_ALERTS_ENABLED=true
DBVIZ_ALERT_RULES_JSON='[{"id":"log-volume","name":"High log volume","tenantId":"dev","enabled":true,"query":{"dataset":"logs","groupBy":"service_name","aggregation":"count"},"condition":{"operator":"gt","threshold":100},"intervalSeconds":60,"contacts":[{"kind":"webhook","target":"http://example-webhook:8080/alerts","config":{}}]}]'
```

The worker evaluates rules through the same constrained ClickHouse query builder
used by the UI. Contact kinds are `webhook`, `pagerduty`, and `email`; email is
currently logged until SMTP configuration is added.
