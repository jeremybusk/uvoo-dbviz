Using PostgreSQL instead of MongoDB for your application state is an excellent idea. In fact, it is the superior choice for an observability control plane.

Observability metadata is inherently relational: Users belong to Organizations, Organizations own Dashboards, Dashboards contain Charts, and Alert Rules map to specific Contact Endpoints. Managing these complex relationships, foreign key constraints, and cascading deletions in MongoDB requires building fragile application-level logic. PostgreSQL handles this natively with strict ACID guarantees.

Furthermore, PostgreSQL's `JSONB` support gives you the exact same schema-less flexibility as MongoDB for storing arbitrary dashboard configurations or dynamic alert payloads, meaning you get the best of both worlds without the drawbacks of a document store.

Here is a recommended, highly performant stack tailored for a minimalist, turn-key observability platform:

### The Recommended Stack

* **Frontend:** React + Apache ECharts
* React provides the component-driven architecture needed for a complex UI. Apache ECharts is incredibly performant for time-series data and handles high-cardinality charts efficiently, especially when rendering to a Canvas element instead of SVG for massive datasets. Use Antd with dark and light theme.


* **API & Alerting Engine:** Go
* A custom backend written in Go provides a minimalist, high-performance foundation. Go's concurrency model (goroutines) is perfectly suited for running continuous alert evaluations, polling ClickHouse for threshold breaches, and firing off webhook triggers to contact endpoints without consuming massive amounts of memory.


* **Configuration & State:** PostgreSQL + PostgREST
* PostgreSQL stores all users, organization mapping, and dashboard layouts. Putting PostgREST on top of PostgreSQL allows your React frontend to perform direct CRUD operations on application state via a clean REST API, drastically reducing the amount of boilerplate backend code you need to write.


* **Identity & Access Management:** Keycloak (OIDC) + PostgreSQL RLS
* Keycloak handles the heavy lifting of user authentication, password management, and OIDC token generation. You can map the Keycloak identities directly to PostgreSQL using Row-Level Security (RLS). This ensures multi-tenant data isolation at the database layer; the API simply passes the JWT, and PostgreSQL ensures a user only queries configurations belonging to their tenant.


* **Telemetry Storage:** ClickHouse
* The undisputed champion for wide-column event storage, fast aggregations, and high-throughput ingestion of logs, metrics, and traces.


* **Ingestion Pipeline:** OpenTelemetry Collector & Fluent Bit
* Standardize your incoming data. Use the OpenTelemetry Collector as your primary gateway to receive structured OTLP traces and metrics. Use Fluent Bit for lightweight, high-throughput log tailing and routing directly into your ClickHouse tables.



### Architecture Flow

1. **Ingestion:** Services send data via OTLP or Fluent Bit directly to ClickHouse.
2. **Auth:** Users authenticate via Keycloak and receive a JWT.
3. **UI State:** React passes the JWT to PostgREST. PostgreSQL evaluates the RLS policies and returns the specific user's dashboard layouts (stored as JSONB).
4. **Visualization:** React parses the layout and queries the Go backend (or directly queries ClickHouse via a read-only proxy) to fetch the analytical data.
5. **Alerting:** The Go daemon continuously runs queries against ClickHouse based on the active rules stored in PostgreSQL, firing events when thresholds are met.

```sql
CREATE POLICY tenant_isolation_policy ON dashboards
    FOR ALL
    USING (tenant_id = current_setting('request.jwt.claim.tenant_id')::uuid);

```

This stack gives you the analytical power of ClickHouse, the relational integrity of PostgreSQL, and a lightweight, compiled backend that is incredibly easy to deploy and maintain.
