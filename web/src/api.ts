export type Provider = {
  id: string;
  name: string;
  issuer: string;
  clientId: string;
  scopes: string[];
  enabled: boolean;
};

export type Dataset = {
  id: string;
  name: string;
  table: string;
  timeColumn: string;
  tenantColumn: string;
  dimensions: string[];
  filters: string[];
  filterOperators: Record<string, string[]>;
  measures: string[];
  aggregations: string[];
  defaultMeasure: string;
  defaultAggregation: string;
  maxLookbackHours: number;
  maxRows: number;
};

export type PublicConfig = {
  providers: Provider[];
  datasets: Dataset[];
  devMode: boolean;
};

export type Principal = {
  subject: string;
  tenantId: string;
  email: string;
  name: string;
  provider: string;
};

export type UserProfile = {
  id: string;
  tenant_id: string;
  tenant_slug: string;
  subject: string;
  email: string;
  display_name: string;
  provider: string;
  role: 'owner' | 'admin' | 'editor' | 'viewer';
};

export type TenantMembership = {
  tenant_id: string;
  tenant_slug: string;
  tenant_name: string;
  role: 'owner' | 'admin' | 'editor' | 'viewer';
};

export type TenantMember = {
  id: string;
  email: string;
  display_name: string;
  provider: string;
  role: 'owner' | 'admin' | 'editor' | 'viewer';
  disabled_at: string | null;
  created_at: string;
};

export type QueryRow = {
  ts: number;
  series: string;
  value: number;
};

export type DataSource = {
  id: string;
  name: string;
  kind: 'clickhouse';
  config: {
    url?: string;
    database?: string;
    username?: string;
    passwordSecretRef?: string;
    [key: string]: unknown;
  };
  updated_at: string;
  created_at: string;
};

export type QueryHistory = {
  id: string;
  user_email: string;
  dataset: string;
  query: Record<string, unknown>;
  rows_count: number;
  duration_ms: number;
  status: 'success' | 'failed';
  error: string;
  created_at: string;
};

export type AuditEvent = {
  id: string;
  actor_email: string;
  action: string;
  target_type: string;
  target_id: string;
  payload: Record<string, unknown>;
  created_at: string;
};

export type SavedQuery = {
  id: string;
  name: string;
  description: string;
  query: Record<string, unknown>;
  updated_at: string;
  created_at: string;
};

export type DashboardChart = {
  id?: string;
  title: string;
  query: unknown;
  visualization?: {
    type?: 'line' | 'bar' | 'area';
    [key: string]: unknown;
  };
  position?: {
    x?: number;
    y?: number;
    w?: number;
    h?: number;
    [key: string]: unknown;
  };
};

export type Dashboard = {
  id: string;
  name: string;
  layout: {
    version: number;
    charts: DashboardChart[];
  };
  updated_at: string;
  created_at: string;
};

export type ContactEndpoint = {
  id: string;
  name: string;
  kind: 'email' | 'webhook' | 'pagerduty';
  target: string;
  config: Record<string, unknown>;
  created_at: string;
};

export type AlertRule = {
  id: string;
  name: string;
  query: unknown;
  condition: {
    operator: string;
    threshold: number;
    for?: string;
  };
  interval_seconds: number;
  enabled: boolean;
  contact_endpoint_id: string | null;
  created_at: string;
};

export type AlertIncident = {
  id: string;
  alert_rule_id: string | null;
  fingerprint: string;
  status: 'firing' | 'resolved' | 'notify_failed';
  value: number;
  payload: Record<string, unknown>;
  occurrence_count: number;
  first_seen_at: string;
  last_seen_at: string;
  last_notified_at: string | null;
  resolved_at: string | null;
  created_at: string;
};

export type AlertNotification = {
  id: string;
  alert_rule_id: string | null;
  alert_incident_id: string | null;
  contact_kind: string;
  contact_target: string;
  status: 'success' | 'failed' | 'skipped';
  status_code: number;
  error: string;
  payload: Record<string, unknown>;
  created_at: string;
};

export type TenantInvite = {
  id: string;
  email: string;
  role: 'admin' | 'editor' | 'viewer';
  token?: string;
  accepted_at: string | null;
  expires_at: string;
  created_at: string;
};

const tokenKey = 'uvoo-dbviz-token';
const tenantKey = 'uvoo-dbviz-active-tenant';
const devAuthPausedKey = 'uvoo-dbviz-dev-auth-paused';

export function getToken(): string {
  const legacy = localStorage.getItem(tokenKey);
  if (legacy) localStorage.removeItem(tokenKey);
  return sessionStorage.getItem(tokenKey) || '';
}

export function setToken(token: string) {
  localStorage.removeItem(tokenKey);
  sessionStorage.setItem(tokenKey, token);
}

export function clearToken() {
  sessionStorage.removeItem(tokenKey);
  localStorage.removeItem(tokenKey);
}

export function isDevAuthPaused(): boolean {
  return sessionStorage.getItem(devAuthPausedKey) === 'true';
}

export function pauseDevAuth() {
  sessionStorage.setItem(devAuthPausedKey, 'true');
}

export function resumeDevAuth() {
  sessionStorage.removeItem(devAuthPausedKey);
}

export function getActiveTenant(): string {
  return localStorage.getItem(tenantKey) || '';
}

export function setActiveTenant(tenant: string) {
  if (tenant) localStorage.setItem(tenantKey, tenant);
  else localStorage.removeItem(tenantKey);
}

export async function apiGet<T>(path: string): Promise<T> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const tenant = getActiveTenant();
  if (tenant) headers['X-DBViz-Tenant'] = tenant;
  const res = await fetch(path, { headers });
  if (!res.ok) throw new Error(await errorText(res));
  return res.json();
}

export async function apiPost<T>(path: string, body: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const tenant = getActiveTenant();
  if (tenant) headers['X-DBViz-Tenant'] = tenant;
  const res = await fetch(path, { method: 'POST', headers, body: JSON.stringify(body) });
  if (!res.ok) throw new Error(await errorText(res));
  return res.json();
}

async function errorText(res: Response): Promise<string> {
  try {
    const parsed = await res.json();
    return parsed.error || res.statusText;
  } catch {
    return res.statusText;
  }
}

export async function sha256base64url(value: string): Promise<string> {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return base64url(new Uint8Array(digest));
}

export function randomVerifier(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return base64url(bytes);
}

function base64url(bytes: Uint8Array): string {
  let raw = '';
  for (const byte of bytes) raw += String.fromCharCode(byte);
  return btoa(raw).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}
