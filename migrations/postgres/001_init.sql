CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dbviz_web') THEN
        CREATE ROLE dbviz_web NOLOGIN;
    END IF;
END $$;

DO $$
BEGIN
    EXECUTE format('GRANT dbviz_web TO %I', current_user);
END $$;

CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subject text NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    provider text NOT NULL,
    role text NOT NULL DEFAULT 'viewer' CHECK (role IN ('owner', 'admin', 'editor', 'viewer')),
    preferences jsonb NOT NULL DEFAULT '{}'::jsonb,
    disabled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(provider, subject),
    UNIQUE(tenant_id, email)
);

CREATE TABLE IF NOT EXISTS tenant_invites (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email text NOT NULL,
    role text NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'editor', 'viewer')),
    token text NOT NULL UNIQUE DEFAULT encode(gen_random_bytes(24), 'hex'),
    accepted_at timestamptz,
    expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, email)
);

CREATE TABLE IF NOT EXISTS data_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('clickhouse')),
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS dashboards (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    layout jsonb NOT NULL DEFAULT '{"version":1,"charts":[]}'::jsonb,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS saved_queries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    query jsonb NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS charts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    dashboard_id uuid NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    title text NOT NULL,
    query jsonb NOT NULL,
    visualization jsonb NOT NULL DEFAULT '{"type":"line"}'::jsonb,
    position jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS contact_endpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('email', 'webhook', 'pagerduty')),
    target text NOT NULL,
    secret_ref text NOT NULL DEFAULT '',
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenant_secrets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    ciphertext text NOT NULL,
    nonce text NOT NULL,
    key_version text NOT NULL DEFAULT 'v1',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    query jsonb NOT NULL,
    condition jsonb NOT NULL,
    interval_seconds integer NOT NULL DEFAULT 60 CHECK (interval_seconds >= 30),
    enabled boolean NOT NULL DEFAULT true,
    contact_endpoint_id uuid REFERENCES contact_endpoints(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_incidents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    alert_rule_id uuid REFERENCES alert_rules(id) ON DELETE SET NULL,
    fingerprint text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'firing' CHECK (status IN ('firing', 'resolved', 'notify_failed')),
    value double precision NOT NULL DEFAULT 0,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurrence_count integer NOT NULL DEFAULT 1,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    last_notified_at timestamptz,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    alert_rule_id uuid REFERENCES alert_rules(id) ON DELETE SET NULL,
    alert_incident_id uuid REFERENCES alert_incidents(id) ON DELETE SET NULL,
    contact_kind text NOT NULL DEFAULT '',
    contact_target text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('success', 'failed', 'skipped')),
    status_code integer NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS query_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    dataset text NOT NULL,
    query jsonb NOT NULL DEFAULT '{}'::jsonb,
    rows_count integer NOT NULL DEFAULT 0,
    duration_ms integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'success' CHECK (status IN ('success', 'failed')),
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_subject text NOT NULL DEFAULT '',
    actor_provider text NOT NULL DEFAULT '',
    actor_email text NOT NULL DEFAULT '',
    action text NOT NULL,
    target_type text NOT NULL DEFAULT '',
    target_id text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS disabled_at timestamptz;
ALTER TABLE users ADD COLUMN IF NOT EXISTS preferences jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE alert_incidents ADD COLUMN IF NOT EXISTS fingerprint text NOT NULL DEFAULT '';
ALTER TABLE alert_incidents ADD COLUMN IF NOT EXISTS occurrence_count integer NOT NULL DEFAULT 1;
ALTER TABLE alert_incidents ADD COLUMN IF NOT EXISTS first_seen_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE alert_incidents ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE alert_incidents ADD COLUMN IF NOT EXISTS last_notified_at timestamptz;
ALTER TABLE alert_incidents ADD COLUMN IF NOT EXISTS resolved_at timestamptz;

UPDATE alert_incidents
SET fingerprint = COALESCE(NULLIF(payload->>'fingerprint', ''), COALESCE(alert_rule_id::text, 'legacy') || ':' || id::text)
WHERE fingerprint = '';

CREATE INDEX IF NOT EXISTS users_tenant_id_idx ON users(tenant_id);
CREATE INDEX IF NOT EXISTS tenant_invites_tenant_id_idx ON tenant_invites(tenant_id);
CREATE INDEX IF NOT EXISTS data_sources_tenant_id_idx ON data_sources(tenant_id);
CREATE INDEX IF NOT EXISTS dashboards_tenant_id_idx ON dashboards(tenant_id);
CREATE INDEX IF NOT EXISTS saved_queries_tenant_updated_idx ON saved_queries(tenant_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS charts_dashboard_id_idx ON charts(dashboard_id);
CREATE INDEX IF NOT EXISTS tenant_secrets_tenant_updated_idx ON tenant_secrets(tenant_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS alert_rules_enabled_idx ON alert_rules(tenant_id, enabled);
CREATE INDEX IF NOT EXISTS alert_incidents_tenant_created_idx ON alert_incidents(tenant_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS alert_incidents_open_fingerprint_idx ON alert_incidents(tenant_id, fingerprint)
    WHERE status = 'firing' AND resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS alert_notifications_tenant_created_idx ON alert_notifications(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_history_tenant_created_idx ON query_history(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_tenant_created_idx ON audit_events(tenant_id, created_at DESC);

GRANT USAGE ON SCHEMA public TO dbviz_web;
GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, users, tenant_invites, data_sources, dashboards, saved_queries, charts, contact_endpoints, tenant_secrets, alert_rules, alert_incidents, alert_notifications, query_history, audit_events TO dbviz_web;

INSERT INTO tenants (slug, name)
VALUES ('dev', 'Development')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO dashboards (tenant_id, name, layout)
SELECT id, 'Sample Observability', '{"version":1,"charts":[{"title":"Log volume","query":{"dataset":"logs","groupBy":"service_name","measure":"_rows","aggregation":"count"},"visualization":{"type":"line"}}]}'::jsonb
FROM tenants
WHERE slug = 'dev'
  AND NOT EXISTS (
      SELECT 1
      FROM dashboards
      WHERE dashboards.tenant_id = tenants.id
        AND dashboards.name = 'Sample Observability'
)
ON CONFLICT DO NOTHING;

INSERT INTO saved_queries (tenant_id, name, description, query)
SELECT id, 'Log volume by service', 'Count log rows grouped by service name', '{"dataset":"logs","groupBy":"service_name","measure":"_rows","aggregation":"count"}'::jsonb
FROM tenants
WHERE slug = 'dev'
ON CONFLICT (tenant_id, name) DO NOTHING;

INSERT INTO data_sources (tenant_id, name, kind, config)
SELECT id, 'Default ClickHouse', 'clickhouse', '{"url":"http://clickhouse:8123","database":"default","username":"default","passwordSecretRef":"clickhouse-default"}'::jsonb
FROM tenants
WHERE slug = 'dev'
ON CONFLICT (tenant_id, name) DO NOTHING;

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE dashboards ENABLE ROW LEVEL SECURITY;
ALTER TABLE saved_queries ENABLE ROW LEVEL SECURITY;
ALTER TABLE charts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_endpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_incidents ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE query_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION request_tenant_id() RETURNS uuid
LANGUAGE plpgsql STABLE
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    request_headers jsonb;
    tenant_id_claim text;
    tenant_slug_claim text;
    requested_subject text;
    requested_provider text;
    resolved_tenant_id uuid;
BEGIN
    request_headers := COALESCE(NULLIF(current_setting('request.headers', true), '')::jsonb, '{}'::jsonb);
    tenant_id_claim := NULLIF(current_setting('request.jwt.claim.tenant_id', true), '');
    tenant_slug_claim := COALESCE(
        NULLIF(request_headers->>'x-dbviz-tenant', ''),
        NULLIF(request_headers->>'x-dev-tenant', ''),
        NULLIF(current_setting('request.jwt.claim.tenant_key', true), ''),
        NULLIF(current_setting('request.jwt.claim.tenant_slug', true), ''),
        NULLIF(current_setting('request.jwt.claim.hd', true), ''),
        NULLIF(current_setting('request.jwt.claim.tid', true), '')
    );
    requested_subject := NULLIF(request_headers->>'x-dbviz-subject', '');
    requested_provider := NULLIF(request_headers->>'x-dbviz-provider', '');

    IF tenant_slug_claim ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
        resolved_tenant_id := tenant_slug_claim::uuid;
    ELSIF tenant_id_claim ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
        resolved_tenant_id := tenant_id_claim::uuid;
    ELSIF tenant_slug_claim IS NOT NULL THEN
        SELECT tenants.id INTO resolved_tenant_id
        FROM tenants
        WHERE tenants.slug = tenant_slug_claim;
    END IF;

    IF resolved_tenant_id IS NULL THEN
        RETURN NULL;
    END IF;

    IF requested_subject IS NOT NULL AND requested_provider IS NOT NULL THEN
        IF EXISTS (
            SELECT 1
            FROM users u
            WHERE u.tenant_id = resolved_tenant_id
              AND u.subject = requested_subject
              AND u.provider = requested_provider
              AND u.disabled_at IS NULL
        ) THEN
            RETURN resolved_tenant_id;
        END IF;
        RETURN NULL;
    END IF;

    RETURN resolved_tenant_id;
END;
$$;

DROP POLICY IF EXISTS tenant_self ON tenants;
CREATE POLICY tenant_self ON tenants
    FOR ALL USING (id = request_tenant_id())
    WITH CHECK (id = request_tenant_id());

DROP POLICY IF EXISTS users_tenant_isolation ON users;
CREATE POLICY users_tenant_isolation ON users
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS tenant_invites_tenant_isolation ON tenant_invites;
CREATE POLICY tenant_invites_tenant_isolation ON tenant_invites
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS data_sources_tenant_isolation ON data_sources;
CREATE POLICY data_sources_tenant_isolation ON data_sources
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS dashboards_tenant_isolation ON dashboards;
CREATE POLICY dashboards_tenant_isolation ON dashboards
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS saved_queries_tenant_isolation ON saved_queries;
CREATE POLICY saved_queries_tenant_isolation ON saved_queries
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS charts_tenant_isolation ON charts;
CREATE POLICY charts_tenant_isolation ON charts
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS contact_endpoints_tenant_isolation ON contact_endpoints;
CREATE POLICY contact_endpoints_tenant_isolation ON contact_endpoints
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS tenant_secrets_tenant_isolation ON tenant_secrets;
CREATE POLICY tenant_secrets_tenant_isolation ON tenant_secrets
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS alert_rules_tenant_isolation ON alert_rules;
CREATE POLICY alert_rules_tenant_isolation ON alert_rules
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS alert_incidents_tenant_isolation ON alert_incidents;
CREATE POLICY alert_incidents_tenant_isolation ON alert_incidents
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS alert_notifications_tenant_isolation ON alert_notifications;
CREATE POLICY alert_notifications_tenant_isolation ON alert_notifications
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS query_history_tenant_isolation ON query_history;
CREATE POLICY query_history_tenant_isolation ON query_history
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS audit_events_tenant_isolation ON audit_events;
CREATE POLICY audit_events_tenant_isolation ON audit_events
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

CREATE OR REPLACE FUNCTION list_data_sources()
RETURNS TABLE(id uuid, name text, kind text, config jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT ds.id, ds.name, ds.kind, ds.config - 'password' AS config, ds.updated_at, ds.created_at
    FROM data_sources ds
    WHERE ds.tenant_id = request_tenant_id()
    ORDER BY ds.name;
$$;

CREATE OR REPLACE FUNCTION get_data_source(source_id uuid)
RETURNS TABLE(id uuid, name text, kind text, config jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT ds.id, ds.name, ds.kind, ds.config - 'password' AS config, ds.updated_at, ds.created_at
    FROM data_sources ds
    WHERE ds.tenant_id = request_tenant_id()
      AND ds.id = source_id
    LIMIT 1;
$$;

CREATE OR REPLACE FUNCTION save_data_source(source_id uuid, source_name text, source_kind text, source_config jsonb)
RETURNS TABLE(id uuid, name text, kind text, config jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
    sanitized_config jsonb;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    IF source_kind <> 'clickhouse' THEN
        RAISE EXCEPTION 'unsupported data source kind: %', source_kind;
    END IF;

    sanitized_config := COALESCE(source_config, '{}'::jsonb) - 'password';

    IF source_id IS NULL THEN
        INSERT INTO data_sources (tenant_id, name, kind, config)
        VALUES (request_tenant_id(), source_name, source_kind, sanitized_config)
        ON CONFLICT ON CONSTRAINT data_sources_tenant_id_name_key DO UPDATE
        SET kind = EXCLUDED.kind,
            config = EXCLUDED.config,
            updated_at = now()
        RETURNING data_sources.id INTO saved_id;
    ELSE
        UPDATE data_sources
        SET name = source_name,
            kind = source_kind,
            config = sanitized_config,
            updated_at = now()
        WHERE data_sources.id = source_id
          AND data_sources.tenant_id = request_tenant_id()
        RETURNING data_sources.id INTO saved_id;
    END IF;

    RETURN QUERY
    SELECT ds.id, ds.name, ds.kind, ds.config - 'password' AS config, ds.updated_at, ds.created_at
    FROM data_sources ds
    WHERE ds.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_data_source(source_id uuid)
RETURNS TABLE(id uuid, name text, kind text, config jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    RETURN QUERY
    DELETE FROM data_sources ds
    WHERE ds.id = source_id
      AND ds.tenant_id = request_tenant_id()
    RETURNING ds.id, ds.name, ds.kind, ds.config - 'password', ds.updated_at, ds.created_at;
END;
$$;

CREATE OR REPLACE FUNCTION list_query_history(history_limit integer DEFAULT 50)
RETURNS TABLE(id uuid, user_email text, dataset text, query jsonb, rows_count integer, duration_ms integer, status text, error text, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT qh.id, COALESCE(u.email, '') AS user_email, qh.dataset, qh.query, qh.rows_count, qh.duration_ms, qh.status, qh.error, qh.created_at
    FROM query_history qh
    LEFT JOIN users u ON u.id = qh.user_id
    WHERE qh.tenant_id = request_tenant_id()
    ORDER BY qh.created_at DESC
    LIMIT LEAST(GREATEST(COALESCE(history_limit, 50), 1), 200);
$$;

CREATE OR REPLACE FUNCTION record_query_history(
    user_subject text,
    user_provider text,
    query_dataset text,
    query_payload jsonb,
    query_rows_count integer,
    query_duration_ms integer,
    query_status text,
    query_error text
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    resolved_user_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    SELECT u.id INTO resolved_user_id
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = user_subject
      AND u.provider = user_provider
    LIMIT 1;

    INSERT INTO query_history (tenant_id, user_id, dataset, query, rows_count, duration_ms, status, error)
    VALUES (
        request_tenant_id(),
        resolved_user_id,
        COALESCE(NULLIF(query_dataset, ''), 'unknown'),
        COALESCE(query_payload, '{}'::jsonb),
        GREATEST(COALESCE(query_rows_count, 0), 0),
        GREATEST(COALESCE(query_duration_ms, 0), 0),
        CASE WHEN query_status = 'failed' THEN 'failed' ELSE 'success' END,
        LEFT(COALESCE(query_error, ''), 2000)
    );
END;
$$;

GRANT EXECUTE ON FUNCTION list_data_sources() TO dbviz_web;
GRANT EXECUTE ON FUNCTION get_data_source(uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_data_source(uuid, text, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_data_source(uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_query_history(integer) TO dbviz_web;
GRANT EXECUTE ON FUNCTION record_query_history(text, text, text, jsonb, integer, integer, text, text) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_audit_events(event_limit integer DEFAULT 100)
RETURNS TABLE(id uuid, actor_email text, action text, target_type text, target_id text, payload jsonb, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT ae.id, ae.actor_email, ae.action, ae.target_type, ae.target_id, ae.payload, ae.created_at
    FROM audit_events ae
    WHERE ae.tenant_id = request_tenant_id()
    ORDER BY ae.created_at DESC
    LIMIT LEAST(GREATEST(COALESCE(event_limit, 100), 1), 500);
$$;

CREATE OR REPLACE FUNCTION record_audit_event(
    actor_subject text,
    actor_provider text,
    actor_email text,
    event_action text,
    event_target_type text,
    event_target_id text,
    event_payload jsonb
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    resolved_user_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;
    IF event_action IS NULL OR event_action = '' THEN
        RAISE EXCEPTION 'audit action is required';
    END IF;

    SELECT u.id INTO resolved_user_id
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = actor_subject
      AND u.provider = actor_provider
    LIMIT 1;

    INSERT INTO audit_events (tenant_id, actor_user_id, actor_subject, actor_provider, actor_email, action, target_type, target_id, payload)
    VALUES (
        request_tenant_id(),
        resolved_user_id,
        COALESCE(actor_subject, ''),
        COALESCE(actor_provider, ''),
        COALESCE(actor_email, ''),
        event_action,
        COALESCE(event_target_type, ''),
        COALESCE(event_target_id, ''),
        COALESCE(event_payload, '{}'::jsonb)
    );
END;
$$;

GRANT EXECUTE ON FUNCTION list_audit_events(integer) TO dbviz_web;
GRANT EXECUTE ON FUNCTION record_audit_event(text, text, text, text, text, text, jsonb) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_dashboards()
RETURNS TABLE(id uuid, name text, layout jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT d.id, d.name, d.layout, d.updated_at, d.created_at
    FROM dashboards d
    WHERE d.tenant_id = request_tenant_id()
    ORDER BY d.updated_at DESC;
$$;

CREATE OR REPLACE FUNCTION save_dashboard(dashboard_id uuid, dashboard_name text, dashboard_layout jsonb)
RETURNS TABLE(id uuid, name text, layout jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    IF dashboard_id IS NULL THEN
        INSERT INTO dashboards (tenant_id, name, layout)
        VALUES (request_tenant_id(), dashboard_name, dashboard_layout)
        RETURNING dashboards.id INTO saved_id;
    ELSE
        UPDATE dashboards
        SET name = dashboard_name,
            layout = dashboard_layout,
            updated_at = now()
        WHERE dashboards.id = dashboard_id
          AND dashboards.tenant_id = request_tenant_id()
        RETURNING dashboards.id INTO saved_id;
    END IF;

    RETURN QUERY
    SELECT d.id, d.name, d.layout, d.updated_at, d.created_at
    FROM dashboards d
    WHERE d.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_dashboard(dashboard_id uuid)
RETURNS TABLE(id uuid, name text, layout jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    RETURN QUERY
    DELETE FROM dashboards d
    WHERE d.id = dashboard_id
      AND d.tenant_id = request_tenant_id()
    RETURNING d.id, d.name, d.layout, d.updated_at, d.created_at;
END;
$$;

GRANT EXECUTE ON FUNCTION list_dashboards() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_dashboard(uuid, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_dashboard(uuid) TO dbviz_web;
NOTIFY pgrst, 'reload schema';

CREATE OR REPLACE FUNCTION list_saved_queries()
RETURNS TABLE(id uuid, name text, description text, query jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT sq.id, sq.name, sq.description, sq.query, sq.updated_at, sq.created_at
    FROM saved_queries sq
    WHERE sq.tenant_id = request_tenant_id()
    ORDER BY sq.updated_at DESC;
$$;

CREATE OR REPLACE FUNCTION save_saved_query(saved_query_id uuid, saved_query_name text, saved_query_description text, saved_query_payload jsonb)
RETURNS TABLE(id uuid, name text, description text, query jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
    resolved_user_id uuid;
    request_headers jsonb;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    request_headers := COALESCE(NULLIF(current_setting('request.headers', true), '')::jsonb, '{}'::jsonb);

    SELECT u.id INTO resolved_user_id
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = request_headers->>'x-dbviz-subject'
      AND u.provider = request_headers->>'x-dbviz-provider'
    LIMIT 1;

    IF saved_query_id IS NULL THEN
        INSERT INTO saved_queries (tenant_id, name, description, query, created_by)
        VALUES (request_tenant_id(), saved_query_name, COALESCE(saved_query_description, ''), saved_query_payload, resolved_user_id)
        ON CONFLICT ON CONSTRAINT saved_queries_tenant_id_name_key DO UPDATE
        SET description = EXCLUDED.description,
            query = EXCLUDED.query,
            updated_at = now()
        RETURNING saved_queries.id INTO saved_id;
    ELSE
        UPDATE saved_queries
        SET name = saved_query_name,
            description = COALESCE(saved_query_description, ''),
            query = saved_query_payload,
            updated_at = now()
        WHERE saved_queries.id = saved_query_id
          AND saved_queries.tenant_id = request_tenant_id()
        RETURNING saved_queries.id INTO saved_id;
    END IF;

    RETURN QUERY
    SELECT sq.id, sq.name, sq.description, sq.query, sq.updated_at, sq.created_at
    FROM saved_queries sq
    WHERE sq.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_saved_query(saved_query_id uuid)
RETURNS TABLE(id uuid, name text, description text, query jsonb, updated_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    RETURN QUERY
    DELETE FROM saved_queries sq
    WHERE sq.id = saved_query_id
      AND sq.tenant_id = request_tenant_id()
    RETURNING sq.id, sq.name, sq.description, sq.query, sq.updated_at, sq.created_at;
END;
$$;

GRANT EXECUTE ON FUNCTION list_saved_queries() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_saved_query(uuid, text, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_saved_query(uuid) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_tenant_secrets()
RETURNS TABLE(id uuid, name text, description text, key_version text, created_at timestamptz, updated_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT s.id, s.name, s.description, s.key_version, s.created_at, s.updated_at
    FROM tenant_secrets s
    WHERE s.tenant_id = request_tenant_id()
    ORDER BY s.updated_at DESC;
$$;

CREATE OR REPLACE FUNCTION get_tenant_secret(secret_name text)
RETURNS TABLE(name text, ciphertext text, nonce text, key_version text)
LANGUAGE sql STABLE
AS $$
    SELECT s.name, s.ciphertext, s.nonce, s.key_version
    FROM tenant_secrets s
    WHERE s.tenant_id = request_tenant_id()
      AND s.name = secret_name
    LIMIT 1;
$$;

CREATE OR REPLACE FUNCTION list_tenant_secret_usage(secret_name text)
RETURNS TABLE(resource_type text, resource_id uuid, resource_name text, field text)
LANGUAGE sql STABLE
AS $$
    SELECT 'contact'::text, c.id, c.name, 'PagerDuty Events integration key'::text
    FROM contact_endpoints c
    WHERE c.tenant_id = request_tenant_id()
      AND c.config->>'routingKeySecretRef' = secret_name
    UNION ALL
    SELECT 'contact'::text, c.id, c.name, 'PagerDuty REST API key'::text
    FROM contact_endpoints c
    WHERE c.tenant_id = request_tenant_id()
      AND c.config->>'restApiKeySecretRef' = secret_name
    UNION ALL
    SELECT 'contact'::text, c.id, c.name, 'Webhook bearer token'::text
    FROM contact_endpoints c
    WHERE c.tenant_id = request_tenant_id()
      AND c.config->>'tokenSecretRef' = secret_name
    UNION ALL
    SELECT 'contact'::text, c.id, c.name, 'Webhook custom header'::text
    FROM contact_endpoints c
    WHERE c.tenant_id = request_tenant_id()
      AND c.config->>'headerValueSecretRef' = secret_name
    UNION ALL
    SELECT 'data_source'::text, d.id, d.name, 'Password'::text
    FROM data_sources d
    WHERE d.tenant_id = request_tenant_id()
      AND d.config->>'passwordSecretRef' = secret_name
    ORDER BY 1, 3, 4;
$$;

CREATE OR REPLACE FUNCTION save_tenant_secret(secret_id uuid, secret_name text, secret_description text, secret_ciphertext text, secret_nonce text, secret_key_version text)
RETURNS TABLE(id uuid, name text, description text, key_version text, created_at timestamptz, updated_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    IF NULLIF(trim(secret_name), '') IS NULL THEN
        RAISE EXCEPTION 'secret name is required';
    END IF;

    IF NULLIF(secret_ciphertext, '') IS NULL OR NULLIF(secret_nonce, '') IS NULL THEN
        RAISE EXCEPTION 'encrypted secret value is required';
    END IF;

    IF secret_id IS NULL THEN
        INSERT INTO tenant_secrets (tenant_id, name, description, ciphertext, nonce, key_version)
        VALUES (
            request_tenant_id(),
            trim(secret_name),
            COALESCE(secret_description, ''),
            secret_ciphertext,
            secret_nonce,
            COALESCE(NULLIF(secret_key_version, ''), 'v1')
        )
        ON CONFLICT ON CONSTRAINT tenant_secrets_tenant_id_name_key DO UPDATE
        SET description = EXCLUDED.description,
            ciphertext = EXCLUDED.ciphertext,
            nonce = EXCLUDED.nonce,
            key_version = EXCLUDED.key_version,
            updated_at = now()
        RETURNING tenant_secrets.id INTO saved_id;
    ELSE
        UPDATE tenant_secrets
        SET name = trim(secret_name),
            description = COALESCE(secret_description, ''),
            ciphertext = secret_ciphertext,
            nonce = secret_nonce,
            key_version = COALESCE(NULLIF(secret_key_version, ''), 'v1'),
            updated_at = now()
        WHERE tenant_secrets.id = secret_id
          AND tenant_secrets.tenant_id = request_tenant_id()
        RETURNING tenant_secrets.id INTO saved_id;
    END IF;

    RETURN QUERY
    SELECT s.id, s.name, s.description, s.key_version, s.created_at, s.updated_at
    FROM tenant_secrets s
    WHERE s.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_tenant_secret(secret_id uuid)
RETURNS TABLE(id uuid, name text, description text, key_version text, created_at timestamptz, updated_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    RETURN QUERY
    DELETE FROM tenant_secrets s
    WHERE s.id = secret_id
      AND s.tenant_id = request_tenant_id()
    RETURNING s.id, s.name, s.description, s.key_version, s.created_at, s.updated_at;
END;
$$;

GRANT EXECUTE ON FUNCTION list_tenant_secrets() TO dbviz_web;
GRANT EXECUTE ON FUNCTION get_tenant_secret(text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_tenant_secret_usage(text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_tenant_secret(uuid, text, text, text, text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_tenant_secret(uuid) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_contact_endpoints()
RETURNS TABLE(id uuid, name text, kind text, target text, config jsonb, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT c.id, c.name, c.kind, c.target, c.config, c.created_at
    FROM contact_endpoints c
    WHERE c.tenant_id = request_tenant_id()
    ORDER BY c.created_at DESC;
$$;

CREATE OR REPLACE FUNCTION save_contact_endpoint(contact_id uuid, contact_name text, contact_kind text, contact_target text, contact_config jsonb)
RETURNS TABLE(id uuid, name text, kind text, target text, config jsonb, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    IF contact_kind NOT IN ('email', 'webhook', 'pagerduty') THEN
        RAISE EXCEPTION 'unsupported contact kind: %', contact_kind;
    END IF;

    IF contact_id IS NULL THEN
        INSERT INTO contact_endpoints (tenant_id, name, kind, target, config)
        VALUES (request_tenant_id(), contact_name, contact_kind, contact_target, COALESCE(contact_config, '{}'::jsonb))
        RETURNING contact_endpoints.id INTO saved_id;
    ELSE
        UPDATE contact_endpoints
        SET name = contact_name,
            kind = contact_kind,
            target = contact_target,
            config = COALESCE(contact_config, '{}'::jsonb)
        WHERE contact_endpoints.id = contact_id
          AND contact_endpoints.tenant_id = request_tenant_id()
        RETURNING contact_endpoints.id INTO saved_id;
    END IF;

    RETURN QUERY
    SELECT c.id, c.name, c.kind, c.target, c.config, c.created_at
    FROM contact_endpoints c
    WHERE c.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_contact_endpoint(contact_id uuid)
RETURNS TABLE(id uuid, name text, kind text, target text, config jsonb, created_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    RETURN QUERY
    DELETE FROM contact_endpoints c
    WHERE c.id = contact_id
      AND c.tenant_id = request_tenant_id()
    RETURNING c.id, c.name, c.kind, c.target, c.config, c.created_at;
END;
$$;

CREATE OR REPLACE FUNCTION list_alert_rules()
RETURNS TABLE(id uuid, name text, query jsonb, condition jsonb, interval_seconds integer, enabled boolean, contact_endpoint_id uuid, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT a.id, a.name, a.query, a.condition, a.interval_seconds, a.enabled, a.contact_endpoint_id, a.created_at
    FROM alert_rules a
    WHERE a.tenant_id = request_tenant_id()
    ORDER BY a.created_at DESC;
$$;

DROP FUNCTION IF EXISTS list_alert_incidents(integer);

CREATE OR REPLACE FUNCTION list_alert_incidents(incident_limit integer DEFAULT 100)
RETURNS TABLE(
    id uuid,
    alert_rule_id uuid,
    fingerprint text,
    status text,
    value double precision,
    payload jsonb,
    occurrence_count integer,
    first_seen_at timestamptz,
    last_seen_at timestamptz,
    last_notified_at timestamptz,
    resolved_at timestamptz,
    created_at timestamptz
)
LANGUAGE sql STABLE
AS $$
    SELECT
        i.id,
        i.alert_rule_id,
        i.fingerprint,
        i.status,
        i.value,
        i.payload,
        i.occurrence_count,
        i.first_seen_at,
        i.last_seen_at,
        i.last_notified_at,
        i.resolved_at,
        i.created_at
    FROM alert_incidents i
    WHERE i.tenant_id = request_tenant_id()
    ORDER BY i.last_seen_at DESC
    LIMIT LEAST(GREATEST(COALESCE(incident_limit, 100), 1), 500);
$$;

CREATE OR REPLACE FUNCTION list_alert_notifications(notification_limit integer DEFAULT 100)
RETURNS TABLE(
    id uuid,
    alert_rule_id uuid,
    alert_incident_id uuid,
    contact_kind text,
    contact_target text,
    status text,
    status_code integer,
    error text,
    payload jsonb,
    created_at timestamptz
)
LANGUAGE sql STABLE
AS $$
    SELECT
        n.id,
        n.alert_rule_id,
        n.alert_incident_id,
        n.contact_kind,
        n.contact_target,
        n.status,
        n.status_code,
        n.error,
        n.payload,
        n.created_at
    FROM alert_notifications n
    WHERE n.tenant_id = request_tenant_id()
    ORDER BY n.created_at DESC
    LIMIT LEAST(GREATEST(COALESCE(notification_limit, 100), 1), 500);
$$;

CREATE OR REPLACE FUNCTION record_contact_test_notification(
    notification_contact_kind text,
    notification_contact_target text,
    delivery_status text,
    delivery_status_code integer,
    delivery_error text,
    delivery_payload jsonb
)
RETURNS TABLE(
    id uuid,
    alert_rule_id uuid,
    alert_incident_id uuid,
    contact_kind text,
    contact_target text,
    status text,
    status_code integer,
    error text,
    payload jsonb,
    created_at timestamptz
)
LANGUAGE plpgsql
AS $$
DECLARE
    normalized_status text;
    saved_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    normalized_status := COALESCE(NULLIF(delivery_status, ''), 'failed');
    IF normalized_status NOT IN ('success', 'failed', 'skipped') THEN
        normalized_status := 'failed';
    END IF;

    INSERT INTO alert_notifications (
        tenant_id,
        contact_kind,
        contact_target,
        status,
        status_code,
        error,
        payload
    )
    VALUES (
        request_tenant_id(),
        COALESCE(notification_contact_kind, ''),
        COALESCE(notification_contact_target, ''),
        normalized_status,
        GREATEST(COALESCE(delivery_status_code, 0), 0),
        LEFT(COALESCE(delivery_error, ''), 2000),
        COALESCE(delivery_payload, '{}'::jsonb)
    )
    RETURNING alert_notifications.id INTO saved_id;

    RETURN QUERY
    SELECT
        n.id,
        n.alert_rule_id,
        n.alert_incident_id,
        n.contact_kind,
        n.contact_target,
        n.status,
        n.status_code,
        n.error,
        n.payload,
        n.created_at
    FROM alert_notifications n
    WHERE n.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION resolve_alert_incident(actor_subject text, actor_provider text, incident_id uuid)
RETURNS TABLE(
    id uuid,
    alert_rule_id uuid,
    fingerprint text,
    status text,
    value double precision,
    payload jsonb,
    occurrence_count integer,
    first_seen_at timestamptz,
    last_seen_at timestamptz,
    last_notified_at timestamptz,
    resolved_at timestamptz,
    created_at timestamptz
)
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT current_user_has_role(actor_subject, actor_provider, ARRAY['owner', 'admin', 'editor']) THEN
        RAISE EXCEPTION 'insufficient role';
    END IF;

    UPDATE alert_incidents
    SET status = 'resolved',
        resolved_at = COALESCE(alert_incidents.resolved_at, now()),
        last_seen_at = now(),
        payload = alert_incidents.payload || jsonb_build_object('resolvedBy', actor_subject, 'resolvedManually', true)
    WHERE alert_incidents.id = incident_id
      AND alert_incidents.tenant_id = request_tenant_id()
    RETURNING alert_incidents.id INTO incident_id;

    RETURN QUERY
    SELECT
        i.id,
        i.alert_rule_id,
        i.fingerprint,
        i.status,
        i.value,
        i.payload,
        i.occurrence_count,
        i.first_seen_at,
        i.last_seen_at,
        i.last_notified_at,
        i.resolved_at,
        i.created_at
    FROM alert_incidents i
    WHERE i.id = incident_id;
END;
$$;

CREATE OR REPLACE FUNCTION save_alert_rule(
    alert_id uuid,
    alert_name text,
    alert_query jsonb,
    alert_condition jsonb,
    alert_interval integer,
    alert_enabled boolean,
    alert_contact_id uuid
)
RETURNS TABLE(id uuid, name text, query jsonb, condition jsonb, interval_seconds integer, enabled boolean, contact_endpoint_id uuid, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    IF alert_interval IS NULL OR alert_interval < 30 THEN
        alert_interval := 60;
    END IF;

    IF alert_contact_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM contact_endpoints
        WHERE contact_endpoints.id = alert_contact_id
          AND contact_endpoints.tenant_id = request_tenant_id()
    ) THEN
        RAISE EXCEPTION 'contact endpoint is not available for tenant';
    END IF;

    IF alert_id IS NULL THEN
        INSERT INTO alert_rules (tenant_id, name, query, condition, interval_seconds, enabled, contact_endpoint_id)
        VALUES (request_tenant_id(), alert_name, alert_query, alert_condition, alert_interval, COALESCE(alert_enabled, true), alert_contact_id)
        RETURNING alert_rules.id INTO saved_id;
    ELSE
        UPDATE alert_rules
        SET name = alert_name,
            query = alert_query,
            condition = alert_condition,
            interval_seconds = alert_interval,
            enabled = COALESCE(alert_enabled, true),
            contact_endpoint_id = alert_contact_id
        WHERE alert_rules.id = alert_id
          AND alert_rules.tenant_id = request_tenant_id()
        RETURNING alert_rules.id INTO saved_id;
    END IF;

    RETURN QUERY
    SELECT a.id, a.name, a.query, a.condition, a.interval_seconds, a.enabled, a.contact_endpoint_id, a.created_at
    FROM alert_rules a
    WHERE a.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_alert_rule(alert_id uuid)
RETURNS TABLE(id uuid, name text, query jsonb, condition jsonb, interval_seconds integer, enabled boolean, contact_endpoint_id uuid, created_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF request_tenant_id() IS NULL THEN
        RAISE EXCEPTION 'tenant context is required';
    END IF;

    RETURN QUERY
    DELETE FROM alert_rules a
    WHERE a.id = alert_id
      AND a.tenant_id = request_tenant_id()
    RETURNING a.id, a.name, a.query, a.condition, a.interval_seconds, a.enabled, a.contact_endpoint_id, a.created_at;
END;
$$;

GRANT EXECUTE ON FUNCTION list_contact_endpoints() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_contact_endpoint(uuid, text, text, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_contact_endpoint(uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_alert_rules() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_alert_rule(uuid, text, jsonb, jsonb, integer, boolean, uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_alert_rule(uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_alert_incidents(integer) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_alert_notifications(integer) TO dbviz_web;
GRANT EXECUTE ON FUNCTION record_contact_test_notification(text, text, text, integer, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION resolve_alert_incident(text, text, uuid) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_enabled_alert_rules_for_worker(worker_key text)
RETURNS TABLE(
    id uuid,
    name text,
    tenant_id text,
    query jsonb,
    condition jsonb,
    interval_seconds integer,
    enabled boolean,
    contact_kind text,
    contact_target text,
    contact_config jsonb
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
    IF worker_key IS NULL OR worker_key <> COALESCE(current_setting('app.alert_worker_key', true), 'dev-alert-worker-key') THEN
        RAISE EXCEPTION 'invalid alert worker key';
    END IF;

    RETURN QUERY
    SELECT
        a.id,
        a.name,
        t.slug AS tenant_id,
        a.query,
        a.condition,
        a.interval_seconds,
        a.enabled,
        c.kind AS contact_kind,
        c.target AS contact_target,
        COALESCE(c.config, '{}'::jsonb) AS contact_config
    FROM alert_rules a
    JOIN tenants t ON t.id = a.tenant_id
    LEFT JOIN contact_endpoints c ON c.id = a.contact_endpoint_id
    WHERE a.enabled = true
    ORDER BY a.created_at ASC;
END;
$$;

GRANT EXECUTE ON FUNCTION list_enabled_alert_rules_for_worker(text) TO dbviz_web;

CREATE OR REPLACE FUNCTION get_tenant_secret_for_worker(worker_key text, tenant_slug text, secret_name text)
RETURNS TABLE(name text, ciphertext text, nonce text, key_version text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    resolved_tenant_id uuid;
BEGIN
    IF worker_key IS NULL OR worker_key <> COALESCE(current_setting('app.alert_worker_key', true), 'dev-alert-worker-key') THEN
        RAISE EXCEPTION 'invalid alert worker key';
    END IF;

    SELECT tenants.id INTO resolved_tenant_id
    FROM tenants
    WHERE tenants.slug = tenant_slug;

    IF resolved_tenant_id IS NULL THEN
        RAISE EXCEPTION 'tenant not found: %', tenant_slug;
    END IF;

    RETURN QUERY
    SELECT s.name, s.ciphertext, s.nonce, s.key_version
    FROM tenant_secrets s
    WHERE s.tenant_id = resolved_tenant_id
      AND s.name = secret_name
    LIMIT 1;
END;
$$;

GRANT EXECUTE ON FUNCTION get_tenant_secret_for_worker(text, text, text) TO dbviz_web;

DROP FUNCTION IF EXISTS record_alert_incident_for_worker(text, uuid, text, text, double precision, jsonb);
DROP FUNCTION IF EXISTS record_alert_incident_for_worker(text, uuid, text, text, double precision, jsonb, text, integer);

CREATE OR REPLACE FUNCTION record_alert_incident_for_worker(
    worker_key text,
    rule_id uuid,
    tenant_slug text,
    incident_status text,
    incident_value double precision,
    incident_payload jsonb,
    incident_fingerprint text DEFAULT '',
    cooldown_seconds integer DEFAULT 300
)
RETURNS TABLE(
    id uuid,
    alert_rule_id uuid,
    fingerprint text,
    status text,
    value double precision,
    payload jsonb,
    occurrence_count integer,
    first_seen_at timestamptz,
    last_seen_at timestamptz,
    last_notified_at timestamptz,
    resolved_at timestamptz,
    created_at timestamptz,
    deduped boolean,
    should_notify boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    resolved_tenant_id uuid;
    normalized_status text;
    normalized_fingerprint text;
    existing_id uuid;
    notify_now boolean;
    saved_id uuid;
BEGIN
    IF worker_key IS NULL OR worker_key <> COALESCE(current_setting('app.alert_worker_key', true), 'dev-alert-worker-key') THEN
        RAISE EXCEPTION 'invalid alert worker key';
    END IF;

    SELECT tenants.id INTO resolved_tenant_id
    FROM tenants
    WHERE tenants.slug = tenant_slug;

    IF resolved_tenant_id IS NULL THEN
        RAISE EXCEPTION 'tenant not found: %', tenant_slug;
    END IF;

    normalized_status := COALESCE(NULLIF(incident_status, ''), 'firing');
    normalized_fingerprint := COALESCE(NULLIF(incident_fingerprint, ''), COALESCE(rule_id::text, tenant_slug || ':' || COALESCE(incident_payload->>'ruleName', 'unknown')));

    IF normalized_status = 'resolved' THEN
    UPDATE alert_incidents
    SET status = 'resolved',
        value = COALESCE(incident_value, alert_incidents.value),
        payload = COALESCE(incident_payload, '{}'::jsonb),
        last_seen_at = now(),
        resolved_at = now()
        WHERE alert_incidents.id = (
            SELECT i.id
            FROM alert_incidents i
            WHERE i.tenant_id = resolved_tenant_id
              AND i.fingerprint = normalized_fingerprint
              AND i.status = 'firing'
              AND i.resolved_at IS NULL
            ORDER BY i.last_seen_at DESC
            LIMIT 1
        )
        RETURNING alert_incidents.id INTO saved_id;

        IF saved_id IS NULL THEN
            RETURN;
        END IF;

        RETURN QUERY
        SELECT
            i.id, i.alert_rule_id, i.fingerprint, i.status, i.value, i.payload,
            i.occurrence_count, i.first_seen_at, i.last_seen_at, i.last_notified_at,
            i.resolved_at, i.created_at, true, false
        FROM alert_incidents i
        WHERE i.id = saved_id;
        RETURN;
    END IF;

    IF normalized_status = 'firing' THEN
        SELECT i.id INTO existing_id
        FROM alert_incidents i
        WHERE i.tenant_id = resolved_tenant_id
          AND i.fingerprint = normalized_fingerprint
          AND i.status = 'firing'
          AND i.resolved_at IS NULL
        ORDER BY i.last_seen_at DESC
        LIMIT 1;

        IF existing_id IS NOT NULL THEN
            SELECT i.last_notified_at IS NULL
                OR now() - i.last_notified_at >= make_interval(secs => GREATEST(COALESCE(cooldown_seconds, 300), 0))
            INTO notify_now
            FROM alert_incidents i
            WHERE i.id = existing_id;

            UPDATE alert_incidents
            SET value = COALESCE(incident_value, alert_incidents.value),
                payload = COALESCE(incident_payload, '{}'::jsonb),
                occurrence_count = alert_incidents.occurrence_count + 1,
                last_seen_at = now(),
                last_notified_at = CASE WHEN notify_now THEN now() ELSE alert_incidents.last_notified_at END
            WHERE alert_incidents.id = existing_id
            RETURNING alert_incidents.id INTO saved_id;

            RETURN QUERY
            SELECT
                i.id, i.alert_rule_id, i.fingerprint, i.status, i.value, i.payload,
                i.occurrence_count, i.first_seen_at, i.last_seen_at, i.last_notified_at,
                i.resolved_at, i.created_at, true, notify_now
            FROM alert_incidents i
            WHERE i.id = saved_id;
            RETURN;
        END IF;
    END IF;

    INSERT INTO alert_incidents (tenant_id, alert_rule_id, fingerprint, status, value, payload, last_notified_at)
    VALUES (
        resolved_tenant_id,
        rule_id,
        normalized_fingerprint,
        normalized_status,
        COALESCE(incident_value, 0),
        COALESCE(incident_payload, '{}'::jsonb),
        CASE WHEN normalized_status = 'firing' THEN now() ELSE NULL END
    )
    RETURNING alert_incidents.id INTO saved_id;

    RETURN QUERY
    SELECT
        i.id, i.alert_rule_id, i.fingerprint, i.status, i.value, i.payload,
        i.occurrence_count, i.first_seen_at, i.last_seen_at, i.last_notified_at,
        i.resolved_at, i.created_at, false, normalized_status = 'firing'
    FROM alert_incidents i
    WHERE i.id = saved_id;
END;
$$;

GRANT EXECUTE ON FUNCTION record_alert_incident_for_worker(text, uuid, text, text, double precision, jsonb, text, integer) TO dbviz_web;

CREATE OR REPLACE FUNCTION record_alert_notification_for_worker(
    worker_key text,
    rule_id uuid,
    tenant_slug text,
    incident_id uuid,
    notification_contact_kind text,
    notification_contact_target text,
    delivery_status text,
    delivery_status_code integer,
    delivery_error text,
    delivery_payload jsonb
)
RETURNS TABLE(
    id uuid,
    alert_rule_id uuid,
    alert_incident_id uuid,
    contact_kind text,
    contact_target text,
    status text,
    status_code integer,
    error text,
    payload jsonb,
    created_at timestamptz
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    resolved_tenant_id uuid;
    normalized_status text;
    saved_id uuid;
BEGIN
    IF worker_key IS NULL OR worker_key <> COALESCE(current_setting('app.alert_worker_key', true), 'dev-alert-worker-key') THEN
        RAISE EXCEPTION 'invalid alert worker key';
    END IF;

    SELECT tenants.id INTO resolved_tenant_id
    FROM tenants
    WHERE tenants.slug = tenant_slug;

    IF resolved_tenant_id IS NULL THEN
        RAISE EXCEPTION 'tenant not found: %', tenant_slug;
    END IF;

    normalized_status := COALESCE(NULLIF(delivery_status, ''), 'failed');
    IF normalized_status NOT IN ('success', 'failed', 'skipped') THEN
        normalized_status := 'failed';
    END IF;

    INSERT INTO alert_notifications (
        tenant_id,
        alert_rule_id,
        alert_incident_id,
        contact_kind,
        contact_target,
        status,
        status_code,
        error,
        payload
    )
    VALUES (
        resolved_tenant_id,
        rule_id,
        incident_id,
        COALESCE(notification_contact_kind, ''),
        COALESCE(notification_contact_target, ''),
        normalized_status,
        GREATEST(COALESCE(delivery_status_code, 0), 0),
        LEFT(COALESCE(delivery_error, ''), 2000),
        COALESCE(delivery_payload, '{}'::jsonb)
    )
    RETURNING alert_notifications.id INTO saved_id;

    RETURN QUERY
    SELECT
        n.id,
        n.alert_rule_id,
        n.alert_incident_id,
        n.contact_kind,
        n.contact_target,
        n.status,
        n.status_code,
        n.error,
        n.payload,
        n.created_at
    FROM alert_notifications n
    WHERE n.id = saved_id;
END;
$$;

GRANT EXECUTE ON FUNCTION record_alert_notification_for_worker(text, uuid, text, uuid, text, text, text, integer, text, jsonb) TO dbviz_web;

CREATE OR REPLACE FUNCTION sync_current_user(user_subject text, user_email text, user_name text, user_provider text, tenant_slug text, tenant_name text)
RETURNS TABLE(id uuid, tenant_id uuid, subject text, email text, display_name text, provider text, role text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    resolved_tenant_id uuid;
    resolved_user_id uuid;
BEGIN
    IF tenant_slug IS NULL OR tenant_slug = '' THEN
        RAISE EXCEPTION 'tenant slug is required';
    END IF;
    IF user_subject IS NULL OR user_subject = '' THEN
        RAISE EXCEPTION 'user subject is required';
    END IF;

    INSERT INTO tenants (slug, name)
    VALUES (tenant_slug, COALESCE(NULLIF(tenant_name, ''), tenant_slug))
    ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
    RETURNING tenants.id INTO resolved_tenant_id;

    INSERT INTO users (tenant_id, subject, email, display_name, provider, role)
    VALUES (
        resolved_tenant_id,
        user_subject,
        COALESCE(NULLIF(user_email, ''), user_subject),
        COALESCE(user_name, ''),
        COALESCE(NULLIF(user_provider, ''), 'unknown'),
        CASE
            WHEN NOT EXISTS (SELECT 1 FROM users WHERE users.tenant_id = resolved_tenant_id) THEN 'owner'
            ELSE 'viewer'
        END
    )
    ON CONFLICT ON CONSTRAINT users_provider_subject_key DO UPDATE
    SET email = EXCLUDED.email,
        display_name = EXCLUDED.display_name,
        tenant_id = EXCLUDED.tenant_id
    RETURNING users.id INTO resolved_user_id;

    RETURN QUERY
    SELECT u.id, u.tenant_id, u.subject, u.email, u.display_name, u.provider, u.role
    FROM users u
    WHERE u.id = resolved_user_id;
END;
$$;

GRANT EXECUTE ON FUNCTION sync_current_user(text, text, text, text, text, text) TO dbviz_web;

CREATE OR REPLACE FUNCTION current_user_profile(user_subject text, user_provider text)
RETURNS TABLE(id uuid, tenant_id uuid, tenant_slug text, subject text, email text, display_name text, provider text, role text)
LANGUAGE sql STABLE
AS $$
    SELECT u.id, u.tenant_id, t.slug AS tenant_slug, u.subject, u.email, u.display_name, u.provider, u.role
    FROM users u
    JOIN tenants t ON t.id = u.tenant_id
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = user_subject
      AND u.provider = user_provider
      AND u.disabled_at IS NULL
    LIMIT 1;
$$;

CREATE OR REPLACE FUNCTION current_user_preferences(user_subject text, user_provider text)
RETURNS jsonb
LANGUAGE sql STABLE
AS $$
    SELECT COALESCE(u.preferences, '{}'::jsonb)
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = user_subject
      AND u.provider = user_provider
      AND u.disabled_at IS NULL
    LIMIT 1;
$$;

CREATE OR REPLACE FUNCTION save_current_user_preferences(user_subject text, user_provider text, user_preferences jsonb)
RETURNS jsonb
LANGUAGE plpgsql
AS $$
DECLARE
    saved_preferences jsonb;
BEGIN
    UPDATE users
    SET preferences = COALESCE(user_preferences, '{}'::jsonb)
    WHERE users.tenant_id = request_tenant_id()
      AND users.subject = user_subject
      AND users.provider = user_provider
      AND users.disabled_at IS NULL
    RETURNING users.preferences INTO saved_preferences;

    RETURN COALESCE(saved_preferences, '{}'::jsonb);
END;
$$;

CREATE OR REPLACE FUNCTION current_user_has_role(user_subject text, user_provider text, allowed_roles text[])
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT EXISTS (
        SELECT 1
        FROM users u
        WHERE u.tenant_id = request_tenant_id()
          AND u.subject = user_subject
          AND u.provider = user_provider
          AND u.disabled_at IS NULL
          AND u.role = ANY(allowed_roles)
    );
$$;

GRANT EXECUTE ON FUNCTION current_user_profile(text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION current_user_preferences(text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_current_user_preferences(text, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION current_user_has_role(text, text, text[]) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_user_memberships(user_subject text, user_provider text)
RETURNS TABLE(tenant_id uuid, tenant_slug text, tenant_name text, role text)
LANGUAGE sql STABLE
SECURITY DEFINER
SET search_path = public
AS $$
    SELECT t.id, t.slug, t.name, u.role
    FROM users u
    JOIN tenants t ON t.id = u.tenant_id
    WHERE u.subject = user_subject
      AND u.provider = user_provider
      AND u.disabled_at IS NULL
    ORDER BY t.name;
$$;

DROP FUNCTION IF EXISTS list_tenant_members(text, text);
DROP FUNCTION IF EXISTS update_tenant_member_role(text, text, uuid, text);

CREATE OR REPLACE FUNCTION list_tenant_members(actor_subject text, actor_provider text)
RETURNS TABLE(id uuid, email text, display_name text, provider text, role text, disabled_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql STABLE
AS $$
BEGIN
    IF NOT current_user_has_role(actor_subject, actor_provider, ARRAY['owner', 'admin']) THEN
        RAISE EXCEPTION 'insufficient role';
    END IF;

    RETURN QUERY
    SELECT u.id, u.email, u.display_name, u.provider, u.role, u.disabled_at, u.created_at
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
    ORDER BY u.created_at ASC;
END;
$$;

CREATE OR REPLACE FUNCTION update_tenant_member_role(actor_subject text, actor_provider text, member_id uuid, member_role text)
RETURNS TABLE(id uuid, email text, display_name text, provider text, role text, disabled_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    actor_role text;
    target_role text;
    owner_count integer;
BEGIN
    SELECT u.role INTO actor_role
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = actor_subject
      AND u.provider = actor_provider
      AND u.disabled_at IS NULL;

    IF actor_role NOT IN ('owner', 'admin') THEN
        RAISE EXCEPTION 'insufficient role';
    END IF;

    IF member_role NOT IN ('owner', 'admin', 'editor', 'viewer') THEN
        RAISE EXCEPTION 'unsupported member role: %', member_role;
    END IF;

    SELECT u.role INTO target_role
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.id = member_id
      AND u.disabled_at IS NULL;

    IF target_role IS NULL THEN
        RAISE EXCEPTION 'member is not available for tenant';
    END IF;

    IF (target_role = 'owner' OR member_role = 'owner') AND actor_role <> 'owner' THEN
        RAISE EXCEPTION 'owner role changes require owner access';
    END IF;

    SELECT count(*) INTO owner_count
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.role = 'owner'
      AND u.disabled_at IS NULL;

    IF target_role = 'owner' AND member_role <> 'owner' AND owner_count <= 1 THEN
        RAISE EXCEPTION 'cannot remove the last owner';
    END IF;

    UPDATE users
    SET role = member_role
    WHERE users.tenant_id = request_tenant_id()
      AND users.id = member_id;

    RETURN QUERY
    SELECT u.id, u.email, u.display_name, u.provider, u.role, u.disabled_at, u.created_at
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.id = member_id;
END;
$$;

CREATE OR REPLACE FUNCTION deactivate_tenant_member(actor_subject text, actor_provider text, member_id uuid)
RETURNS TABLE(id uuid, email text, display_name text, provider text, role text, disabled_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    actor_role text;
    target_role text;
    owner_count integer;
BEGIN
    SELECT u.role INTO actor_role
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.subject = actor_subject
      AND u.provider = actor_provider
      AND u.disabled_at IS NULL;

    IF actor_role NOT IN ('owner', 'admin') THEN
        RAISE EXCEPTION 'insufficient role';
    END IF;

    SELECT u.role INTO target_role
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.id = member_id
      AND u.disabled_at IS NULL;

    IF target_role IS NULL THEN
        RAISE EXCEPTION 'member is not active for tenant';
    END IF;

    IF target_role = 'owner' AND actor_role <> 'owner' THEN
        RAISE EXCEPTION 'owner deactivation requires owner access';
    END IF;

    SELECT count(*) INTO owner_count
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.role = 'owner'
      AND u.disabled_at IS NULL;

    IF target_role = 'owner' AND owner_count <= 1 THEN
        RAISE EXCEPTION 'cannot deactivate the last owner';
    END IF;

    UPDATE users
    SET disabled_at = now()
    WHERE users.tenant_id = request_tenant_id()
      AND users.id = member_id;

    RETURN QUERY
    SELECT u.id, u.email, u.display_name, u.provider, u.role, u.disabled_at, u.created_at
    FROM users u
    WHERE u.tenant_id = request_tenant_id()
      AND u.id = member_id;
END;
$$;

GRANT EXECUTE ON FUNCTION list_user_memberships(text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_tenant_members(text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION update_tenant_member_role(text, text, uuid, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION deactivate_tenant_member(text, text, uuid) TO dbviz_web;

CREATE OR REPLACE FUNCTION list_tenant_invites()
RETURNS TABLE(id uuid, email text, role text, accepted_at timestamptz, expires_at timestamptz, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT i.id, i.email, i.role, i.accepted_at, i.expires_at, i.created_at
    FROM tenant_invites i
    WHERE i.tenant_id = request_tenant_id()
    ORDER BY i.created_at DESC;
$$;

CREATE OR REPLACE FUNCTION create_tenant_invite(actor_subject text, actor_provider text, invite_email text, invite_role text)
RETURNS TABLE(id uuid, email text, role text, token text, accepted_at timestamptz, expires_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
DECLARE
    saved_id uuid;
BEGIN
    IF NOT current_user_has_role(actor_subject, actor_provider, ARRAY['owner', 'admin']) THEN
        RAISE EXCEPTION 'insufficient role';
    END IF;

    IF invite_role NOT IN ('admin', 'editor', 'viewer') THEN
        RAISE EXCEPTION 'unsupported invite role: %', invite_role;
    END IF;

    INSERT INTO tenant_invites (tenant_id, email, role)
    VALUES (request_tenant_id(), lower(invite_email), invite_role)
    ON CONFLICT ON CONSTRAINT tenant_invites_tenant_id_email_key DO UPDATE
    SET role = EXCLUDED.role,
        token = encode(gen_random_bytes(24), 'hex'),
        accepted_at = NULL,
        expires_at = now() + interval '7 days',
        created_at = now()
    RETURNING tenant_invites.id INTO saved_id;

    RETURN QUERY
    SELECT i.id, i.email, i.role, i.token, i.accepted_at, i.expires_at, i.created_at
    FROM tenant_invites i
    WHERE i.id = saved_id;
END;
$$;

CREATE OR REPLACE FUNCTION delete_tenant_invite(actor_subject text, actor_provider text, invite_id uuid)
RETURNS TABLE(id uuid, email text, role text, accepted_at timestamptz, expires_at timestamptz, created_at timestamptz)
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT current_user_has_role(actor_subject, actor_provider, ARRAY['owner', 'admin']) THEN
        RAISE EXCEPTION 'insufficient role';
    END IF;

    RETURN QUERY
    DELETE FROM tenant_invites i
    WHERE i.id = invite_id
      AND i.tenant_id = request_tenant_id()
    RETURNING i.id, i.email, i.role, i.accepted_at, i.expires_at, i.created_at;
END;
$$;

CREATE OR REPLACE FUNCTION accept_tenant_invite(invite_token text, user_subject text, user_email text, user_name text, user_provider text)
RETURNS TABLE(id uuid, tenant_id uuid, tenant_slug text, subject text, email text, display_name text, provider text, role text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    invite_row tenant_invites%ROWTYPE;
    saved_id uuid;
BEGIN
    IF invite_token IS NULL OR invite_token = '' THEN
        RAISE EXCEPTION 'invite token is required';
    END IF;
    IF user_subject IS NULL OR user_subject = '' THEN
        RAISE EXCEPTION 'user subject is required';
    END IF;

    SELECT i.* INTO invite_row
    FROM tenant_invites i
    WHERE i.token = invite_token
      AND i.accepted_at IS NULL
      AND i.expires_at > now();

    IF invite_row.id IS NULL THEN
        RAISE EXCEPTION 'invite is invalid or expired';
    END IF;

    IF lower(COALESCE(user_email, '')) <> lower(invite_row.email) THEN
        RAISE EXCEPTION 'invite email does not match current user';
    END IF;

    INSERT INTO users (tenant_id, subject, email, display_name, provider, role)
    VALUES (
        invite_row.tenant_id,
        user_subject,
        lower(user_email),
        COALESCE(user_name, ''),
        COALESCE(NULLIF(user_provider, ''), 'unknown'),
        invite_row.role
    )
    ON CONFLICT ON CONSTRAINT users_provider_subject_key DO UPDATE
    SET tenant_id = EXCLUDED.tenant_id,
        email = EXCLUDED.email,
        display_name = EXCLUDED.display_name,
        role = EXCLUDED.role,
        disabled_at = NULL
    RETURNING users.id INTO saved_id;

    UPDATE tenant_invites
    SET accepted_at = now()
    WHERE tenant_invites.id = invite_row.id;

    RETURN QUERY
    SELECT u.id, u.tenant_id, t.slug AS tenant_slug, u.subject, u.email, u.display_name, u.provider, u.role
    FROM users u
    JOIN tenants t ON t.id = u.tenant_id
    WHERE u.id = saved_id;
END;
$$;

GRANT EXECUTE ON FUNCTION list_tenant_invites() TO dbviz_web;
GRANT EXECUTE ON FUNCTION create_tenant_invite(text, text, text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION delete_tenant_invite(text, text, uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION accept_tenant_invite(text, text, text, text, text) TO dbviz_web;
NOTIFY pgrst, 'reload schema';
