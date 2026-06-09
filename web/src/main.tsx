import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Alert, Button, ConfigProvider, Flex, Input, Layout, Space, Spin, Switch, Table, Tabs, Typography, theme } from 'antd';
import { BulbOutlined, MoonOutlined } from '@ant-design/icons';
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
import { QueryState, RelativeRange, RelativeRangeUnit, ThemeMode, VisualizationType } from './types';
import 'antd/dist/reset.css';
import './style.css';

const { Content, Sider } = Layout;
const Chart = React.lazy(() => import('./components/Chart').then((module) => ({ default: module.Chart })));
const primaryColor = '#2563eb';
const secondaryColor = '#64748b';

function App() {
  const [themeMode, setThemeMode] = useState<ThemeMode>('light');
  const [config, setConfig] = useState<PublicConfig | null>(null);
  const [user, setUser] = useState<Principal | null>(null);
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [activeTenant, setActiveTenantState] = useState(getActiveTenant());
  const [memberships, setMemberships] = useState<TenantMembership[]>([]);
  const [members, setMembers] = useState<TenantMember[]>([]);
  const [auditEvents, setAuditEvents] = useState<AuditEvent[]>([]);
  const [rows, setRows] = useState<QueryRow[]>([]);
  const [eventRows, setEventRows] = useState<Record<string, unknown>[]>([]);
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
  const [contactName, setContactName] = useState('Primary webhook');
  const [contactTarget, setContactTarget] = useState('');
  const [contactKind, setContactKind] = useState<ContactEndpoint['kind']>('webhook');
  const [selectedContact, setSelectedContact] = useState('');
  const [relativeRange, setRelativeRange] = useState<RelativeRange>({ value: 1, unit: 'hours' });
  const [query, setQuery] = useState<QueryState>(() => {
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    return { dataset: 'logs', sourceId: '', groupBy: 'service_name', measure: '_rows', aggregation: 'count', from: toInput(from), to: toInput(to), search: '', limit: 200 };
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

  async function loadData() {
    setError('');
    const [series, events] = await Promise.all([
      apiPost<{ rows: QueryRow[] }>('/api/query', queryPayload()),
      apiPost<{ rows: Record<string, unknown>[] }>('/api/events', queryPayload())
    ]);
    setRows(series.rows);
    setEventRows(events.rows);
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
      condition: { operator: 'gt', threshold: Number(alertThreshold) },
      intervalSeconds: 60,
      enabled: true,
      contactEndpointId: selectedContact || null
    });
    setAlertRules((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
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
    if (savedQuery.dataset) setQuery((current) => ({ ...current, ...savedQuery }));
  }

  function queryPayload(): QueryState {
    return { ...query, sourceId: query.sourceId || '', from: new Date(query.from).toISOString(), to: new Date(query.to).toISOString() };
  }

  function applyRelativeRange(value: number, unit: RelativeRangeUnit) {
    const nextRange = { value: Math.max(1, Math.trunc(value || 1)), unit };
    const to = new Date();
    const from = subtractRelativeRange(to, nextRange);
    setRelativeRange(nextRange);
    setQuery((current) => ({ ...current, from: toInput(from), to: toInput(to) }));
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
    { key: 'access', label: 'Access', children: <AccessSection config={config} user={user} profile={profile} activeTenant={activeTenant} memberships={memberships} tokenInput={tokenInput} onTokenInput={setTokenInput} onLogin={login} onSaveToken={saveToken} onDevLogin={() => devLogin().catch((err) => setError(err.message))} onSelectTenant={selectTenant} onSignOut={signOut} /> },
    { key: 'sources', label: 'Sources', children: <SourceSection user={user} dataSources={dataSources} sourceName={sourceName} sourceURL={sourceURL} sourceDatabase={sourceDatabase} sourceUser={sourceUser} sourceSecretRef={sourceSecretRef} sourceStatus={sourceStatus} editingSourceId={editingSourceId} onName={setSourceName} onURL={setSourceURL} onDatabase={setSourceDatabase} onUser={setSourceUser} onSecretRef={setSourceSecretRef} onSave={saveDataSource} onTest={testDataSource} onOpen={fillDataSource} /> },
    { key: 'query', label: 'Query', children: <QuerySection config={config} user={user} query={query} dataset={dataset} dataSources={dataSources} relativeRange={relativeRange} onQuery={setQuery} onSource={fillDataSource} onRun={loadData} onRelativeRange={applyRelativeRange} /> },
    { key: 'history', label: 'History', children: <HistorySection queryHistory={queryHistory} onOpen={(history) => applyQuery(history.query)} /> },
    { key: 'saved', label: 'Saved Queries', children: <SavedQueriesSection user={user} savedQueries={savedQueries} savedQueryName={savedQueryName} savedQueryDescription={savedQueryDescription} onName={setSavedQueryName} onDescription={setSavedQueryDescription} onSave={saveSavedQuery} onOpen={openSavedQuery} /> },
    { key: 'dashboards', label: 'Dashboards', children: <DashboardsSection user={user} dashboards={dashboards} dashboardPanels={dashboardPanels} dashboardName={dashboardName} panelTitle={panelTitle} panelVisualization={panelVisualization} onDashboardName={setDashboardName} onPanelTitle={setPanelTitle} onPanelVisualization={setPanelVisualization} onAddPanel={addPanelToDashboard} onSave={saveDashboard} onOpen={openDashboard} onOpenPanel={openPanel} onRemovePanel={removePanel} /> },
    { key: 'alerts', label: 'Alerts', children: <AlertsSection user={user} alertRules={alertRules} contacts={contacts} alertName={alertName} alertThreshold={alertThreshold} selectedContact={selectedContact} onName={setAlertName} onThreshold={setAlertThreshold} onContact={setSelectedContact} onSave={saveAlert} /> },
    { key: 'contacts', label: 'Contacts', children: <ContactsSection user={user} contactName={contactName} contactTarget={contactTarget} contactKind={contactKind} onName={setContactName} onTarget={setContactTarget} onKind={setContactKind} onSave={saveContact} /> },
    { key: 'incidents', label: 'Incidents', children: <IncidentsSection incidents={incidents} onResolve={resolveIncident} /> },
    { key: 'notifications', label: 'Notifications', children: <NotificationsSection notifications={notifications} /> },
    { key: 'invites', label: 'Invites', children: <InvitesSection user={user} invites={invites} inviteEmail={inviteEmail} inviteRole={inviteRole} inviteToken={inviteToken} onEmail={setInviteEmail} onRole={setInviteRole} onToken={setInviteToken} onAccept={acceptInvite} onCreate={createInvite} /> },
    { key: 'members', label: 'Members', children: <MembersSection members={members} onRole={updateMemberRole} onDeactivate={deactivateMember} /> },
    { key: 'audit', label: 'Audit', children: <AuditSection auditEvents={auditEvents} /> }
  ];

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
                onChange={(checked) => setThemeMode(checked ? 'dark' : 'light')}
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
            <Button>{rows.length} rows</Button>
          </Flex>
          {error && <Alert type="error" showIcon closable message={error} onClose={() => setError('')} />}
          <Tabs
            className="workspace-tabs"
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
                children: <EventsTable rows={eventRows} />
              }
            ]}
          />
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

function EventsTable({ rows }: { rows: Record<string, unknown>[] }) {
  const [filter, setFilter] = useState('');
  const filteredRows = useMemo(() => {
    const term = filter.trim().toLowerCase();
    if (!term) return rows;
    return rows.filter((row) => Object.values(row).some((value) => formatCell(value, '').toLowerCase().includes(term)));
  }, [filter, rows]);
  const columns = useMemo(() => {
    const keys = Array.from(new Set(rows.flatMap((row) => Object.keys(row))));
    return keys.map((key) => ({
      title: key,
      dataIndex: key,
      key,
      width: key === 'body' || key === 'attributes' ? 420 : 170,
      ellipsis: key !== 'body',
      render: (value: unknown) => formatCell(value, key)
    }));
  }, [rows]);

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
        <Typography.Text type="secondary">{filteredRows.length} of {rows.length} rows</Typography.Text>
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

function toInput(date: Date): string {
  return date.toISOString().slice(0, 16);
}

createRoot(document.getElementById('root')!).render(<App />);
