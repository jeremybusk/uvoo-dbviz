import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Alert, Button, ConfigProvider, Dropdown, Flex, Input, Layout, Segmented, Select, Space, Spin, Switch, Table, Tabs, Tag, Typography, theme } from 'antd';
import { BulbOutlined, DownOutlined, FilterOutlined, MoonOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import {
  AlertIncident,
  AlertNotification,
  AlertRule,
  AuditEvent,
  ContactEndpoint,
  DataSource,
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
  isDevAuthPaused,
  pauseDevAuth,
  randomVerifier,
  resumeDevAuth,
  setActiveTenant as storeActiveTenant,
  setToken,
  sha256base64url
} from './api';
import {
  AccessSection,
  AlertsSection,
  AuditSection,
  ContactsSection,
  ControlSections,
  DashboardsSection,
  HistorySection,
  IncidentsSection,
  InvitesSection,
  MembersSection,
  NotificationsSection,
  QuerySection,
  SavedQueriesSection,
  SourceSection
} from './components/ControlSections';
import { JwtClaims, QueryState, RelativeRange, RelativeRangeUnit, ThemeMode, VisualizationType } from './types';
import 'antd/dist/reset.css';
import './style.css';

const { Content, Sider } = Layout;
const Chart = React.lazy(() => import('./components/Chart').then((module) => ({ default: module.Chart })));
const primaryColor = '#2563eb';
const secondaryColor = '#64748b';
const themeStorageKey = 'uvoo-dbviz-theme';

function App() {
  const [themeMode, setThemeMode] = useState<ThemeMode>(readThemePreference);
  const [config, setConfig] = useState<PublicConfig | null>(null);
  const [user, setUser] = useState<Principal | null>(null);
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [activeTenant, setActiveTenantState] = useState(getActiveTenant());
  const [memberships, setMemberships] = useState<TenantMembership[]>([]);
  const [members, setMembers] = useState<TenantMember[]>([]);
  const [auditEvents, setAuditEvents] = useState<AuditEvent[]>([]);
  const [rows, setRows] = useState<QueryRow[]>([]);
  const [eventRows, setEventRows] = useState<Record<string, unknown>[]>([]);
  const [activeTab, setActiveTab] = useState('chart');
  const [refreshSeconds, setRefreshSeconds] = useState(0);
  const [lastUpdated, setLastUpdated] = useState('');
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
  const [panelVisualization, setPanelVisualization] = useState<VisualizationType>('line');
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
  const [notifications, setNotifications] = useState<AlertNotification[]>([]);
  const [invites, setInvites] = useState<TenantInvite[]>([]);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState<TenantInvite['role']>('viewer');
  const [inviteToken, setInviteToken] = useState('');
  const [alertName, setAlertName] = useState('High signal volume');
  const [alertThreshold, setAlertThreshold] = useState('100');
  const [alertOperator, setAlertOperator] = useState('gt');
  const [alertInterval, setAlertInterval] = useState(60);
  const [alertEnabled, setAlertEnabled] = useState(true);
  const [alertPreview, setAlertPreview] = useState('');
  const [contactName, setContactName] = useState('Primary webhook');
  const [contactTarget, setContactTarget] = useState('');
  const [contactKind, setContactKind] = useState<ContactEndpoint['kind']>('webhook');
  const [selectedContact, setSelectedContact] = useState('');
  const [relativeRange, setRelativeRange] = useState<RelativeRange>({ value: 1, unit: 'hours' });
  const [query, setQuery] = useState<QueryState>(() => {
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    return { dataset: 'logs', sourceId: '', mode: 'builder', sql: defaultSQL('logs'), groupBy: 'service_name', measure: '_rows', aggregation: 'count', from: toInput(from), to: toInput(to), search: '', limit: 200 };
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
    if (!config?.devMode || user || isDevAuthPaused()) return;
    devLogin();
  }, [config, user]);

  const dataset = useMemo(() => config?.datasets.find((item) => item.id === query.dataset), [config, query.dataset]);
  const jwtClaims = useMemo(() => decodeJwtClaims(getToken()), [user]);

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
    loadAuditEvents().catch(() => setAuditEvents([]));
  }, [config, user, activeTenant]);

  useEffect(() => {
    if (!user || refreshSeconds <= 0) return;
    const interval = window.setInterval(() => {
      const nextQuery = queryWithRelativeRange(query, relativeRange);
      setQuery(nextQuery);
      loadData(nextQuery).catch((err) => setError(err.message));
    }, refreshSeconds * 1000);
    return () => window.clearInterval(interval);
  }, [user, refreshSeconds, query, relativeRange]);

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
    resumeDevAuth();
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

  async function devLogin() {
    resumeDevAuth();
    const principal = await apiGet<Principal>('/api/me');
    setUser(principal);
    if (!getActiveTenant()) selectTenant(principal.tenantId);
    apiPost('/api/session/sync', {}).then(() => loadAccessState()).catch(() => undefined);
  }

  async function loadData(nextQuery: QueryState = query) {
    setError('');
    if ((nextQuery.mode || 'builder') === 'sql') {
      const result = await apiPost<{ rows: Record<string, unknown>[] }>('/api/sql', queryPayload(nextQuery));
      setEventRows(result.rows);
      setRows(chartRowsFromCustomSQL(result.rows));
      setActiveTab('events');
      setLastUpdated(new Date().toISOString());
      loadQueryHistory().catch(() => undefined);
      return;
    }
    const [series, events] = await Promise.all([
      apiPost<{ rows: QueryRow[] }>('/api/query', queryPayload(nextQuery)),
      apiPost<{ rows: Record<string, unknown>[] }>('/api/events', queryPayload(nextQuery))
    ]);
    setRows(series.rows);
    setEventRows(events.rows);
    setLastUpdated(new Date().toISOString());
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
      config: { url: sourceURL, database: sourceDatabase, username: sourceUser, passwordSecretRef: sourceSecretRef }
    });
    setDataSources((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
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
    setQueryHistory(await apiGet<QueryHistory[]>('/api/query/history'));
  }

  async function loadSavedQueries() {
    setSavedQueries(await apiGet<SavedQuery[]>('/api/saved-queries'));
  }

  async function loadDashboards() {
    setDashboards(await apiGet<Dashboard[]>('/api/dashboards'));
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
    setMembers(await apiGet<TenantMember[]>('/api/members'));
  }

  async function loadAuditEvents() {
    setAuditEvents(await apiGet<AuditEvent[]>('/api/audit/events'));
  }

  async function saveDashboard() {
    const panels = dashboardPanels.length > 0 ? dashboardPanels : [currentDashboardPanel()];
    const saved = await apiPost<Dashboard[]>('/api/dashboards', {
      id: editingDashboardId || null,
      name: dashboardName,
      layout: { version: 1, charts: panels }
    });
    setDashboards((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
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
    loadAuditEvents().catch(() => undefined);
    if (saved[0]) {
      setEditingSavedQueryId(saved[0].id);
      setSavedQueryName(saved[0].name);
      setSavedQueryDescription(saved[0].description || '');
    }
  }

  async function saveCurrentView() {
    const name = `${dataset?.name || query.dataset} view ${new Date().toLocaleString()}`;
    const saved = await apiPost<SavedQuery[]>('/api/saved-queries', {
      id: null,
      name,
      description: compactQueryDescription(query),
      query: queryPayload()
    });
    setSavedQueries((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    if (saved[0]) {
      setEditingSavedQueryId(saved[0].id);
      setSavedQueryName(saved[0].name);
      setSavedQueryDescription(saved[0].description || '');
    }
    loadAuditEvents().catch(() => undefined);
  }

  async function loadAlertState() {
    const [rules, endpoints, recentIncidents, recentNotifications] = await Promise.all([
      apiGet<AlertRule[]>('/api/alerts/rules'),
      apiGet<ContactEndpoint[]>('/api/alerts/contacts'),
      apiGet<AlertIncident[]>('/api/alerts/incidents'),
      apiGet<AlertNotification[]>('/api/alerts/notifications')
    ]);
    setAlertRules(rules);
    setContacts(endpoints);
    setIncidents(recentIncidents);
    setNotifications(recentNotifications);
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
    loadAuditEvents().catch(() => undefined);
    if (saved[0]) setSelectedContact(saved[0].id);
  }

  async function saveAlert() {
    const saved = await apiPost<AlertRule[]>('/api/alerts/rules', {
      id: null,
      name: alertName,
      query: queryPayload(),
      condition: { operator: alertOperator, threshold: Number(alertThreshold) },
      intervalSeconds: alertInterval,
      enabled: alertEnabled,
      contactEndpointId: selectedContact || null
    });
    setAlertRules((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
  }

  async function testAlert() {
    setAlertPreview('');
    const result = await apiPost<{ value: number; operator: string; threshold: number; firing: boolean }>('/api/alerts/test', {
      query: queryPayload(),
      condition: { operator: alertOperator, threshold: Number(alertThreshold) }
    });
    setAlertPreview(`${result.firing ? 'Firing' : 'OK'}: value ${formatNumber(result.value)} ${operatorLabel(result.operator)} ${formatNumber(result.threshold)}`);
  }

  async function resolveIncident(incident: AlertIncident) {
    const saved = await apiPost<AlertIncident[]>('/api/alerts/incidents/resolve', { id: incident.id });
    setIncidents((current) => current.map((item) => item.id === incident.id ? (saved[0] || item) : item));
    loadAuditEvents().catch(() => undefined);
  }

  async function loadInvites() {
    setInvites(await apiGet<TenantInvite[]>('/api/invites'));
  }

  async function createInvite() {
    const saved = await apiPost<TenantInvite[]>('/api/invites', { email: inviteEmail, role: inviteRole });
    setInvites((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    setInviteEmail('');
    loadAuditEvents().catch(() => undefined);
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
    loadAuditEvents().catch(() => undefined);
  }

  async function deactivateMember(member: TenantMember) {
    const saved = await apiPost<TenantMember[]>('/api/members/deactivate', { id: member.id });
    setMembers((current) => current.map((item) => item.id === member.id ? (saved[0] || item) : item));
    loadAccessState().catch(() => undefined);
    loadAuditEvents().catch(() => undefined);
  }

  function addPanelToDashboard() {
    setDashboardPanels((current) => [...current, currentDashboardPanel()]);
    setPanelTitle(defaultPanelTitle());
  }

  function selectTenant(tenant: string) {
    storeActiveTenant(tenant);
    setActiveTenantState(tenant);
  }

  function signOut() {
    clearToken();
    pauseDevAuth();
    storeActiveTenant('');
    setUser(null);
    setProfile(null);
  }

  function openDashboard(dashboard: Dashboard) {
    setEditingDashboardId(dashboard.id);
    setDashboardName(dashboard.name);
    const charts = dashboard.layout?.charts || [];
    setDashboardPanels(charts);
    if (charts[0]) openPanel(charts[0]);
  }

  function openSavedQuery(savedQuery: SavedQuery) {
    setEditingSavedQueryId(savedQuery.id);
    setSavedQueryName(savedQuery.name);
    setSavedQueryDescription(savedQuery.description || '');
    applyQuery(savedQuery.query);
  }

  function openSavedView(savedQuery: SavedQuery) {
    const savedQueryPayload = editableQuery(savedQuery.query as Partial<QueryState>);
    if (!savedQueryPayload.dataset) return;
    const nextQuery = { ...query, ...savedQueryPayload } as QueryState;
    setEditingSavedQueryId(savedQuery.id);
    setSavedQueryName(savedQuery.name);
    setSavedQueryDescription(savedQuery.description || '');
    setQuery(nextQuery);
    loadData(nextQuery).catch((err) => setError(err.message));
  }

  function openPanel(panel: DashboardChart) {
    setPanelTitle(panel.title || defaultPanelTitle());
    setPanelVisualization((panel.visualization?.type as VisualizationType) || 'line');
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

  function applyQuery(payload: unknown) {
    const savedQuery = editableQuery(payload as Partial<QueryState>);
    if (savedQuery.dataset) updateQuery((current) => ({ ...current, ...savedQuery }));
  }

  function applyEventFilter(field: string, value: unknown) {
    const term = formatCell(value, field).trim();
    if (!term) return;
    const nextQuery = { ...query, mode: 'builder' as const, search: term };
    setQuery(nextQuery);
    setActiveTab('events');
    loadData(nextQuery).catch((err) => setError(err.message));
  }

  function queryPayload(nextQuery: QueryState = query): QueryState {
    return { ...nextQuery, sourceId: nextQuery.sourceId || '', from: new Date(nextQuery.from).toISOString(), to: new Date(nextQuery.to).toISOString() };
  }

  function changeTheme(nextTheme: ThemeMode) {
    setThemeMode(nextTheme);
    writeThemePreference(nextTheme);
  }

  function applyRelativeRange(value: number, unit: RelativeRangeUnit) {
    const nextRange = { value: Math.max(1, Math.trunc(value || 1)), unit };
    setRelativeRange(nextRange);
    updateQuery((current) => queryWithRelativeRange(current, nextRange));
  }

  function updateQuery(next: QueryState | ((current: QueryState) => QueryState)) {
    setQuery((current) => {
      const candidate = typeof next === 'function' ? next(current) : next;
      if ((candidate.mode || 'builder') === 'sql' && !stringsHaveText(candidate.sql)) {
        return { ...candidate, sql: defaultSQL(candidate.dataset) };
      }
      return candidate;
    });
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
    resumeDevAuth();
    setToken(tokenInput.trim());
    apiGet<Principal>('/api/me').then((principal) => {
      setUser(principal);
      if (!getActiveTenant()) selectTenant(principal.tenantId);
      apiPost('/api/session/sync', {}).then(() => loadAccessState()).catch(() => undefined);
    }).catch((err) => setError(err.message));
  }

  const controlItems = [
    { key: 'access', label: 'Access', children: <AccessSection config={config} user={user} profile={profile} activeTenant={activeTenant} memberships={memberships} jwtClaims={jwtClaims} tokenInput={tokenInput} onTokenInput={setTokenInput} onLogin={login} onSaveToken={saveToken} onDevLogin={() => devLogin().catch((err) => setError(err.message))} onSelectTenant={selectTenant} onSignOut={signOut} /> },
    { key: 'sources', label: 'Sources', children: <SourceSection user={user} dataSources={dataSources} sourceName={sourceName} sourceURL={sourceURL} sourceDatabase={sourceDatabase} sourceUser={sourceUser} sourceSecretRef={sourceSecretRef} sourceStatus={sourceStatus} editingSourceId={editingSourceId} onName={setSourceName} onURL={setSourceURL} onDatabase={setSourceDatabase} onUser={setSourceUser} onSecretRef={setSourceSecretRef} onSave={saveDataSource} onTest={testDataSource} onOpen={fillDataSource} /> },
    { key: 'query', label: 'Query', children: <QuerySection config={config} user={user} query={query} dataset={dataset} dataSources={dataSources} relativeRange={relativeRange} onQuery={updateQuery} onSource={fillDataSource} onRun={() => loadData()} onRelativeRange={applyRelativeRange} /> },
    { key: 'history', label: 'History', children: <HistorySection queryHistory={queryHistory} onOpen={(history) => applyQuery(history.query)} /> },
    { key: 'saved', label: 'Saved Queries', children: <SavedQueriesSection user={user} savedQueries={savedQueries} savedQueryName={savedQueryName} savedQueryDescription={savedQueryDescription} onName={setSavedQueryName} onDescription={setSavedQueryDescription} onSave={saveSavedQuery} onOpen={openSavedQuery} /> },
    { key: 'dashboards', label: 'Dashboards', children: <DashboardsSection user={user} dashboards={dashboards} dashboardPanels={dashboardPanels} dashboardName={dashboardName} panelTitle={panelTitle} panelVisualization={panelVisualization} onDashboardName={setDashboardName} onPanelTitle={setPanelTitle} onPanelVisualization={setPanelVisualization} onAddPanel={addPanelToDashboard} onSave={saveDashboard} onOpen={openDashboard} onOpenPanel={openPanel} onRemovePanel={removePanel} /> },
    { key: 'alerts', label: 'Alerts', children: <AlertsSection user={user} alertRules={alertRules} contacts={contacts} alertName={alertName} alertThreshold={alertThreshold} alertOperator={alertOperator} alertInterval={alertInterval} alertEnabled={alertEnabled} alertPreview={alertPreview} selectedContact={selectedContact} queryMode={query.mode || 'builder'} onName={setAlertName} onThreshold={setAlertThreshold} onOperator={setAlertOperator} onInterval={setAlertInterval} onEnabled={setAlertEnabled} onContact={setSelectedContact} onTest={() => testAlert().catch((err) => setError(err.message))} onSave={saveAlert} /> },
    { key: 'contacts', label: 'Contacts', children: <ContactsSection user={user} contactName={contactName} contactTarget={contactTarget} contactKind={contactKind} onName={setContactName} onTarget={setContactTarget} onKind={setContactKind} onSave={saveContact} /> },
    { key: 'incidents', label: 'Incidents', children: <IncidentsSection incidents={incidents} onResolve={resolveIncident} /> },
    { key: 'notifications', label: 'Notifications', children: <NotificationsSection notifications={notifications} /> },
    { key: 'invites', label: 'Invites', children: <InvitesSection user={user} invites={invites} inviteEmail={inviteEmail} inviteRole={inviteRole} inviteToken={inviteToken} onEmail={setInviteEmail} onRole={setInviteRole} onToken={setInviteToken} onAccept={acceptInvite} onCreate={createInvite} /> },
    { key: 'members', label: 'Members', children: <MembersSection members={members} onRole={updateMemberRole} onDeactivate={deactivateMember} /> },
    { key: 'audit', label: 'Audit', children: <AuditSection auditEvents={auditEvents} /> }
  ];
  const savedViewItems = savedQueries.slice(0, 12).map((savedQuery) => ({
    key: savedQuery.id,
    label: savedQuery.name
  }));

  return (
    <ConfigProvider
      theme={{
        algorithm: themeMode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
        token: {
          borderRadius: 6,
          colorPrimary: primaryColor,
          colorInfo: primaryColor,
          colorLink: primaryColor,
          colorTextSecondary: secondaryColor,
          colorBorder: themeMode === 'dark' ? '#334155' : '#cbd5e1',
          colorBgLayout: themeMode === 'dark' ? '#020617' : '#f8fafc',
          colorBgContainer: themeMode === 'dark' ? '#0f172a' : '#ffffff'
        }
      }}
    >
      <Layout className={`app-shell theme-${themeMode}`}>
        <Sider width={390} className="app-sidebar">
          <Space direction="vertical" size={16} className="full">
            <Flex align="center" gap={12}>
              <div className="mark">U</div>
              <div>
                <Typography.Title level={1}>Uvoo DBViz</Typography.Title>
                <Typography.Text type="secondary">Tenant-aware ClickHouse analytics</Typography.Text>
              </div>
            </Flex>
            <Flex align="center" justify="space-between">
              <Typography.Text type="secondary">Theme</Typography.Text>
              <Switch
                checkedChildren={<MoonOutlined />}
                unCheckedChildren={<BulbOutlined />}
                checked={themeMode === 'dark'}
                onChange={(checked) => changeTheme(checked ? 'dark' : 'light')}
              />
            </Flex>
            <ControlSections items={controlItems} />
          </Space>
        </Sider>
        <Content className="workspace">
          <Flex align="center" justify="space-between" gap={16} wrap="wrap">
            <div>
              <Typography.Title level={2}>Explore</Typography.Title>
              <Typography.Text type="secondary">{dataset?.table || 'Waiting for configuration'}</Typography.Text>
            </div>
            <Flex align="center" gap={8} wrap="wrap">
              <Segmented
                size="small"
                value={refreshSeconds}
                options={[
                  { label: 'Off', value: 0 },
                  { label: '10s', value: 10 },
                  { label: '30s', value: 30 },
                  { label: '1m', value: 60 },
                  { label: '5m', value: 300 }
                ]}
                onChange={(value) => setRefreshSeconds(Number(value))}
              />
              <Dropdown
                trigger={['click']}
                menu={{
                  items: savedViewItems.length > 0 ? savedViewItems : [{ key: 'empty', label: 'No saved views', disabled: true }],
                  onClick: ({ key }) => {
                    const savedQuery = savedQueries.find((item) => item.id === key);
                    if (savedQuery) openSavedView(savedQuery);
                  }
                }}
              >
                <Button>
                  Saved views <DownOutlined />
                </Button>
              </Dropdown>
              <Button icon={<SaveOutlined />} disabled={!user} onClick={() => saveCurrentView().catch((err) => setError(err.message))}>Save view</Button>
              <Button icon={<ReloadOutlined />} disabled={!user} onClick={() => loadData().catch((err) => setError(err.message))}>{rows.length} rows</Button>
            </Flex>
          </Flex>
          {error && <Alert type="error" showIcon closable message={error} onClose={() => setError('')} />}
          <QuerySummary query={query} datasetName={dataset?.name || query.dataset} tenant={activeTenant || user?.tenantId || ''} refreshSeconds={refreshSeconds} lastUpdated={lastUpdated} />
          <LogBreakdown rows={eventRows} onFilter={applyEventFilter} />
          <Tabs
            className="workspace-tabs"
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'chart',
                label: 'Chart',
                children: (
                  <React.Suspense fallback={<div className="chart chart-loading"><Spin /></div>}>
                    <Chart rows={rows} themeMode={themeMode} type={panelVisualization} />
                  </React.Suspense>
                )
              },
              {
                key: 'events',
                label: `Events (${eventRows.length})`,
                children: <EventsTable rows={eventRows} onFilter={applyEventFilter} onTrace={(value) => applyEventFilter('trace_id', value)} />
              }
            ]}
          />
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

function QuerySummary(props: {
  query: QueryState;
  datasetName: string;
  tenant: string;
  refreshSeconds: number;
  lastUpdated: string;
}) {
  return (
    <Flex className="query-summary" align="center" gap={8} wrap="wrap">
      <Tag color="blue">{props.datasetName}</Tag>
      <Tag>{(props.query.mode || 'builder') === 'sql' ? 'SQL' : 'Builder'}</Tag>
      <Tag>{props.tenant || 'default tenant'}</Tag>
      <Tag>{formatRange(props.query.from, props.query.to)}</Tag>
      {props.query.search && <Tag icon={<FilterOutlined />}>{props.query.search}</Tag>}
      <Tag>{props.query.limit} event rows</Tag>
      <Tag>{props.refreshSeconds > 0 ? `refresh ${formatRefresh(props.refreshSeconds)}` : 'manual refresh'}</Tag>
      {props.lastUpdated && <Typography.Text type="secondary">Updated {new Date(props.lastUpdated).toLocaleTimeString()}</Typography.Text>}
    </Flex>
  );
}

function LogBreakdown({ rows, onFilter }: { rows: Record<string, unknown>[]; onFilter: (field: string, value: unknown) => void }) {
  const breakdown = useMemo(() => {
    const severities = countValues(rows, 'severity');
    const services = countValues(rows, 'service_name');
    const hosts = countValues(rows, 'host_name');
    const errors = rows.filter((row) => formatCell(row.severity, 'severity').toLowerCase().includes('error')).length;
    const warnings = rows.filter((row) => formatCell(row.severity, 'severity').toLowerCase().includes('warn')).length;
    return {
      total: rows.length,
      errors,
      warnings,
      services: services.slice(0, 4),
      hosts: hosts.slice(0, 3),
      severities: severities.slice(0, 4)
    };
  }, [rows]);

  return (
    <Flex className="log-breakdown" align="center" gap={8} wrap="wrap">
      <MetricPill label="events" value={breakdown.total} />
      <MetricPill label="errors" value={breakdown.errors} tone={breakdown.errors > 0 ? 'danger' : undefined} />
      <MetricPill label="warnings" value={breakdown.warnings} tone={breakdown.warnings > 0 ? 'warn' : undefined} />
      {breakdown.severities.map(([value, count]) => (
        <Button key={`severity-${value}`} size="small" onClick={() => onFilter('severity', value)}>{value} {count}</Button>
      ))}
      {breakdown.services.map(([value, count]) => (
        <Button key={`service-${value}`} size="small" onClick={() => onFilter('service_name', value)}>{value} {count}</Button>
      ))}
      {breakdown.hosts.map(([value, count]) => (
        <Button key={`host-${value}`} size="small" onClick={() => onFilter('host_name', value)}>{value} {count}</Button>
      ))}
    </Flex>
  );
}

function MetricPill({ label, value, tone }: { label: string; value: number; tone?: 'danger' | 'warn' }) {
  return (
    <span className={`metric-pill ${tone ? `metric-pill-${tone}` : ''}`}>
      <strong>{value}</strong>
      <span>{label}</span>
    </span>
  );
}

function EventsTable({
  rows,
  onFilter,
  onTrace
}: {
  rows: Record<string, unknown>[];
  onFilter: (field: string, value: unknown) => void;
  onTrace: (value: unknown) => void;
}) {
  const [filter, setFilter] = useState('');
  const [visibleColumns, setVisibleColumns] = useState<string[]>([]);
  const allKeys = useMemo(() => Array.from(new Set(rows.flatMap((row) => Object.keys(row)))), [rows]);
  const selectedColumns = visibleColumns.length > 0 ? visibleColumns : allKeys;
  const filteredRows = useMemo(() => {
    const term = filter.trim().toLowerCase();
    if (!term) return rows;
    return rows.filter((row) => Object.values(row).some((value) => formatCell(value, '').toLowerCase().includes(term)));
  }, [filter, rows]);
  const columns = useMemo(() => {
    return selectedColumns.map((key) => ({
      title: key,
      dataIndex: key,
      key,
      width: key === 'body' || key === 'attributes' ? 420 : 170,
      ellipsis: key !== 'body',
      render: (value: unknown) => {
        const display = formatCell(value, key);
        if (!display) return '';
        if (key === 'trace_id') {
          return <Button className="cell-action" type="link" size="small" onClick={() => onTrace(value)}>{display}</Button>;
        }
        if (key === 'service_name' || key === 'severity' || key === 'host_name') {
          return <Button className="cell-action" type="link" size="small" onClick={() => onFilter(key, value)}>{display}</Button>;
        }
        return display;
      }
    }));
  }, [onFilter, onTrace, selectedColumns]);

  return (
    <Space direction="vertical" size={10} className="full">
      <Flex align="center" justify="space-between" gap={12} wrap="wrap">
        <Input.Search
          allowClear
          className="events-filter"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder="Filter loaded rows"
        />
        <Flex align="center" gap={8} wrap="wrap">
          <Select
            mode="multiple"
            className="column-select"
            maxTagCount="responsive"
            value={selectedColumns}
            options={allKeys.map((key) => ({ label: key, value: key }))}
            onChange={(values) => setVisibleColumns(values)}
            placeholder="Columns"
          />
          <Button size="small" onClick={() => setVisibleColumns([])}>All</Button>
          <Typography.Text type="secondary">{filteredRows.length} of {rows.length} rows</Typography.Text>
        </Flex>
      </Flex>
      <Table
        className="events-table"
        size="small"
        rowKey={(_, index) => String(index)}
        columns={columns}
        dataSource={filteredRows}
        pagination={{ pageSize: 25, showSizeChanger: true }}
        scroll={{ x: 'max-content', y: 520 }}
      />
    </Space>
  );
}

function formatCell(value: unknown, key: string) {
  if (value == null) return '';
  if (key === 'ts' && typeof value === 'number') return new Date(value * 1000).toLocaleString();
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function countValues(rows: Record<string, unknown>[], key: string): [string, number][] {
  const counts = new Map<string, number>();
  for (const row of rows) {
    const value = formatCell(row[key], key);
    if (!value) continue;
    counts.set(value, (counts.get(value) || 0) + 1);
  }
  return Array.from(counts.entries()).sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]));
}

function chartRowsFromCustomSQL(rows: Record<string, unknown>[]): QueryRow[] {
  return rows.flatMap((row) => {
    const value = numberFromUnknown(row.value);
    if (value == null) return [];
    const ts = numberFromUnknown(row.ts) ?? Math.floor(Date.now() / 1000);
    return [{
      ts,
      series: formatCell(row.series || row.service_name || row.name || 'sql', 'series'),
      value
    }];
  });
}

function numberFromUnknown(value: unknown): number | null {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null;
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function formatRange(from: string, to: string): string {
  const fromDate = new Date(from);
  const toDate = new Date(to);
  if (Number.isNaN(fromDate.getTime()) || Number.isNaN(toDate.getTime())) return `${from} to ${to}`;
  return `${fromDate.toLocaleString()} to ${toDate.toLocaleString()}`;
}

function formatRefresh(seconds: number): string {
  if (seconds >= 60) return `${seconds / 60}m`;
  return `${seconds}s`;
}

function formatNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(3);
}

function operatorLabel(operator: string): string {
  switch (operator) {
    case 'gte':
      return '>=';
    case 'lt':
      return '<';
    case 'lte':
      return '<=';
    case 'eq':
      return '=';
    default:
      return '>';
  }
}

function compactQueryDescription(query: QueryState): string {
  if ((query.mode || 'builder') === 'sql') {
    return [
      query.dataset,
      'custom sql',
      formatRange(query.from, query.to)
    ].filter(Boolean).join(' - ');
  }
  return [
    query.dataset,
    query.groupBy ? `by ${query.groupBy}` : '',
    query.search ? `search ${query.search}` : '',
    formatRange(query.from, query.to)
  ].filter(Boolean).join(' - ');
}

function stringsHaveText(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

function defaultSQL(datasetID: string) {
  if (datasetID === 'metrics') {
    return 'SELECT service_name, avg(value) AS value\nFROM otel_metrics\nWHERE tenant_id = {tenant:String}\n  AND timestamp >= {from:DateTime}\n  AND timestamp < {to:DateTime}\nGROUP BY service_name\nORDER BY value DESC';
  }
  if (datasetID === 'traces') {
    return 'SELECT service_name, count() AS value\nFROM otel_traces\nWHERE tenant_id = {tenant:String}\n  AND timestamp >= {from:DateTime}\n  AND timestamp < {to:DateTime}\nGROUP BY service_name\nORDER BY value DESC';
  }
  return 'SELECT service_name, severity, count() AS value\nFROM otel_logs\nWHERE tenant_id = {tenant:String}\n  AND timestamp >= {from:DateTime}\n  AND timestamp < {to:DateTime}\nGROUP BY service_name, severity\nORDER BY value DESC';
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

function queryWithRelativeRange(query: QueryState, range: RelativeRange): QueryState {
  const to = new Date();
  const from = subtractRelativeRange(to, range);
  return { ...query, from: toInput(from), to: toInput(to) };
}

function subtractRelativeRange(to: Date, range: RelativeRange): Date {
  const from = new Date(to);
  switch (range.unit) {
    case 'minutes':
      from.setMinutes(from.getMinutes() - range.value);
      return from;
    case 'hours':
      from.setHours(from.getHours() - range.value);
      return from;
    case 'days':
      from.setDate(from.getDate() - range.value);
      return from;
    case 'weeks':
      from.setDate(from.getDate() - range.value * 7);
      return from;
    case 'months':
      from.setMonth(from.getMonth() - range.value);
      return from;
    case 'years':
      from.setFullYear(from.getFullYear() - range.value);
      return from;
  }
}

function decodeJwtClaims(token: string): JwtClaims | null {
  const [, payload] = token.split('.');
  if (!payload) return null;
  try {
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
    const json = decodeURIComponent(Array.from(atob(padded), (char) => `%${char.charCodeAt(0).toString(16).padStart(2, '0')}`).join(''));
    const claims = JSON.parse(json);
    return claims && typeof claims === 'object' && !Array.isArray(claims) ? claims as JwtClaims : null;
  } catch {
    return null;
  }
}

function readThemePreference(): ThemeMode {
  try {
    const stored = localStorage.getItem(themeStorageKey);
    return stored === 'light' || stored === 'dark' ? stored : 'dark';
  } catch {
    return 'dark';
  }
}

function writeThemePreference(nextTheme: ThemeMode) {
  try {
    localStorage.setItem(themeStorageKey, nextTheme);
  } catch {
    return;
  }
}

function toInput(date: Date): string {
  return date.toISOString().slice(0, 16);
}

createRoot(document.getElementById('root')!).render(<App />);
