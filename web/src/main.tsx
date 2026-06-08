import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import * as echarts from 'echarts';
import {
  Dataset,
  Principal,
  Provider,
  PublicConfig,
  QueryRow,
  apiGet,
  apiPost,
  clearToken,
  getToken,
  randomVerifier,
  setToken,
  sha256base64url
} from './api';
import './style.css';

type QueryState = {
  dataset: string;
  groupBy: string;
  from: string;
  to: string;
};

function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null);
  const [user, setUser] = useState<Principal | null>(null);
  const [rows, setRows] = useState<QueryRow[]>([]);
  const [error, setError] = useState('');
  const [tokenInput, setTokenInput] = useState('');
  const [query, setQuery] = useState<QueryState>(() => {
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    return { dataset: 'logs', groupBy: 'service_name', from: toInput(from), to: toInput(to) };
  });

  useEffect(() => {
    apiGet<PublicConfig>('/api/config').then(setConfig).catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    handleOIDCCallback().catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    if (!getToken()) return;
    apiGet<Principal>('/api/me').then(setUser).catch(() => clearToken());
  }, []);

  useEffect(() => {
    if (!config?.devMode || user) return;
    apiGet<Principal>('/api/me').then(setUser).catch(() => undefined);
  }, [config, user]);

  const dataset = useMemo(() => config?.datasets.find((item) => item.id === query.dataset), [config, query.dataset]);

  async function handleOIDCCallback() {
    const params = new URLSearchParams(location.search);
    const code = params.get('code');
    const state = params.get('state');
    if (!code || !state) return;
    const stored = JSON.parse(sessionStorage.getItem(`oidc:${state}`) || '{}');
    if (!stored.provider || !stored.verifier) return;
    const tokens = await apiPost<{ id_token?: string; access_token?: string }>(`/api/oidc/${stored.provider}/exchange`, {
      code,
      redirectUri: location.origin + location.pathname,
      codeVerifier: stored.verifier
    });
    const token = tokens.id_token || tokens.access_token;
    if (!token) throw new Error('OIDC token response did not include a usable token');
    setToken(token);
    history.replaceState(null, '', location.pathname);
    setUser(await apiGet<Principal>('/api/me'));
  }

  async function login(provider: Provider) {
    if (!provider.clientId) {
      setError(`${provider.name} needs a client ID configured on the server`);
      return;
    }
    const discovery = await apiGet<{ authorizationEndpoint: string }>(`/api/oidc/${provider.id}/discovery`);
    const verifier = randomVerifier();
    const challenge = await sha256base64url(verifier);
    const state = randomVerifier();
    sessionStorage.setItem(`oidc:${state}`, JSON.stringify({ provider: provider.id, verifier }));
    const params = new URLSearchParams({
      client_id: provider.clientId,
      response_type: 'code',
      redirect_uri: location.origin + location.pathname,
      scope: provider.scopes.join(' '),
      code_challenge: challenge,
      code_challenge_method: 'S256',
      state
    });
    location.href = `${discovery.authorizationEndpoint}?${params.toString()}`;
  }

  async function loadData() {
    setError('');
    const result = await apiPost<{ rows: QueryRow[] }>('/api/query', {
      dataset: query.dataset,
      groupBy: query.groupBy,
      from: new Date(query.from).toISOString(),
      to: new Date(query.to).toISOString()
    });
    setRows(result.rows);
  }

  function saveToken() {
    setToken(tokenInput.trim());
    apiGet<Principal>('/api/me').then(setUser).catch((err) => setError(err.message));
  }

  return (
    <main className="shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="mark">U</span>
          <div>
            <h1>Uvoo DBViz</h1>
            <p>Tenant-aware ClickHouse analytics</p>
          </div>
        </div>

        {user ? (
          <section className="panel compact">
            <strong>{user.name || user.email}</strong>
            <span>{user.tenantId}</span>
            <button onClick={() => { clearToken(); setUser(null); }}>Sign out</button>
          </section>
        ) : (
          <section className="panel">
            <h2>Access</h2>
            <div className="providers">
              {config?.providers.filter((p) => p.enabled).map((provider) => (
                <button key={provider.id} onClick={() => login(provider)}>{provider.name}</button>
              ))}
            </div>
            <textarea value={tokenInput} onChange={(event) => setTokenInput(event.target.value)} placeholder="Paste an OIDC JWT" />
            <button onClick={saveToken}>Use token</button>
          </section>
        )}

        <section className="panel">
          <h2>Query</h2>
          <label>
            Dataset
            <select value={query.dataset} onChange={(event) => setQuery({ ...query, dataset: event.target.value, groupBy: firstDimension(config, event.target.value) })}>
              {config?.datasets.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
            </select>
          </label>
          <label>
            Group
            <select value={query.groupBy} onChange={(event) => setQuery({ ...query, groupBy: event.target.value })}>
              <option value="">All</option>
              {dataset?.dimensions.map((item) => <option key={item} value={item}>{item}</option>)}
            </select>
          </label>
          <label>
            From
            <input type="datetime-local" value={query.from} onChange={(event) => setQuery({ ...query, from: event.target.value })} />
          </label>
          <label>
            To
            <input type="datetime-local" value={query.to} onChange={(event) => setQuery({ ...query, to: event.target.value })} />
          </label>
          <button disabled={!user} onClick={loadData}>Run</button>
        </section>
      </aside>

      <section className="workspace">
        <div className="toolbar">
          <div>
            <h2>Explore</h2>
            <p>{dataset?.table || 'Waiting for configuration'}</p>
          </div>
          <span>{rows.length} rows</span>
        </div>
        {error && <div className="error">{error}</div>}
        <Chart rows={rows} />
      </section>
    </main>
  );
}

function Chart({ rows }: { rows: QueryRow[] }) {
  const ref = React.useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current, undefined, { renderer: 'canvas' });
    const seriesNames = Array.from(new Set(rows.map((row) => row.series || 'all')));
    chart.setOption({
      grid: { top: 24, right: 24, bottom: 36, left: 54 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'time' },
      yAxis: { type: 'value' },
      series: seriesNames.map((name) => ({
        name,
        type: 'line',
        showSymbol: false,
        data: rows.filter((row) => (row.series || 'all') === name).map((row) => [row.ts * 1000, row.value])
      }))
    });
    const resize = () => chart.resize();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      chart.dispose();
    };
  }, [rows]);
  return <div className="chart" ref={ref} />;
}

function firstDimension(config: PublicConfig | null, datasetID: string): string {
  return config?.datasets.find((item) => item.id === datasetID)?.dimensions[0] || '';
}

function toInput(date: Date): string {
  return date.toISOString().slice(0, 16);
}

createRoot(document.getElementById('root')!).render(<App />);
