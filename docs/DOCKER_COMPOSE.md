# Docker Compose

Use Docker Compose for local end-to-end testing. It starts the application,
ClickHouse, PostgreSQL, PostgREST, Keycloak, and an OpenTelemetry Collector.

## Start

```sh
cp .env.example .env
docker compose up -d --build
```

Open <http://localhost:8080>.

Development auth is enabled by default. The seeded Keycloak users are:

- `alice` / `password`, tenant `dev`
- `bob` / `password`, tenant `example.com`

## LAN Browser Testing

Set the host clients will use before starting Compose:

```sh
SQVIZ_BIND_ADDR=0.0.0.0
SQVIZ_PUBLIC_HOST=192.168.1.50
docker compose up -d --build
```

Open `http://192.168.1.50:8080`.

## Sample Telemetry

```sh
docker compose run --rm otel-sample
```

The sample emitter sends OTLP JSON to the collector and inserts matching
normalized sample rows into ClickHouse.

## Smoke Test

```sh
make compose-smoke
```

The smoke test uses isolated host ports, waits for `/healthz`, syncs a dev
session, runs a query, saves a dashboard, emits sample telemetry, and tears the
stack down.
