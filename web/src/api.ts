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
};

export type PublicConfig = {
  providers: Provider[];
  datasets: Dataset[];
  postgrest: { url: string };
  devMode: boolean;
};

export type Principal = {
  subject: string;
  tenantId: string;
  email: string;
  name: string;
  provider: string;
};

export type QueryRow = {
  ts: number;
  series: string;
  value: number;
};

const tokenKey = 'uvoo-dbviz-token';

export function getToken(): string {
  return localStorage.getItem(tokenKey) || '';
}

export function setToken(token: string) {
  localStorage.setItem(tokenKey, token);
}

export function clearToken() {
  localStorage.removeItem(tokenKey);
}

export async function apiGet<T>(path: string): Promise<T> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(path, { headers });
  if (!res.ok) throw new Error(await errorText(res));
  return res.json();
}

export async function apiPost<T>(path: string, body: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
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

