import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import * as echarts from 'echarts';
import {
  AlertIncident,
  AlertRule,
  ContactEndpoint,
  DataSource,
  Dataset,
  Dashboard,
  DashboardChart,
  Principal,
  Provider,
  PublicConfig,
  QueryHistory,
  QueryRow,
  SavedQuery,
  TenantInvite,
  TenantMember,
  TenantMembership,
  UserProfile,
  apiGet,
  apiPost,
  clearToken,
  getActiveTenant,
  getToken,
  randomVerifier,
  setActiveTenant as storeActiveTenant,
  setToken,
  sha256base64url
} from './api';
import './style.css';

type QueryState = {
  dataset: string;
  sourceId: string;
  groupBy: string;
  measure: string;
  aggregation: string;
  from: string;
  to: string;
};

function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null);
  const [user, setUser] = useState<Principal | null>(null);
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [activeTenant, setActiveTenantState] = useState(getActiveTenant());
  const [memberships, setMemberships] = useState<TenantMembership[]>([]);
  const [members, setMembers] = useState<TenantMember[]>([]);
  const [rows, setRows] = useState<QueryRow[]>([]);
  const [error, setError] = useState('');
  const [tokenInput, setTokenInput] = useState('');
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>([]);
  const [dataSources, setDataSources] = useState<DataSource[]>([]);
  const [queryHistory, setQueryHistory] = useState<QueryHistory[]>([]);
  const [editingDashboardId, setEditingDashboardId] = useState('');
  const [dashboardName, setDashboardName] = useState('Sample Observability');
  const [dashboardPanels, setDashboardPanels] = useState<DashboardChart[]>([]);
  const [panelTitle, setPanelTitle] = useState('Log volume');
  const [panelVisualization, setPanelVisualization] = useState<'line' | 'bar' | 'area'>('line');
  const [editingSavedQueryId, setEditingSavedQueryId] = useState('');
  const [savedQueryName, setSavedQueryName] = useState('Current query');
  const [savedQueryDescription, setSavedQueryDescription] = useState('');
  const [editingSourceId, setEditingSourceId] = useState('');
  const [sourceName, setSourceName] = useState('Default ClickHouse');
  const [sourceURL, setSourceURL] = useState('http://clickhouse:8123');
  const [sourceDatabase, setSourceDatabase] = useState('default');
  const [sourceUser, setSourceUser] = useState('default');
  const [sourceSecretRef, setSourceSecretRef] = useState('clickhouse-default');
  const [sourceStatus, setSourceStatus] = useState('');
  const [alertRules, setAlertRules] = useState<AlertRule[]>([]);
  const [contacts, setContacts] = useState<ContactEndpoint[]>([]);
  const [incidents, setIncidents] = useState<AlertIncident[]>([]);
  const [invites, setInvites] = useState<TenantInvite[]>([]);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState<TenantInvite['role']>('viewer');
  const [inviteToken, setInviteToken] = useState('');
  const [alertName, setAlertName] = useState('High signal volume');
  const [alertThreshold, setAlertThreshold] = useState('100');
  const [contactName, setContactName] = useState('Primary webhook');
  const [contactTarget, setContactTarget] = useState('');
  const [contactKind, setContactKind] = useState<ContactEndpoint['kind']>('webhook');
  const [selectedContact, setSelectedContact] = useState('');
  const [query, setQuery] = useState<QueryState>(() => {
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    return { dataset: 'logs', sourceId: '', groupBy: 'service_name', measure: '_rows', aggregation: 'count', from: toInput(from), to: toInput(to) };
  });

  useEffect(() => {
    apiGet<PublicConfig>('/api/config').then(setConfig).catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    handleOIDCCallback().catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    if (!getToken()) return;
    apiGet<Principal>('/api/me').then((principal) => {
      setUser(principal);
      if (!getActiveTenant()) selectTenant(principal.tenantId);
      apiPost('/api/session/sync', {}).then(() => loadAccessState()).catch(() => undefined);
    }).catch(() => clearToken());
  }, []);

  useEffect(() => {
    if (!config?.devMode || user) return;
    apiGet<Principal>('/api/me').then((principal) => {
      setUser(principal);
      if (!getActiveTenant()) selectTenant(principal.tenantId);
      apiPost('/api/session/sync', {}).then(() => loadAccessState()).catch(() => undefined);
    }).catch(() => undefined);
  }, [config, user]);

  const dataset = useMemo(() => config?.datasets.find((item) => item.id === query.dataset), [config, query.dataset]);

  useEffect(() => {
    if (!config || !user) return;
    loadAccessState().catch((err) => setError(err.message));
    loadDataSources().catch((err) => setError(err.message));
    loadQueryHistory().catch(() => undefined);
    loadSavedQueries().catch(() => undefined);
    loadDashboards().catch((err) => setError(err.message));
    loadAlertState().catch((err) => setError(err.message));
    loadInvites().catch(() => undefined);
    loadMembers().catch(() => setMembers([]));
  }, [config, user, activeTenant]);

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
    const principal = await apiGet<Principal>('/api/me');
    setUser(principal);
    if (!getActiveTenant()) selectTenant(principal.tenantId);
    await apiPost('/api/session/sync', {}).catch(() => undefined);
    await loadAccessState().catch(() => undefined);
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
    const result = await apiPost<{ rows: QueryRow[] }>('/api/query', queryPayload());
    setRows(result.rows);
    loadQueryHistory().catch(() => undefined);
  }

  async function loadDataSources() {
    const result = await apiGet<DataSource[]>('/api/data-sources');
    setDataSources(result);
    if (result[0] && !editingSourceId) {
      fillDataSource(result[0]);
      if (!query.sourceId) setQuery((current) => ({ ...current, sourceId: result[0].id }));
    }
  }

  async function saveDataSource() {
    const saved = await apiPost<DataSource[]>('/api/data-sources', {
      id: editingSourceId || null,
      name: sourceName,
      kind: 'clickhouse',
      config: {
        url: sourceURL,
        database: sourceDatabase,
        username: sourceUser,
        passwordSecretRef: sourceSecretRef
      }
    });
    setDataSources((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    if (saved[0]) {
      setEditingSourceId(saved[0].id);
      setQuery((current) => ({ ...current, sourceId: saved[0].id }));
    }
  }

  async function testDataSource() {
    setSourceStatus('');
    if (!editingSourceId) {
      setSourceStatus('Save source before testing');
      return;
    }
    const result = await apiPost<{ ok: boolean; durationMs: number }>('/api/data-sources/test', { id: editingSourceId });
    setSourceStatus(result.ok ? `Connected in ${result.durationMs} ms` : 'Connection failed');
  }

  async function loadQueryHistory() {
    const result = await apiGet<QueryHistory[]>('/api/query/history');
    setQueryHistory(result);
  }

  async function loadSavedQueries() {
    const result = await apiGet<SavedQuery[]>('/api/saved-queries');
    setSavedQueries(result);
  }

  async function loadDashboards() {
    const result = await apiGet<Dashboard[]>('/api/dashboards');
    setDashboards(result);
  }

  async function loadAccessState() {
    const [nextMemberships, nextProfile] = await Promise.all([
      apiGet<TenantMembership[]>('/api/session/memberships'),
      apiGet<UserProfile>('/api/session/profile')
    ]);
    setMemberships(nextMemberships);
    setProfile(nextProfile);
  }

  async function loadMembers() {
    const result = await apiGet<TenantMember[]>('/api/members');
    setMembers(result);
  }

  async function saveDashboard() {
    const panels = dashboardPanels.length > 0 ? dashboardPanels : [currentDashboardPanel()];
    const layout = {
      version: 1,
      charts: panels
    };
    const saved = await apiPost<Dashboard[]>('/api/dashboards', {
      id: editingDashboardId || null,
      name: dashboardName,
      layout
    });
    setDashboards((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    if (saved[0]) {
      setEditingDashboardId(saved[0].id);
      setDashboardPanels(saved[0].layout?.charts || panels);
    }
  }

  async function saveSavedQuery() {
    const saved = await apiPost<SavedQuery[]>('/api/saved-queries', {
      id: editingSavedQueryId || null,
      name: savedQueryName,
      description: savedQueryDescription,
      query: queryPayload()
    });
    setSavedQueries((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    if (saved[0]) {
      setEditingSavedQueryId(saved[0].id);
      setSavedQueryName(saved[0].name);
      setSavedQueryDescription(saved[0].description || '');
    }
  }

  function addPanelToDashboard() {
    const panel = currentDashboardPanel();
    setDashboardPanels((current) => [...current, panel]);
    setPanelTitle(defaultPanelTitle());
  }

  async function loadAlertState() {
    const [rules, endpoints, recentIncidents] = await Promise.all([
      apiGet<AlertRule[]>('/api/alerts/rules'),
      apiGet<ContactEndpoint[]>('/api/alerts/contacts'),
      apiGet<AlertIncident[]>('/api/alerts/incidents')
    ]);
    setAlertRules(rules);
    setContacts(endpoints);
    setIncidents(recentIncidents);
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
      query: queryPayload(),
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

  async function resolveIncident(incident: AlertIncident) {
    const saved = await apiPost<AlertIncident[]>('/api/alerts/incidents/resolve', { id: incident.id });
    setIncidents((current) => current.map((item) => item.id === incident.id ? (saved[0] || item) : item));
  }

  async function loadInvites() {
    const result = await apiGet<TenantInvite[]>('/api/invites');
    setInvites(result);
  }

  async function createInvite() {
    const saved = await apiPost<TenantInvite[]>('/api/invites', {
      email: inviteEmail,
      role: inviteRole
    });
    setInvites((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    setInviteEmail('');
  }

  async function acceptInvite() {
    const accepted = await apiPost<UserProfile[]>('/api/invites/accept', { token: inviteToken });
    if (accepted[0]?.tenant_slug) {
      selectTenant(accepted[0].tenant_slug);
      setProfile(accepted[0]);
    }
    setInviteToken('');
    await loadAccessState().catch(() => undefined);
  }

  async function updateMemberRole(member: TenantMember, role: TenantMember['role']) {
    const saved = await apiPost<TenantMember[]>('/api/members/role', { id: member.id, role });
    setMembers((current) => current.map((item) => item.id === member.id ? (saved[0] || item) : item));
  }

  function selectTenant(tenant: string) {
    storeActiveTenant(tenant);
    setActiveTenantState(tenant);
  }

  function openDashboard(dashboard: Dashboard) {
    setEditingDashboardId(dashboard.id);
    setDashboardName(dashboard.name);
    const charts = dashboard.layout?.charts || [];
    setDashboardPanels(charts);
    if (charts[0]) {
      setPanelTitle(charts[0].title || defaultPanelTitle());
      setPanelVisualization((charts[0].visualization?.type as 'line' | 'bar' | 'area') || 'line');
      applyQuery(charts[0].query);
    }
  }

  function openSavedQuery(savedQuery: SavedQuery) {
    setEditingSavedQueryId(savedQuery.id);
    setSavedQueryName(savedQuery.name);
    setSavedQueryDescription(savedQuery.description || '');
    applyQuery(savedQuery.query);
  }

  function openPanel(panel: DashboardChart) {
    setPanelTitle(panel.title || defaultPanelTitle());
    setPanelVisualization((panel.visualization?.type as 'line' | 'bar' | 'area') || 'line');
    applyQuery(panel.query);
  }

  function removePanel(index: number) {
    setDashboardPanels((current) => current.filter((_, itemIndex) => itemIndex !== index));
  }

  function fillDataSource(source: DataSource) {
    setEditingSourceId(source.id);
    setQuery((current) => ({ ...current, sourceId: source.id }));
    setSourceStatus('');
    setSourceName(source.name);
    setSourceURL(String(source.config.url || ''));
    setSourceDatabase(String(source.config.database || 'default'));
    setSourceUser(String(source.config.username || 'default'));
    setSourceSecretRef(String(source.config.passwordSecretRef || ''));
  }

  function openHistory(history: QueryHistory) {
    applyQuery(history.query);
  }

  function applyQuery(payload: unknown) {
    const savedQuery = editableQuery(payload as Partial<QueryState>);
    if (savedQuery.dataset) {
      setQuery((current) => ({ ...current, ...savedQuery }));
    }
  }

  function queryPayload(): QueryState {
    return {
      ...query,
      sourceId: query.sourceId || '',
      from: new Date(query.from).toISOString(),
      to: new Date(query.to).toISOString()
    };
  }

  function defaultPanelTitle() {
    return `${dataset?.name || query.dataset} by ${query.groupBy || 'all'}`;
  }

  function currentDashboardPanel(): DashboardChart {
    return {
      id: crypto.randomUUID(),
      title: panelTitle.trim() || defaultPanelTitle(),
      query: queryPayload(),
      visualization: { type: panelVisualization }
    };
  }

  function saveToken() {
    setToken(tokenInput.trim());
    apiGet<Principal>('/api/me').then((principal) => {
      setUser(principal);
      if (!getActiveTenant()) selectTenant(principal.tenantId);
      apiPost('/api/session/sync', {}).then(() => loadAccessState()).catch(() => undefined);
    }).catch((err) => setError(err.message));
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
            <span>{profile?.role || 'member'} in {profile?.tenant_slug || activeTenant || user.tenantId}</span>
            <select value={activeTenant || user.tenantId} onChange={(event) => selectTenant(event.target.value)}>
              {memberships.length === 0 && <option value={activeTenant || user.tenantId}>{activeTenant || user.tenantId}</option>}
              {memberships.map((membership) => (
                <option key={membership.tenant_slug} value={membership.tenant_slug}>
                  {membership.tenant_name} ({membership.role})
                </option>
              ))}
            </select>
            <button onClick={() => { clearToken(); storeActiveTenant(''); setUser(null); setProfile(null); }}>Sign out</button>
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
          <h2>Sources</h2>
          <label>
            Name
            <input value={sourceName} onChange={(event) => setSourceName(event.target.value)} />
          </label>
          <label>
            URL
            <input value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} />
          </label>
          <label>
            Database
            <input value={sourceDatabase} onChange={(event) => setSourceDatabase(event.target.value)} />
          </label>
          <label>
            User
            <input value={sourceUser} onChange={(event) => setSourceUser(event.target.value)} />
          </label>
          <label>
            Secret
            <input value={sourceSecretRef} onChange={(event) => setSourceSecretRef(event.target.value)} />
          </label>
          <button disabled={!user || !sourceURL} onClick={saveDataSource}>Save source</button>
          <button disabled={!user || !editingSourceId} onClick={testDataSource}>Test source</button>
          {sourceStatus && <small>{sourceStatus}</small>}
          <div className="dashboard-list">
            {dataSources.map((source) => (
              <button key={source.id} onClick={() => fillDataSource(source)}>{source.name} - {source.kind}</button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Query</h2>
          <label>
            Source
            <select value={query.sourceId} onChange={(event) => {
              const selected = dataSources.find((source) => source.id === event.target.value);
              setQuery({ ...query, sourceId: event.target.value });
              if (selected) fillDataSource(selected);
            }}>
              <option value="">Server default</option>
              {dataSources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}
            </select>
          </label>
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
          <h2>History</h2>
          <div className="history-list">
            {queryHistory.slice(0, 8).map((history) => (
              <button className={history.status === 'failed' ? 'history failed' : 'history'} key={history.id} onClick={() => openHistory(history)}>
                <strong>{history.dataset}</strong>
                <span>{history.rows_count} rows - {history.duration_ms} ms</span>
                <small>{new Date(history.created_at).toLocaleString()}</small>
              </button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Saved Queries</h2>
          <label>
            Name
            <input value={savedQueryName} onChange={(event) => setSavedQueryName(event.target.value)} />
          </label>
          <label>
            Description
            <textarea value={savedQueryDescription} onChange={(event) => setSavedQueryDescription(event.target.value)} />
          </label>
          <button disabled={!user || !savedQueryName} onClick={saveSavedQuery}>Save query</button>
          <div className="dashboard-list">
            {savedQueries.map((savedQuery) => (
              <button key={savedQuery.id} onClick={() => openSavedQuery(savedQuery)}>
                {savedQuery.name}
              </button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Dashboards</h2>
          <label>
            Name
            <input value={dashboardName} onChange={(event) => setDashboardName(event.target.value)} />
          </label>
          <label>
            Panel title
            <input value={panelTitle} onChange={(event) => setPanelTitle(event.target.value)} />
          </label>
          <label>
            Visualization
            <select value={panelVisualization} onChange={(event) => setPanelVisualization(event.target.value as 'line' | 'bar' | 'area')}>
              <option value="line">Line</option>
              <option value="area">Area</option>
              <option value="bar">Bar</option>
            </select>
          </label>
          <button disabled={!user} onClick={addPanelToDashboard}>Add panel</button>
          <button disabled={!user || !dashboardName} onClick={saveDashboard}>Save dashboard</button>
          <div className="panel-list">
            {dashboardPanels.map((panel, index) => (
              <div className="dashboard-panel" key={panel.id || `${panel.title}-${index}`}>
                <button onClick={() => openPanel(panel)}>{panel.title}</button>
                <button aria-label={`Remove ${panel.title}`} onClick={() => removePanel(index)}>Remove</button>
              </div>
            ))}
          </div>
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

        <section className="panel">
          <h2>Incidents</h2>
          <div className="incident-list">
            {incidents.slice(0, 8).map((incident) => (
              <div className="incident" key={incident.id}>
                <strong>{incident.status}</strong>
                <span>{incident.value} x{incident.occurrence_count || 1}</span>
                <small>{new Date(incident.last_seen_at || incident.created_at).toLocaleString()}</small>
                {incident.status === 'firing' && (
                  <button onClick={() => resolveIncident(incident)}>Resolve</button>
                )}
              </div>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Invites</h2>
          <label>
            Accept token
            <input value={inviteToken} onChange={(event) => setInviteToken(event.target.value)} />
          </label>
          <button disabled={!user || !inviteToken} onClick={acceptInvite}>Accept invite</button>
          <label>
            Email
            <input value={inviteEmail} onChange={(event) => setInviteEmail(event.target.value)} />
          </label>
          <label>
            Role
            <select value={inviteRole} onChange={(event) => setInviteRole(event.target.value as TenantInvite['role'])}>
              <option value="viewer">Viewer</option>
              <option value="editor">Editor</option>
              <option value="admin">Admin</option>
            </select>
          </label>
          <button disabled={!user || !inviteEmail} onClick={createInvite}>Create invite</button>
          <div className="dashboard-list">
            {invites.map((invite) => (
              <button key={invite.id}>{invite.email} - {invite.role}{invite.token ? ` - ${invite.token}` : ''}</button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Members</h2>
          <div className="member-list">
            {members.map((member) => (
              <div className="member" key={member.id}>
                <div>
                  <strong>{member.display_name || member.email}</strong>
                  <small>{member.provider}</small>
                </div>
                <select value={member.role} onChange={(event) => updateMemberRole(member, event.target.value as TenantMember['role'])}>
                  <option value="owner">Owner</option>
                  <option value="admin">Admin</option>
                  <option value="editor">Editor</option>
                  <option value="viewer">Viewer</option>
                </select>
              </div>
            ))}
          </div>
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
        <Chart rows={rows} type={panelVisualization} />
      </section>
    </main>
  );
}

function Chart({ rows, type }: { rows: QueryRow[]; type: 'line' | 'bar' | 'area' }) {
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
        type: type === 'bar' ? 'bar' : 'line',
        areaStyle: type === 'area' ? {} : undefined,
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
  }, [rows, type]);
  return <div className="chart" ref={ref} />;
}

function editableQuery(payload: Partial<QueryState>): Partial<QueryState> {
  const next = { ...payload };
  if (typeof next.from === 'string') {
    const from = new Date(next.from);
    if (!Number.isNaN(from.getTime())) next.from = toInput(from);
  }
  if (typeof next.to === 'string') {
    const to = new Date(next.to);
    if (!Number.isNaN(to.getTime())) next.to = toInput(to);
  }
  return next;
}

function firstDimension(config: PublicConfig | null, datasetID: string): string {
  return config?.datasets.find((item) => item.id === datasetID)?.dimensions[0] || '';
}

function toInput(date: Date): string {
  return date.toISOString().slice(0, 16);
}

createRoot(document.getElementById('root')!).render(<App />);
