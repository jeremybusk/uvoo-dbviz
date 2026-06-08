CREATE EXTENSION IF NOT EXISTS pgcrypto;

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

CREATE INDEX IF NOT EXISTS users_tenant_id_idx ON users(tenant_id);
CREATE INDEX IF NOT EXISTS dashboards_tenant_id_idx ON dashboards(tenant_id);
CREATE INDEX IF NOT EXISTS charts_dashboard_id_idx ON charts(dashboard_id);
CREATE INDEX IF NOT EXISTS alert_rules_enabled_idx ON alert_rules(tenant_id, enabled);

INSERT INTO tenants (slug, name)
VALUES ('dev', 'Development')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO dashboards (tenant_id, name, layout)
SELECT id, 'Sample Observability', '{"version":1,"charts":[{"title":"Log volume","dataset":"logs","groupBy":"service_name"}]}'::jsonb
FROM tenants
WHERE slug = 'dev'
ON CONFLICT DO NOTHING;

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE dashboards ENABLE ROW LEVEL SECURITY;
ALTER TABLE charts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contact_endpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION request_tenant_id() RETURNS uuid
LANGUAGE sql STABLE AS $$
    WITH claims AS (
        SELECT
            NULLIF(current_setting('request.jwt.claim.tenant_id', true), '') AS tenant_id_claim,
            COALESCE(
                NULLIF(current_setting('request.jwt.claim.tenant_key', true), ''),
                NULLIF(current_setting('request.jwt.claim.tenant_slug', true), ''),
                NULLIF(current_setting('request.jwt.claim.hd', true), ''),
                NULLIF(current_setting('request.jwt.claim.tid', true), '')
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

CREATE POLICY tenant_self ON tenants
    FOR ALL USING (id = request_tenant_id())
    WITH CHECK (id = request_tenant_id());

CREATE POLICY users_tenant_isolation ON users
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

CREATE POLICY data_sources_tenant_isolation ON data_sources
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

CREATE POLICY dashboards_tenant_isolation ON dashboards
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

CREATE POLICY charts_tenant_isolation ON charts
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

CREATE POLICY contact_endpoints_tenant_isolation ON contact_endpoints
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());

CREATE POLICY alert_rules_tenant_isolation ON alert_rules
    FOR ALL USING (tenant_id = request_tenant_id())
    WITH CHECK (tenant_id = request_tenant_id());
