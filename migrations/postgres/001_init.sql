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
    status text NOT NULL DEFAULT 'firing' CHECK (status IN ('firing', 'resolved', 'notify_failed')),
    value double precision NOT NULL DEFAULT 0,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS users_tenant_id_idx ON users(tenant_id);
CREATE INDEX IF NOT EXISTS tenant_invites_tenant_id_idx ON tenant_invites(tenant_id);
CREATE INDEX IF NOT EXISTS dashboards_tenant_id_idx ON dashboards(tenant_id);
CREATE INDEX IF NOT EXISTS charts_dashboard_id_idx ON charts(dashboard_id);
CREATE INDEX IF NOT EXISTS alert_rules_enabled_idx ON alert_rules(tenant_id, enabled);
CREATE INDEX IF NOT EXISTS alert_incidents_tenant_created_idx ON alert_incidents(tenant_id, created_at DESC);

GRANT USAGE ON SCHEMA public TO dbviz_web;
GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, users, tenant_invites, data_sources, dashboards, charts, contact_endpoints, alert_rules, alert_incidents TO dbviz_web;

INSERT INTO tenants (slug, name)
VALUES ('dev', 'Development')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO dashboards (tenant_id, name, layout)
SELECT id, 'Sample Observability', '{"version":1,"charts":[{"title":"Log volume","dataset":"logs","groupBy":"service_name"}]}'::jsonb
FROM tenants
WHERE slug = 'dev'
  AND NOT EXISTS (
      SELECT 1
      FROM dashboards
      WHERE dashboards.tenant_id = tenants.id
        AND dashboards.name = 'Sample Observability'
  )
ON CONFLICT DO NOTHING;

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE dashboards ENABLE ROW LEVEL SECURITY;
ALTER TABLE charts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_endpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_incidents ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION request_tenant_id() RETURNS uuid
LANGUAGE sql STABLE AS $$
    WITH claims AS (
        SELECT
            NULLIF(current_setting('request.jwt.claim.tenant_id', true), '') AS tenant_id_claim,
            COALESCE(
                NULLIF(current_setting('request.jwt.claim.tenant_key', true), ''),
                NULLIF(current_setting('request.jwt.claim.tenant_slug', true), ''),
                NULLIF(current_setting('request.jwt.claim.hd', true), ''),
                NULLIF(current_setting('request.jwt.claim.tid', true), ''),
                NULLIF(NULLIF(current_setting('request.headers', true), '')::json->>'x-dev-tenant', '')
            ) AS tenant_slug_claim
    )
    SELECT CASE
        WHEN tenant_id_claim ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
            THEN tenant_id_claim::uuid
        WHEN tenant_slug_claim IS NOT NULL
            THEN (SELECT id FROM tenants WHERE slug = tenant_slug_claim)
        ELSE NULL
    END
    FROM claims
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

DROP POLICY IF EXISTS charts_tenant_isolation ON charts;
CREATE POLICY charts_tenant_isolation ON charts
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

DROP POLICY IF EXISTS contact_endpoints_tenant_isolation ON contact_endpoints;
CREATE POLICY contact_endpoints_tenant_isolation ON contact_endpoints
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

GRANT EXECUTE ON FUNCTION list_dashboards() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_dashboard(uuid, text, jsonb) TO dbviz_web;

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

CREATE OR REPLACE FUNCTION list_alert_rules()
RETURNS TABLE(id uuid, name text, query jsonb, condition jsonb, interval_seconds integer, enabled boolean, contact_endpoint_id uuid, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT a.id, a.name, a.query, a.condition, a.interval_seconds, a.enabled, a.contact_endpoint_id, a.created_at
    FROM alert_rules a
    WHERE a.tenant_id = request_tenant_id()
    ORDER BY a.created_at DESC;
$$;

CREATE OR REPLACE FUNCTION list_alert_incidents(incident_limit integer DEFAULT 100)
RETURNS TABLE(id uuid, alert_rule_id uuid, status text, value double precision, payload jsonb, created_at timestamptz)
LANGUAGE sql STABLE
AS $$
    SELECT i.id, i.alert_rule_id, i.status, i.value, i.payload, i.created_at
    FROM alert_incidents i
    WHERE i.tenant_id = request_tenant_id()
    ORDER BY i.created_at DESC
    LIMIT LEAST(GREATEST(COALESCE(incident_limit, 100), 1), 500);
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

GRANT EXECUTE ON FUNCTION list_contact_endpoints() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_contact_endpoint(uuid, text, text, text, jsonb) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_alert_rules() TO dbviz_web;
GRANT EXECUTE ON FUNCTION save_alert_rule(uuid, text, jsonb, jsonb, integer, boolean, uuid) TO dbviz_web;
GRANT EXECUTE ON FUNCTION list_alert_incidents(integer) TO dbviz_web;

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

CREATE OR REPLACE FUNCTION record_alert_incident_for_worker(worker_key text, rule_id uuid, tenant_slug text, incident_status text, incident_value double precision, incident_payload jsonb)
RETURNS TABLE(id uuid, alert_rule_id uuid, status text, value double precision, payload jsonb, created_at timestamptz)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
    resolved_tenant_id uuid;
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

    INSERT INTO alert_incidents (tenant_id, alert_rule_id, status, value, payload)
    VALUES (
        resolved_tenant_id,
        rule_id,
        COALESCE(NULLIF(incident_status, ''), 'firing'),
        COALESCE(incident_value, 0),
        COALESCE(incident_payload, '{}'::jsonb)
    )
    RETURNING alert_incidents.id INTO saved_id;

    RETURN QUERY
    SELECT i.id, i.alert_rule_id, i.status, i.value, i.payload, i.created_at
    FROM alert_incidents i
    WHERE i.id = saved_id;
END;
$$;

GRANT EXECUTE ON FUNCTION record_alert_incident_for_worker(text, uuid, text, text, double precision, jsonb) TO dbviz_web;

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
    LIMIT 1;
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
          AND u.role = ANY(allowed_roles)
    );
$$;

GRANT EXECUTE ON FUNCTION current_user_profile(text, text) TO dbviz_web;
GRANT EXECUTE ON FUNCTION current_user_has_role(text, text, text[]) TO dbviz_web;

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

GRANT EXECUTE ON FUNCTION list_tenant_invites() TO dbviz_web;
GRANT EXECUTE ON FUNCTION create_tenant_invite(text, text, text, text) TO dbviz_web;
