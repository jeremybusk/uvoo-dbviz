import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import * as echarts from 'echarts';
import {
  AlertRule,
  ContactEndpoint,
  Dataset,
  Dashboard,
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
  measure: string;
  aggregation: string;
  from: string;
  to: string;
};

function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null);
  const [user, setUser] = useState<Principal | null>(null);
  const [rows, setRows] = useState<QueryRow[]>([]);
  const [error, setError] = useState('');
  const [tokenInput, setTokenInput] = useState('');
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [dashboardName, setDashboardName] = useState('Sample Observability');
  const [alertRules, setAlertRules] = useState<AlertRule[]>([]);
  const [contacts, setContacts] = useState<ContactEndpoint[]>([]);
  const [alertName, setAlertName] = useState('High signal volume');
  const [alertThreshold, setAlertThreshold] = useState('100');
  const [contactName, setContactName] = useState('Primary webhook');
  const [contactTarget, setContactTarget] = useState('');
  const [contactKind, setContactKind] = useState<ContactEndpoint['kind']>('webhook');
  const [selectedContact, setSelectedContact] = useState('');
  const [query, setQuery] = useState<QueryState>(() => {
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    return { dataset: 'logs', groupBy: 'service_name', measure: '_rows', aggregation: 'count', from: toInput(from), to: toInput(to) };
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

  useEffect(() => {
    if (!config || !user) return;
    loadDashboards().catch((err) => setError(err.message));
    loadAlertState().catch((err) => setError(err.message));
  }, [config, user]);

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
      measure: query.measure,
      aggregation: query.aggregation,
      from: new Date(query.from).toISOString(),
      to: new Date(query.to).toISOString()
    });
    setRows(result.rows);
  }

  async function loadDashboards() {
    const result = await apiGet<Dashboard[]>('/api/dashboards');
    setDashboards(result);
  }

  async function saveDashboard() {
    const layout = {
      version: 1,
      charts: [
        {
          title: `${dataset?.name || query.dataset} by ${query.groupBy || 'all'}`,
          query
        }
      ]
    };
    const saved = await apiPost<Dashboard[]>('/api/dashboards', {
      id: null,
      name: dashboardName,
      layout
    });
    setDashboards((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
  }

  async function loadAlertState() {
    const [rules, endpoints] = await Promise.all([
      apiGet<AlertRule[]>('/api/alerts/rules'),
      apiGet<ContactEndpoint[]>('/api/alerts/contacts')
    ]);
    setAlertRules(rules);
    setContacts(endpoints);
    if (!selectedContact && endpoints[0]) setSelectedContact(endpoints[0].id);
  }

  async function saveContact() {
    const saved = await apiPost<ContactEndpoint[]>('/api/alerts/contacts', {
      id: null,
      name: contactName,
      kind: contactKind,
      target: contactTarget,
      config: {}
    });
    setContacts((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    if (saved[0]) setSelectedContact(saved[0].id);
  }

  async function saveAlert() {
    const saved = await apiPost<AlertRule[]>('/api/alerts/rules', {
      id: null,
      name: alertName,
      query,
      condition: {
        operator: 'gt',
        threshold: Number(alertThreshold)
      },
      intervalSeconds: 60,
      enabled: true,
      contactEndpointId: selectedContact || null
    });
    setAlertRules((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
  }

  function openDashboard(dashboard: Dashboard) {
    setDashboardName(dashboard.name);
    const savedQuery = dashboard.layout?.charts?.[0]?.query as Partial<QueryState> | undefined;
    if (savedQuery?.dataset) {
      setQuery((current) => ({ ...current, ...savedQuery }));
    }
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
            <select value={query.dataset} onChange={(event) => {
              const next = config?.datasets.find((item) => item.id === event.target.value);
              setQuery({
                ...query,
                dataset: event.target.value,
                groupBy: firstDimension(config, event.target.value),
                measure: next?.defaultMeasure || '_rows',
                aggregation: next?.defaultAggregation || 'count'
              });
            }}>
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
            Measure
            <select value={query.measure} onChange={(event) => setQuery({ ...query, measure: event.target.value })}>
              {dataset?.measures.map((item) => <option key={item} value={item}>{item}</option>)}
            </select>
          </label>
          <label>
            Aggregation
            <select value={query.aggregation} onChange={(event) => setQuery({ ...query, aggregation: event.target.value })}>
              {dataset?.aggregations.map((item) => <option key={item} value={item}>{item}</option>)}
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

        <section className="panel">
          <h2>Dashboards</h2>
          <label>
            Name
            <input value={dashboardName} onChange={(event) => setDashboardName(event.target.value)} />
          </label>
          <button disabled={!user} onClick={saveDashboard}>Save</button>
          <div className="dashboard-list">
            {dashboards.map((dashboard) => (
              <button key={dashboard.id} onClick={() => openDashboard(dashboard)}>{dashboard.name}</button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Alerts</h2>
          <label>
            Rule
            <input value={alertName} onChange={(event) => setAlertName(event.target.value)} />
          </label>
          <label>
            Threshold
            <input type="number" min="0" value={alertThreshold} onChange={(event) => setAlertThreshold(event.target.value)} />
          </label>
          <label>
            Contact
            <select value={selectedContact} onChange={(event) => setSelectedContact(event.target.value)}>
              <option value="">None</option>
              {contacts.map((contact) => <option key={contact.id} value={contact.id}>{contact.name}</option>)}
            </select>
          </label>
          <button disabled={!user} onClick={saveAlert}>Save rule</button>
          <div className="dashboard-list">
            {alertRules.map((rule) => (
              <button key={rule.id} onClick={() => {
                setAlertName(rule.name);
                setAlertThreshold(String(rule.condition?.threshold ?? 0));
                setSelectedContact(rule.contact_endpoint_id || '');
              }}>{rule.name}</button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Contacts</h2>
          <label>
            Name
            <input value={contactName} onChange={(event) => setContactName(event.target.value)} />
          </label>
          <label>
            Kind
            <select value={contactKind} onChange={(event) => setContactKind(event.target.value as ContactEndpoint['kind'])}>
              <option value="webhook">Webhook</option>
              <option value="pagerduty">PagerDuty</option>
              <option value="email">Email</option>
            </select>
          </label>
          <label>
            Target
            <input value={contactTarget} onChange={(event) => setContactTarget(event.target.value)} />
          </label>
          <button disabled={!user || !contactTarget} onClick={saveContact}>Save contact</button>
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
