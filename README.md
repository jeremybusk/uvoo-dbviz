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

Development auth is enabled by default in Compose. Paste any token is not needed;
the API accepts `X-Dev-Tenant: dev` when `DBVIZ_AUTH_DEV_MODE=true`, and the UI
can use real OIDC once a provider client ID is configured.

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
dataset has explicit table, time, tenant, filter, and dimension allow-lists so a
user request cannot bypass tenant scoping with arbitrary SQL.
