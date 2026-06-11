import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Alert, Button, ConfigProvider, Dropdown, Flex, Input, Layout, Modal, Segmented, Select, Space, Spin, Switch, Table, Tabs, Tag, Tooltip, Typography, message, theme } from 'antd';
import { BulbOutlined, ColumnWidthOutlined, CopyOutlined, DeleteOutlined, DownOutlined, EditOutlined, FilterOutlined, MenuFoldOutlined, MenuUnfoldOutlined, MoonOutlined, MoreOutlined, ReloadOutlined, SaveOutlined, UpOutlined } from '@ant-design/icons';
import {
  AlertIncident,
  AlertNotification,
  AlertPreviewResult,
  AlertRule,
  AuditEvent,
  ContactTestResult,
  ContactEndpoint,
  DataSource,
  Dashboard,
  DashboardChart,
  Principal,
  Provider,
  PublicConfig,
  PagerDutySyncResult,
  QueryHistory,
  QueryRow,
  SavedQuery,
  SystemReadiness,
  TenantSecret,
  TenantInvite,
  TenantMember,
  TenantMembership,
  UserPreferences,
  UserProfile,
  apiGet,
  apiPost,
  clearToken,
  consumeForceNextOIDCLogin,
  forceNextOIDCLogin,
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
  SecretsSection,
  SettingsSection,
  SourceSection,
  SystemSection
} from './components/ControlSections';
import { JwtClaims, QueryState, RelativeRange, RelativeRangeUnit, ThemeMode, VisualizationType } from './types';
import 'antd/dist/reset.css';
import './style.css';

const { Content, Sider } = Layout;
const Chart = React.lazy(() => import('./components/Chart').then((module) => ({ default: module.Chart })));
const primaryColor = '#2563eb';
const secondaryColor = '#64748b';
const themeStorageKey = 'uvoo-sqviz-theme';

function App() {
  const [messageApi, messageContext] = message.useMessage();
  const [themeMode, setThemeMode] = useState<ThemeMode>(readThemePreference);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWide, setSidebarWide] = useState(false);
  const [preferencesLoaded, setPreferencesLoaded] = useState(false);
  const [preferencesStatus, setPreferencesStatus] = useState('');
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
  const [dashboardRefreshKey, setDashboardRefreshKey] = useState(0);
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
  const [dashboardSavedSignature, setDashboardSavedSignature] = useState('');
  const [dashboardImportText, setDashboardImportText] = useState('');
  const [editingPanelId, setEditingPanelId] = useState('');
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
  const [editingAlertId, setEditingAlertId] = useState('');
  const [alertConditionType, setAlertConditionType] = useState('numeric_threshold');
  const [alertField, setAlertField] = useState('value');
  const [alertTextValue, setAlertTextValue] = useState('');
  const [alertThreshold, setAlertThreshold] = useState('100');
  const [alertOperator, setAlertOperator] = useState('gt');
  const [alertFor, setAlertFor] = useState('');
  const [alertInterval, setAlertInterval] = useState(60);
  const [alertEnabled, setAlertEnabled] = useState(true);
  const [alertPreview, setAlertPreview] = useState('');
  const [alertPreviewResult, setAlertPreviewResult] = useState<AlertPreviewResult | null>(null);
  const [systemReadiness, setSystemReadiness] = useState<SystemReadiness | null>(null);
  const [secrets, setSecrets] = useState<TenantSecret[]>([]);
  const [editingSecretId, setEditingSecretId] = useState('');
  const [secretName, setSecretName] = useState('');
  const [secretDescription, setSecretDescription] = useState('');
  const [secretValue, setSecretValue] = useState('');
  const [contactName, setContactName] = useState('Primary webhook');
  const [contactTarget, setContactTarget] = useState('');
  const [contactKind, setContactKind] = useState<ContactEndpoint['kind']>('webhook');
  const [contactWebhookTokenSecretRef, setContactWebhookTokenSecretRef] = useState('');
  const [contactWebhookTokenValue, setContactWebhookTokenValue] = useState('');
  const [contactWebhookHeaderName, setContactWebhookHeaderName] = useState('');
  const [contactWebhookHeaderValueSecretRef, setContactWebhookHeaderValueSecretRef] = useState('');
  const [contactWebhookHeaderValue, setContactWebhookHeaderValue] = useState('');
  const [contactWebhookBodyTemplate, setContactWebhookBodyTemplate] = useState('');
  const [contactRoutingKeySecretRef, setContactRoutingKeySecretRef] = useState('');
  const [contactRoutingKeyValue, setContactRoutingKeyValue] = useState('');
  const [contactRestApiKeySecretRef, setContactRestApiKeySecretRef] = useState('');
  const [contactRestApiKeyValue, setContactRestApiKeyValue] = useState('');
  const [contactPagerDutySeverity, setContactPagerDutySeverity] = useState('error');
  const [contactPagerDutySourceField, setContactPagerDutySourceField] = useState('service_name');
  const [contactPagerDutyComponent, setContactPagerDutyComponent] = useState('uvoo-sqviz');
  const [contactPagerDutyGroup, setContactPagerDutyGroup] = useState('observability');
  const [contactPagerDutyClass, setContactPagerDutyClass] = useState('alert');
  const [contactPagerDutyServiceID, setContactPagerDutyServiceID] = useState('');
  const [contactPagerDutyRestSyncEnabled, setContactPagerDutyRestSyncEnabled] = useState(false);
  const [contactPagerDutyAutoSyncEnabled, setContactPagerDutyAutoSyncEnabled] = useState(true);
  const [contactPagerDutySyncInterval, setContactPagerDutySyncInterval] = useState(0);
  const [contactPagerDutyFromEmail, setContactPagerDutyFromEmail] = useState('');
  const [contactPagerDutyApiBaseURL, setContactPagerDutyApiBaseURL] = useState('https://api.pagerduty.com');
  const [editingContactId, setEditingContactId] = useState('');
  const [contactStatus, setContactStatus] = useState('');
  const [selectedContact, setSelectedContact] = useState('');
  const [relativeRange, setRelativeRange] = useState<RelativeRange>({ value: 1, unit: 'hours' });
  const [query, setQuery] = useState<QueryState>(() => {
    const to = new Date();
    const from = new Date(to.getTime() - 60 * 60 * 1000);
    return { dataset: 'logs', sourceId: '', mode: 'builder', sql: defaultSQL('logs'), groupBy: 'service_name', measure: '_rows', aggregation: 'count', from: toInput(from), to: toInput(to), search: '', filters: {}, filterOps: {}, limit: 200 };
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

  useEffect(() => {
    const onUnhandledRejection = (event: PromiseRejectionEvent) => {
      reportError(event.reason);
      event.preventDefault();
    };
    const onWindowError = (event: ErrorEvent) => {
      reportError(event.error || event.message);
    };
    window.addEventListener('unhandledrejection', onUnhandledRejection);
    window.addEventListener('error', onWindowError);
    return () => {
      window.removeEventListener('unhandledrejection', onUnhandledRejection);
      window.removeEventListener('error', onWindowError);
    };
  }, [messageApi]);

  const dataset = useMemo(() => config?.datasets.find((item) => item.id === query.dataset), [config, query.dataset]);
  const jwtClaims = useMemo(() => decodeJwtClaims(getToken()), [user]);

  useEffect(() => {
    if (!config || !user) return;
    setPreferencesLoaded(false);
    loadUserPreferences().catch((err) => {
      setPreferencesLoaded(true);
      setError(err.message);
    });
    loadAccessState().then((nextProfile) => {
      if (canManageTenant(nextProfile)) {
        loadSecrets(nextProfile).catch(() => setSecrets([]));
        loadInvites(nextProfile).catch(() => undefined);
        loadMembers(nextProfile).catch(() => setMembers([]));
        loadAuditEvents(nextProfile).catch(() => setAuditEvents([]));
      } else {
        setSecrets([]);
        setInvites([]);
        setMembers([]);
        setAuditEvents([]);
      }
    }).catch((err) => setError(err.message));
    loadDataSources().catch((err) => setError(err.message));
    loadQueryHistory().catch(() => undefined);
    loadSavedQueries().catch(() => undefined);
    loadDashboards().catch((err) => setError(err.message));
    loadAlertState().catch((err) => setError(err.message));
    loadSystemReadiness().catch(() => setSystemReadiness(null));
  }, [config, user, activeTenant]);

  useEffect(() => {
    if (!user || !preferencesLoaded) return;
    setPreferencesStatus('Saving preferences...');
    const timeout = window.setTimeout(() => {
      saveUserPreferences().catch((err) => {
        setPreferencesStatus('Preferences not saved');
        setError(err.message);
      });
    }, 800);
    return () => window.clearTimeout(timeout);
  }, [user, preferencesLoaded, themeMode, refreshSeconds, relativeRange.value, relativeRange.unit, query.dataset, query.sourceId, query.limit, panelVisualization]);

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
    if (consumeForceNextOIDCLogin()) {
      params.set('prompt', 'login');
      params.set('max_age', '0');
    }
    location.href = `${discovery.authorizationEndpoint}?${params.toString()}`;
  }

  async function devLogin() {
    resumeDevAuth();
    const principal = await apiGet<Principal>('/api/me');
    setUser(principal);
    if (!getActiveTenant()) selectTenant(principal.tenantId);
    apiPost('/api/session/sync', {}).then(() => loadAccessState()).catch(() => undefined);
  }

  function reportError(err: unknown) {
    const text = errorText(err);
    setError(text);
    messageApi.error(text);
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

  async function deleteDataSource(source: DataSource) {
    const deleted = await apiPost<DataSource[]>('/api/data-sources/delete', { id: source.id });
    setDataSources((current) => current.filter((item) => item.id !== source.id));
    if (editingSourceId === source.id) newDataSource();
    if (query.sourceId === source.id) setQuery((current) => ({ ...current, sourceId: '' }));
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) setError('Data source was not deleted. It may already be gone or belong to another tenant.');
    else messageApi.success('Data source deleted');
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
    return nextProfile;
  }

  async function loadUserPreferences() {
    const preferences = await apiGet<UserPreferences>('/api/session/preferences');
    applyUserPreferences(preferences);
    setPreferencesLoaded(true);
    setPreferencesStatus('Preferences loaded');
  }

  async function saveUserPreferences() {
    const saved = await apiPost<UserPreferences>('/api/session/preferences', currentUserPreferences());
    setPreferencesStatus('Preferences saved');
    return saved;
  }

  async function loadSecrets(nextProfile: UserProfile | null = profile) {
    if (!canManageTenant(nextProfile)) {
      setSecrets([]);
      return;
    }
    setSecrets(await apiGet<TenantSecret[]>('/api/secrets'));
  }

  async function loadMembers(nextProfile: UserProfile | null = profile) {
    if (!canManageTenant(nextProfile)) {
      setMembers([]);
      return;
    }
    setMembers(await apiGet<TenantMember[]>('/api/members'));
  }

  async function loadAuditEvents(nextProfile: UserProfile | null = profile) {
    if (!canManageTenant(nextProfile)) {
      setAuditEvents([]);
      return;
    }
    setAuditEvents(await apiGet<AuditEvent[]>('/api/audit/events'));
  }

  async function saveDashboard() {
    setError('');
    const cleanName = dashboardName.trim();
    if (!cleanName) {
      setError('Dashboard name is required');
      return;
    }
    const panels = normalizeDashboardPanels(dashboardPanels);
    if (panels.length === 0) {
      setError('Add at least one panel before saving the dashboard');
      return;
    }
    const saved = await apiPost<Dashboard[]>('/api/dashboards', {
      id: editingDashboardId || null,
      name: cleanName,
      layout: { version: 1, charts: panels }
    });
    setDashboards((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
    if (saved[0]) {
      setEditingDashboardId(saved[0].id);
      setDashboardName(saved[0].name);
      const savedPanels = normalizeDashboardPanels(saved[0].layout?.charts || panels);
      setDashboardPanels(savedPanels);
      setDashboardSavedSignature(dashboardSignature(saved[0].name, savedPanels));
      if (!editingPanelId && savedPanels[0]?.id) setEditingPanelId(savedPanels[0].id);
      messageApi.success('Dashboard saved');
    }
  }

  async function saveDashboardAsCopy() {
    setError('');
    const cleanName = dashboardName.trim();
    const copyName = cleanName.endsWith(' copy') ? cleanName : `${cleanName || 'Dashboard'} copy`;
    const panels = normalizeDashboardPanels(dashboardPanels);
    if (panels.length === 0) {
      setError('Add at least one panel before saving the dashboard');
      return;
    }
    const saved = await apiPost<Dashboard[]>('/api/dashboards', {
      id: null,
      name: copyName,
      layout: { version: 1, charts: panels.map((panel, index) => normalizeDashboardPanel({ ...panel, id: newClientID() }, index)) }
    });
    setDashboards((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
    if (saved[0]) {
      setEditingDashboardId(saved[0].id);
      setDashboardName(saved[0].name);
      const savedPanels = normalizeDashboardPanels(saved[0].layout?.charts || panels);
      setDashboardPanels(savedPanels);
      setDashboardSavedSignature(dashboardSignature(saved[0].name, savedPanels));
      if (savedPanels[0]?.id) setEditingPanelId(savedPanels[0].id);
      messageApi.success('Dashboard copied');
    }
  }

  async function deleteDashboard(dashboard: Dashboard) {
    const deleted = await apiPost<Dashboard[]>('/api/dashboards/delete', { id: dashboard.id });
    setDashboards((current) => current.filter((item) => item.id !== dashboard.id));
    if (editingDashboardId === dashboard.id) {
      resetDashboardDraft();
    }
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) {
      setError('Dashboard was not deleted. It may already be gone or belong to another tenant.');
    } else {
      messageApi.success('Dashboard deleted');
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

  async function deleteSavedQuery(savedQuery: SavedQuery) {
    const deleted = await apiPost<SavedQuery[]>('/api/saved-queries/delete', { id: savedQuery.id });
    setSavedQueries((current) => current.filter((item) => item.id !== savedQuery.id));
    if (editingSavedQueryId === savedQuery.id) newSavedQuery();
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) setError('Saved query was not deleted. It may already be gone or belong to another tenant.');
    else messageApi.success('Saved query deleted');
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

  async function loadSystemReadiness() {
    setSystemReadiness(await apiGet<SystemReadiness>('/api/system/readiness'));
  }

  async function saveSecret() {
    const saved = await apiPost<TenantSecret[]>('/api/secrets', {
      id: editingSecretId || null,
      name: secretName,
      description: secretDescription,
      value: secretValue
    });
    setSecrets((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id && item.name !== saved[0]?.name)]);
    setSecretValue('');
    if (saved[0]) {
      setEditingSecretId(saved[0].id);
      setSecretName(saved[0].name);
      setSecretDescription(saved[0].description || '');
    }
    loadAuditEvents().catch(() => undefined);
    messageApi.success('Secret saved');
  }

  async function deleteSecret(secret: TenantSecret) {
    const deleted = await apiPost<TenantSecret[]>('/api/secrets/delete', { id: secret.id });
    setSecrets((current) => current.filter((item) => item.id !== secret.id));
    if (editingSecretId === secret.id) newSecret();
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) setError('Secret was not deleted. It may already be gone or belong to another tenant.');
    else messageApi.success('Secret deleted');
  }

  async function saveContact(): Promise<ContactEndpoint | null> {
    const saved = await apiPost<ContactEndpoint[]>('/api/alerts/contacts', {
      id: editingContactId || null,
      name: contactName,
      kind: contactKind,
      target: contactKind === 'pagerduty' && !contactTarget.trim() ? 'https://events.pagerduty.com/v2/enqueue' : contactTarget,
      config: contactConfigPayload()
    });
    setContacts((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    loadAuditEvents().catch(() => undefined);
    if (saved[0]) {
      setEditingContactId(saved[0].id);
      setSelectedContact(saved[0].id);
      setContactWebhookTokenValue('');
      setContactWebhookHeaderValue('');
      setContactWebhookTokenSecretRef(String(saved[0].config.tokenSecretRef || contactWebhookTokenSecretRef));
      setContactWebhookHeaderValueSecretRef(String(saved[0].config.headerValueSecretRef || contactWebhookHeaderValueSecretRef));
      setContactWebhookBodyTemplate(String(saved[0].config.bodyTemplate || contactWebhookBodyTemplate));
      setContactRoutingKeyValue('');
      setContactRestApiKeyValue('');
      setContactRoutingKeySecretRef(String(saved[0].config.routingKeySecretRef || contactRoutingKeySecretRef));
      setContactRestApiKeySecretRef(String(saved[0].config.restApiKeySecretRef || contactRestApiKeySecretRef));
      setContactPagerDutyRestSyncEnabled(String(saved[0].config.restSyncEnabled || '').toLowerCase() === 'true' || contactPagerDutyRestSyncEnabled);
      setContactPagerDutyAutoSyncEnabled(contactAutoSyncEnabled(saved[0].config, contactPagerDutyAutoSyncEnabled));
      setContactPagerDutySyncInterval(contactSyncInterval(saved[0].config, contactPagerDutySyncInterval));
      setContactPagerDutyServiceID(String(saved[0].config.serviceId || contactPagerDutyServiceID));
      setContactPagerDutyFromEmail(String(saved[0].config.fromEmail || contactPagerDutyFromEmail));
      setContactPagerDutyApiBaseURL(String(saved[0].config.apiBaseURL || contactPagerDutyApiBaseURL || 'https://api.pagerduty.com'));
      await loadAccessState().catch(() => undefined);
      return saved[0];
    }
    return null;
  }

  async function testCurrentContact() {
    setContactStatus('');
    const contact = await saveContact();
    if (contact) await testContact(contact);
  }

  async function testContact(contact: ContactEndpoint) {
    return testContactAction(contact, 'trigger');
  }

  async function testContactAction(contact: ContactEndpoint, action: 'trigger' | 'resolve' | 'validate') {
    setContactStatus('Testing contact...');
    const result = await apiPost<ContactTestResult>('/api/alerts/contacts/test', { id: contact.id, action });
    const statusCode = result.statusCode ? ` HTTP ${result.statusCode}` : '';
    const detail = result.error ? `: ${result.error}` : statusCode;
    const label = action === 'validate' ? 'contact validation' : action === 'resolve' ? 'contact resolve test' : 'contact test';
    const text = `${capitalize(result.status)} ${label}${detail}`;
    setContactStatus(text);
    if (result.status === 'success') messageApi.success(text);
    else if (result.status === 'skipped') messageApi.warning(text);
    else messageApi.error(text);
    loadAlertState().catch(() => undefined);
  }

  async function deleteContact(contact: ContactEndpoint) {
    const deleted = await apiPost<ContactEndpoint[]>('/api/alerts/contacts/delete', { id: contact.id });
    setContacts((current) => current.filter((item) => item.id !== contact.id));
    setAlertRules((current) => current.map((rule) => rule.contact_endpoint_id === contact.id ? { ...rule, contact_endpoint_id: null } : rule));
    if (editingContactId === contact.id) newContact();
    if (selectedContact === contact.id) setSelectedContact('');
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) setError('Contact was not deleted. It may already be gone or belong to another tenant.');
    else messageApi.success('Contact deleted');
  }

  async function saveAlert() {
    const saved = await apiPost<AlertRule[]>('/api/alerts/rules', {
      id: editingAlertId || null,
      name: alertName,
      query: queryPayload(),
      condition: currentAlertCondition(),
      intervalSeconds: alertInterval,
      enabled: alertEnabled,
      contactEndpointId: selectedContact || null
    });
    setAlertRules((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    if (saved[0]) setEditingAlertId(saved[0].id);
    loadAuditEvents().catch(() => undefined);
  }

  async function deleteAlert(rule: AlertRule) {
    const deleted = await apiPost<AlertRule[]>('/api/alerts/rules/delete', { id: rule.id });
    setAlertRules((current) => current.filter((item) => item.id !== rule.id));
    if (editingAlertId === rule.id) newAlertRule();
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) setError('Alert rule was not deleted. It may already be gone or belong to another tenant.');
    else messageApi.success('Alert rule deleted');
  }

  async function toggleAlert(rule: AlertRule) {
    const saved = await apiPost<AlertRule[]>('/api/alerts/rules', {
      id: rule.id,
      name: rule.name,
      query: rule.query,
      condition: normalizeAlertConditionPayload(rule.condition),
      intervalSeconds: rule.interval_seconds || 60,
      enabled: !rule.enabled,
      contactEndpointId: rule.contact_endpoint_id || null
    });
    setAlertRules((current) => current.map((item) => item.id === rule.id ? (saved[0] || item) : item));
    loadAuditEvents().catch(() => undefined);
  }

  async function testAlert() {
    setAlertPreview('');
    setAlertPreviewResult(null);
    const result = await apiPost<AlertPreviewResult>('/api/alerts/test', {
      query: queryPayload(),
      condition: currentAlertCondition()
    });
    const normalizedResult = normalizeAlertPreviewResult(result);
    setAlertPreviewResult(normalizedResult);
    setAlertPreview(alertPreviewText(normalizedResult));
  }

  async function resolveIncident(incident: AlertIncident) {
    const saved = await apiPost<AlertIncident[]>('/api/alerts/incidents/resolve', { id: incident.id });
    setIncidents((current) => current.map((item) => item.id === incident.id ? (saved[0] || item) : item));
    await loadAlertState();
    loadAuditEvents().catch(() => undefined);
  }

  async function acknowledgeIncident(incident: AlertIncident) {
    const saved = await apiPost<AlertIncident[]>('/api/alerts/incidents/acknowledge', { id: incident.id });
    setIncidents((current) => current.map((item) => item.id === incident.id ? (saved[0] || item) : item));
    await loadAlertState();
    loadAuditEvents().catch(() => undefined);
  }

  async function syncPagerDutyIncidents() {
    const result = await apiPost<PagerDutySyncResult>('/api/alerts/pagerduty/sync', {});
    await loadAlertState();
    loadAuditEvents().catch(() => undefined);
    const syncResults = Array.isArray(result.results) ? result.results : [];
    const failed = syncResults.filter((item) => item.Status === 'failed').length;
    if (result.count === 0) {
      messageApi.info(result.message || 'No mapped PagerDuty incidents found');
    } else if (failed > 0) {
      messageApi.warning(`PagerDuty sync finished with ${failed} failed of ${result.count}`);
    } else {
      messageApi.success(`PagerDuty sync checked ${result.count} incident${result.count === 1 ? '' : 's'}`);
    }
  }

  async function loadInvites(nextProfile: UserProfile | null = profile) {
    if (!canManageTenant(nextProfile)) {
      setInvites([]);
      return;
    }
    setInvites(await apiGet<TenantInvite[]>('/api/invites'));
  }

  async function createInvite() {
    const saved = await apiPost<TenantInvite[]>('/api/invites', { email: inviteEmail, role: inviteRole });
    setInvites((current) => [...saved, ...current.filter((item) => item.id !== saved[0]?.id)]);
    setInviteEmail('');
    loadAuditEvents().catch(() => undefined);
  }

  async function deleteInvite(invite: TenantInvite) {
    const deleted = await apiPost<TenantInvite[]>('/api/invites/delete', { id: invite.id });
    setInvites((current) => current.filter((item) => item.id !== invite.id));
    loadAuditEvents().catch(() => undefined);
    if (deleted.length === 0) setError('Invite was not deleted. It may already be gone or belong to another tenant.');
    else messageApi.success('Invite deleted');
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
    const panel = currentDashboardPanel();
    setDashboardPanels((current) => [...current, panel]);
    setEditingPanelId(panel.id || '');
    setActiveTab('dashboard');
    setPanelTitle(defaultPanelTitle());
    messageApi.success('Panel added');
  }

  function updateSelectedPanel() {
    if (!editingPanelId) {
      addPanelToDashboard();
      return;
    }
    if (!dashboardPanels.some((panel) => panel.id === editingPanelId)) {
      addPanelToDashboard();
      return;
    }
    setDashboardPanels((current) => current.map((panel) => panel.id === editingPanelId ? currentDashboardPanel(panel) : panel));
    setActiveTab('dashboard');
    messageApi.success('Panel updated');
  }

  function duplicatePanel(index: number) {
    const source = dashboardPanels[index];
    if (!source) return;
    const copy = normalizeDashboardPanel({
      ...source,
      id: newClientID(),
      title: `${source.title || 'Panel'} copy`,
      position: { ...(source.position || {}), x: 0, y: index + 1 }
    }, index + 1);
    const next = [...dashboardPanels.slice(0, index + 1), copy, ...dashboardPanels.slice(index + 1)].map((panel, itemIndex) => ({
      ...panel,
      position: { ...(panel.position || {}), y: itemIndex }
    }));
    setDashboardPanels(next);
    openPanel(next[index + 1] || copy);
    setActiveTab('dashboard');
    messageApi.success('Panel copied');
  }

  function resizePanel(panelID: string, width: number) {
    setDashboardPanels((current) => current.map((panel) => panel.id === panelID ? {
      ...panel,
      position: { ...(panel.position || {}), w: Math.max(1, Math.min(2, width)) }
    } : panel));
  }

  function movePanel(index: number, direction: -1 | 1) {
    const targetIndex = index + direction;
    if (targetIndex < 0 || targetIndex >= dashboardPanels.length) return;
    setDashboardPanels((current) => {
      const next = [...current];
      const [panel] = next.splice(index, 1);
      next.splice(targetIndex, 0, panel);
      return next.map((item, itemIndex) => ({
        ...item,
        position: { ...(item.position || {}), y: itemIndex }
      }));
    });
  }

  function newDashboard() {
    if (!confirmDiscardDashboardChanges()) return;
    resetDashboardDraft();
    messageApi.success('New dashboard started');
  }

  function resetDashboardDraft() {
    setEditingDashboardId('');
    setDashboardName('Untitled Dashboard');
    setDashboardPanels([]);
    setDashboardSavedSignature('');
    setEditingPanelId('');
    setPanelTitle(defaultPanelTitle());
    setPanelVisualization('line');
    setActiveTab('dashboard');
  }

  function discardDashboardChanges() {
    if (editingDashboardId) {
      const saved = dashboards.find((dashboard) => dashboard.id === editingDashboardId);
      if (saved) {
        applyDashboard(saved);
        messageApi.success('Dashboard changes discarded');
        return;
      }
    }
    resetDashboardDraft();
    messageApi.success('Dashboard draft cleared');
  }

  function duplicateDashboard(dashboard: Dashboard) {
    if (!confirmDiscardDashboardChanges()) return;
    const charts = normalizeDashboardPanels(dashboard.layout?.charts || []).map((panel, index) => normalizeDashboardPanel({
      ...panel,
      id: newClientID(),
      position: { ...(panel.position || {}), y: index }
    }, index));
    setEditingDashboardId('');
    setDashboardName(`${dashboard.name} copy`);
    setDashboardPanels(charts);
    setDashboardSavedSignature('');
    setActiveTab('dashboard');
    if (charts[0]) openPanel(charts[0]);
    else setEditingPanelId('');
    messageApi.success('Dashboard copied');
  }

  function exportDashboard() {
    const payload = {
      version: 1,
      name: dashboardName.trim() || 'Dashboard',
      layout: {
        version: 1,
        charts: normalizeDashboardPanels(dashboardPanels)
      }
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${safeFileName(payload.name)}.dashboard.json`;
    anchor.click();
    URL.revokeObjectURL(url);
    messageApi.success('Dashboard exported');
  }

  function importDashboard() {
    setError('');
    try {
      if (!confirmDiscardDashboardChanges()) return;
      const parsed = JSON.parse(dashboardImportText) as { name?: unknown; layout?: { charts?: unknown }; charts?: unknown };
      const charts = Array.isArray(parsed.layout?.charts)
        ? parsed.layout.charts
        : Array.isArray(parsed.charts)
          ? parsed.charts
          : [];
      if (charts.length === 0) {
        throw new Error('Imported dashboard does not contain panels');
      }
      const importedPanels = normalizeDashboardPanels(charts as DashboardChart[]);
      setEditingDashboardId('');
      setDashboardName(typeof parsed.name === 'string' && parsed.name.trim() ? parsed.name.trim() : 'Imported Dashboard');
      setDashboardPanels(importedPanels);
      setDashboardSavedSignature('');
      setDashboardImportText('');
      setActiveTab('dashboard');
      if (importedPanels[0]) openPanel(importedPanels[0]);
      messageApi.success('Dashboard imported');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Dashboard JSON could not be imported';
      setError(message);
      messageApi.error(message);
    }
  }

  function selectTenant(tenant: string) {
    storeActiveTenant(tenant);
    setActiveTenantState(tenant);
  }

  function signOut() {
    clearToken();
    forceNextOIDCLogin();
    pauseDevAuth();
    storeActiveTenant('');
    setUser(null);
    setProfile(null);
  }

  function openDashboard(dashboard: Dashboard) {
    if (!confirmDiscardDashboardChanges()) return;
    applyDashboard(dashboard);
  }

  function applyDashboard(dashboard: Dashboard) {
    setEditingDashboardId(dashboard.id);
    setDashboardName(dashboard.name);
    const charts = normalizeDashboardPanels(dashboard.layout?.charts || []);
    setDashboardPanels(charts);
    setDashboardSavedSignature(dashboardSignature(dashboard.name, charts));
    setActiveTab('dashboard');
    if (charts[0]) openPanel(charts[0]);
    else setEditingPanelId('');
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

  function newAlertRule() {
    setEditingAlertId('');
    setAlertName('High signal volume');
    setAlertConditionType('numeric_threshold');
    setAlertField('value');
    setAlertTextValue('');
    setAlertOperator('gt');
    setAlertThreshold('100');
    setAlertFor('');
    setAlertInterval(60);
    setAlertEnabled(true);
    setSelectedContact('');
    setAlertPreview('');
    setAlertPreviewResult(null);
  }

  function newSavedQuery() {
    setEditingSavedQueryId('');
    setSavedQueryName('Current query');
    setSavedQueryDescription('');
  }

  function newDataSource() {
    setEditingSourceId('');
    setSourceName('Default ClickHouse');
    setSourceURL('http://clickhouse:8123');
    setSourceDatabase('default');
    setSourceUser('default');
    setSourceSecretRef('clickhouse-default');
    setSourceStatus('');
  }

  function newContact() {
    setEditingContactId('');
    setContactName('Primary webhook');
    setContactKind('webhook');
    setContactTarget('');
    setContactWebhookTokenSecretRef('');
    setContactWebhookTokenValue('');
    setContactWebhookHeaderName('');
    setContactWebhookHeaderValueSecretRef('');
    setContactWebhookHeaderValue('');
    setContactWebhookBodyTemplate('');
    setContactRoutingKeySecretRef('');
    setContactRoutingKeyValue('');
    setContactRestApiKeySecretRef('');
    setContactRestApiKeyValue('');
    setContactPagerDutySeverity('error');
    setContactPagerDutySourceField('service_name');
    setContactPagerDutyComponent('uvoo-sqviz');
    setContactPagerDutyGroup('observability');
    setContactPagerDutyClass('alert');
    setContactPagerDutyServiceID('');
    setContactPagerDutyRestSyncEnabled(false);
    setContactPagerDutyAutoSyncEnabled(true);
    setContactPagerDutySyncInterval(0);
    setContactPagerDutyFromEmail('');
    setContactPagerDutyApiBaseURL('https://api.pagerduty.com');
    setContactStatus('');
  }

  function newSecret() {
    setEditingSecretId('');
    setSecretName('');
    setSecretDescription('');
    setSecretValue('');
  }

  function openSecret(secret: TenantSecret) {
    setEditingSecretId(secret.id);
    setSecretName(secret.name);
    setSecretDescription(secret.description || '');
    setSecretValue('');
  }

  function openContact(contact: ContactEndpoint) {
    setEditingContactId(contact.id);
    setContactName(contact.name);
    setContactKind(contact.kind);
    setContactTarget(contact.target);
    setContactWebhookTokenSecretRef(String(contact.config.tokenSecretRef || ''));
    setContactWebhookTokenValue('');
    setContactWebhookHeaderName(String(contact.config.headerName || ''));
    setContactWebhookHeaderValueSecretRef(String(contact.config.headerValueSecretRef || ''));
    setContactWebhookHeaderValue('');
    setContactWebhookBodyTemplate(String(contact.config.bodyTemplate || ''));
    setContactRoutingKeySecretRef(String(contact.config.routingKeySecretRef || ''));
    setContactRoutingKeyValue('');
    setContactRestApiKeySecretRef(String(contact.config.restApiKeySecretRef || ''));
    setContactRestApiKeyValue('');
    setContactPagerDutySeverity(String(contact.config.severity || 'error'));
    setContactPagerDutySourceField(String(contact.config.sourceField || 'service_name'));
    setContactPagerDutyComponent(String(contact.config.component || 'uvoo-sqviz'));
    setContactPagerDutyGroup(String(contact.config.group || 'observability'));
    setContactPagerDutyClass(String(contact.config.class || 'alert'));
    setContactPagerDutyServiceID(String(contact.config.serviceId || ''));
    setContactPagerDutyRestSyncEnabled(String(contact.config.restSyncEnabled || '').toLowerCase() === 'true');
    setContactPagerDutyAutoSyncEnabled(contactAutoSyncEnabled(contact.config, true));
    setContactPagerDutySyncInterval(contactSyncInterval(contact.config, 0));
    setContactPagerDutyFromEmail(String(contact.config.fromEmail || ''));
    setContactPagerDutyApiBaseURL(String(contact.config.apiBaseURL || 'https://api.pagerduty.com'));
    setContactStatus('');
  }

  function useContactForAlert(contact: ContactEndpoint) {
    setSelectedContact(contact.id);
    openContact(contact);
  }

  function openAlertRule(rule: AlertRule) {
    const condition = normalizeAlertConditionPayload(rule.condition);
    setEditingAlertId(rule.id);
    setAlertName(rule.name);
    setAlertConditionType(condition.type || 'numeric_threshold');
    setAlertField(condition.field || defaultAlertField(condition.type || 'numeric_threshold'));
    setAlertTextValue(condition.value || '');
    setAlertOperator(condition.operator || defaultAlertOperator(condition.type || 'numeric_threshold'));
    setAlertThreshold(String(condition.threshold ?? 0));
    setAlertFor(condition.for || '');
    setAlertInterval(rule.interval_seconds || 60);
    setAlertEnabled(rule.enabled);
    setSelectedContact(rule.contact_endpoint_id || '');
    setAlertPreview('');
    loadAlertRuleQuery(rule);
  }

  function loadAlertRuleQuery(rule: AlertRule) {
    const savedQueryPayload = editableQuery(rule.query as Partial<QueryState>);
    if (!savedQueryPayload.dataset) return;
    const nextQuery = { ...query, ...savedQueryPayload } as QueryState;
    updateQuery(nextQuery);
    loadData(nextQuery).catch((err) => setError(err.message));
  }

  function openPanel(panel: DashboardChart) {
    setEditingPanelId(panel.id || '');
    setPanelTitle(panel.title || defaultPanelTitle());
    setPanelVisualization((panel.visualization?.type as VisualizationType) || 'line');
    applyQuery(panel.query);
  }

  function removePanel(index: number) {
    const removed = dashboardPanels[index];
    if (!removed) return;
    const next = dashboardPanels.filter((_, itemIndex) => itemIndex !== index).map((panel, itemIndex) => ({
      ...panel,
      position: { ...(panel.position || {}), y: itemIndex }
    }));
    setDashboardPanels(next);
    if (removed?.id === editingPanelId) {
      const replacement = next[Math.min(index, next.length - 1)];
      if (replacement) openPanel(replacement);
      else setEditingPanelId('');
    }
    messageApi.success('Panel removed');
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
    const allowedFilter = dataset?.filters.includes(field);
    const nextQuery = allowedFilter
      ? { ...query, mode: 'builder' as const, search: '', filters: { ...(query.filters || {}), [field]: term }, filterOps: { ...(query.filterOps || {}), [field]: 'eq' } }
      : { ...query, mode: 'builder' as const, search: term };
    setQuery(nextQuery);
    setActiveTab('events');
    loadData(nextQuery).catch((err) => setError(err.message));
  }

  function queryPayload(nextQuery: QueryState = query): QueryState {
    return apiQueryPayload(nextQuery);
  }

  function currentAlertCondition(): AlertRule['condition'] {
    return {
      type: alertConditionType as AlertRule['condition']['type'],
      operator: alertOperator,
      field: alertField.trim() || defaultAlertField(alertConditionType),
      threshold: Number(alertThreshold),
      value: alertTextValue,
      for: alertFor.trim()
    };
  }

  function contactConfigPayload(): Record<string, unknown> {
    if (contactKind === 'webhook') {
      return {
        tokenSecretRef: contactWebhookTokenSecretRef.trim(),
        tokenValue: contactWebhookTokenValue,
        headerName: contactWebhookHeaderName.trim(),
        headerValueSecretRef: contactWebhookHeaderValueSecretRef.trim(),
        headerValue: contactWebhookHeaderValue,
        bodyTemplate: contactWebhookBodyTemplate.trim()
      };
    }
    if (contactKind !== 'pagerduty') return {};
    return {
      mode: 'events_v2',
      routingKeySecretRef: contactRoutingKeySecretRef.trim(),
      routingKeyValue: contactRoutingKeyValue,
      restApiKeySecretRef: contactRestApiKeySecretRef.trim(),
      restApiKeyValue: contactRestApiKeyValue,
      severity: contactPagerDutySeverity,
      sourceField: contactPagerDutySourceField.trim(),
      component: contactPagerDutyComponent.trim(),
      group: contactPagerDutyGroup.trim(),
      class: contactPagerDutyClass.trim(),
      serviceId: contactPagerDutyServiceID.trim(),
      restSyncEnabled: contactPagerDutyRestSyncEnabled ? 'true' : 'false',
      autoSyncEnabled: contactPagerDutyAutoSyncEnabled ? 'true' : 'false',
      syncIntervalSeconds: contactPagerDutySyncInterval,
      fromEmail: contactPagerDutyFromEmail.trim(),
      apiBaseURL: contactPagerDutyApiBaseURL.trim() || 'https://api.pagerduty.com'
    };
  }

  function changeContactKind(kind: ContactEndpoint['kind']) {
    setContactKind(kind);
    if (kind === 'pagerduty') {
      if (!contactTarget.trim()) setContactTarget('https://events.pagerduty.com/v2/enqueue');
      return;
    }
    if (contactTarget === 'https://events.pagerduty.com/v2/enqueue') setContactTarget('');
  }

  function changeAlertConditionType(nextType: string) {
    setAlertConditionType(nextType);
    setAlertOperator(defaultAlertOperator(nextType));
    setAlertField(defaultAlertField(nextType));
    if (nextType !== 'text_match') setAlertTextValue('');
  }

  function changeTheme(nextTheme: ThemeMode) {
    setThemeMode(nextTheme);
    writeThemePreference(nextTheme);
  }

  function currentUserPreferences(): UserPreferences {
    return {
      themeMode,
      refreshSeconds,
      relativeRange,
      eventLimit: query.limit,
      dataset: query.dataset,
      sourceId: query.sourceId || '',
      visualization: panelVisualization
    };
  }

  function applyUserPreferences(preferences: UserPreferences) {
    if (preferences.themeMode === 'light' || preferences.themeMode === 'dark') {
      changeTheme(preferences.themeMode);
    }
    if (isAllowedRefresh(preferences.refreshSeconds)) {
      setRefreshSeconds(preferences.refreshSeconds);
    }
    if (preferences.relativeRange && isAllowedRangeUnit(preferences.relativeRange.unit)) {
      const value = Math.max(1, Math.min(999, Math.trunc(preferences.relativeRange.value || 1)));
      applyRelativeRange(value, preferences.relativeRange.unit);
    }
    if (isAllowedVisualization(preferences.visualization)) {
      setPanelVisualization(preferences.visualization);
    }
    updateQuery((current) => {
      const datasetID = preferences.dataset && config?.datasets.some((item) => item.id === preferences.dataset)
        ? preferences.dataset
        : current.dataset;
      const sourceID = typeof preferences.sourceId === 'string' ? preferences.sourceId : current.sourceId;
      return {
        ...current,
        dataset: datasetID,
        sourceId: sourceID,
        sql: current.mode === 'sql' && datasetID !== current.dataset ? defaultSQL(datasetID) : current.sql,
        groupBy: datasetID !== current.dataset ? defaultGroupBy(config, datasetID) : current.groupBy,
        measure: datasetID !== current.dataset ? defaultMeasure(config, datasetID) : current.measure,
        aggregation: datasetID !== current.dataset ? defaultAggregation(config, datasetID) : current.aggregation,
        filters: datasetID !== current.dataset ? {} : current.filters,
        filterOps: datasetID !== current.dataset ? {} : current.filterOps,
        limit: clampEventLimit(preferences.eventLimit ?? current.limit)
      };
    });
  }

  function changeDefaultDataset(datasetID: string) {
    updateQuery((current) => ({
      ...current,
      dataset: datasetID,
      sql: current.mode === 'sql' ? defaultSQL(datasetID) : current.sql,
      groupBy: defaultGroupBy(config, datasetID),
      measure: defaultMeasure(config, datasetID),
      aggregation: defaultAggregation(config, datasetID),
      filters: {},
      filterOps: {}
    }));
  }

  function changeDefaultSource(sourceID: string) {
    setQuery((current) => ({ ...current, sourceId: sourceID }));
  }

  function changeDefaultEventLimit(limit: number) {
    updateQuery((current) => ({ ...current, limit: clampEventLimit(limit) }));
  }

  function applyRelativeRange(value: number, unit: RelativeRangeUnit) {
    const nextRange = { value: Math.max(1, Math.trunc(value || 1)), unit };
    setRelativeRange(nextRange);
    updateQuery((current) => queryWithRelativeRange(current, nextRange));
    setDashboardRefreshKey((current) => current + 1);
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

  function currentDashboardPanel(existing?: DashboardChart): DashboardChart {
    return {
      id: existing?.id || newClientID(),
      title: panelTitle.trim() || defaultPanelTitle(),
      query: queryPayload(),
      visualization: { type: panelVisualization },
      position: existing?.position || { x: 0, y: dashboardPanels.length, w: 1, h: 1 }
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

  function confirmDiscardDashboardChanges(): boolean {
    if (!dashboardIsDirty(dashboardName, dashboardPanels, dashboardSavedSignature, editingDashboardId)) return true;
    return window.confirm('Discard unsaved dashboard changes?');
  }

  const dashboardDirty = dashboardIsDirty(dashboardName, dashboardPanels, dashboardSavedSignature, editingDashboardId);
  const secretUsages = useMemo(() => buildSecretUsages(dataSources, contacts), [dataSources, contacts]);
  const failedNotificationCount = notifications.filter((notification) => notification.status === 'failed').length;

  const controlItems = [
    { key: 'access', label: 'Access', children: <AccessSection config={config} user={user} profile={profile} activeTenant={activeTenant} memberships={memberships} jwtClaims={jwtClaims} tokenInput={tokenInput} onTokenInput={setTokenInput} onLogin={login} onSaveToken={saveToken} onDevLogin={() => devLogin().catch((err) => setError(err.message))} onSelectTenant={selectTenant} onSignOut={signOut} /> },
    { key: 'settings', label: 'Settings', children: <SettingsSection user={user} config={config} dataSources={dataSources} preferencesStatus={preferencesStatus} themeMode={themeMode} refreshSeconds={refreshSeconds} relativeRange={relativeRange} eventLimit={query.limit} dataset={query.dataset} sourceId={query.sourceId} visualization={panelVisualization} onTheme={changeTheme} onRefreshSeconds={setRefreshSeconds} onRelativeRange={applyRelativeRange} onEventLimit={changeDefaultEventLimit} onDataset={changeDefaultDataset} onSourceId={changeDefaultSource} onVisualization={setPanelVisualization} onSave={() => saveUserPreferences().catch((err) => setError(err.message))} /> },
    { key: 'status', label: 'Status', children: <SystemSection readiness={systemReadiness} config={config} onRefresh={() => loadSystemReadiness().catch(reportError)} /> },
    { key: 'sources', label: 'Sources', children: <SourceSection user={user} dataSources={dataSources} sourceName={sourceName} sourceURL={sourceURL} sourceDatabase={sourceDatabase} sourceUser={sourceUser} sourceSecretRef={sourceSecretRef} sourceStatus={sourceStatus} editingSourceId={editingSourceId} onName={setSourceName} onURL={setSourceURL} onDatabase={setSourceDatabase} onUser={setSourceUser} onSecretRef={setSourceSecretRef} onNew={newDataSource} onSave={saveDataSource} onTest={testDataSource} onOpen={fillDataSource} onDelete={(source) => deleteDataSource(source).catch((err) => setError(err.message))} /> },
    { key: 'query', label: 'Query', children: <QuerySection config={config} user={user} query={query} dataset={dataset} dataSources={dataSources} relativeRange={relativeRange} onQuery={updateQuery} onSource={fillDataSource} onRun={() => loadData()} onRelativeRange={applyRelativeRange} /> },
    { key: 'history', label: 'History', children: <HistorySection queryHistory={queryHistory} onOpen={(history) => applyQuery(history.query)} /> },
    { key: 'saved', label: 'Saved Queries', children: <SavedQueriesSection user={user} savedQueries={savedQueries} editingSavedQueryId={editingSavedQueryId} savedQueryName={savedQueryName} savedQueryDescription={savedQueryDescription} onName={setSavedQueryName} onDescription={setSavedQueryDescription} onNew={newSavedQuery} onSave={saveSavedQuery} onOpen={openSavedQuery} onDelete={(savedQuery) => deleteSavedQuery(savedQuery).catch((err) => setError(err.message))} /> },
    {
      key: 'dashboards',
      label: 'Dashboards',
      children: <DashboardsSection
        user={user}
        dashboards={dashboards}
        dashboardPanels={dashboardPanels}
        editingDashboardId={editingDashboardId}
        activePanelId={editingPanelId}
        dashboardName={dashboardName}
        dashboardDirty={dashboardDirty}
        dashboardImportText={dashboardImportText}
        panelTitle={panelTitle}
        panelVisualization={panelVisualization}
        onDashboardName={setDashboardName}
        onPanelTitle={setPanelTitle}
        onPanelVisualization={setPanelVisualization}
        onDashboardImportText={setDashboardImportText}
        onNewDashboard={newDashboard}
        onDiscardDashboard={discardDashboardChanges}
        onAddPanel={addPanelToDashboard}
        onUpdatePanel={updateSelectedPanel}
        onSave={saveDashboard}
        onSaveAs={() => saveDashboardAsCopy().catch((err) => setError(err.message))}
        onExport={exportDashboard}
        onImport={importDashboard}
        onOpen={openDashboard}
        onDuplicateDashboard={duplicateDashboard}
        onDeleteDashboard={(dashboard) => deleteDashboard(dashboard).catch((err) => setError(err.message))}
        onOpenPanel={openPanel}
        onDuplicatePanel={duplicatePanel}
        onMovePanel={movePanel}
        onRemovePanel={removePanel}
      />
    },
    { key: 'alerts', label: 'Alerts', children: <AlertsSection user={user} alertRules={alertRules} contacts={contacts} editingAlertId={editingAlertId} alertName={alertName} alertConditionType={alertConditionType} alertField={alertField} alertTextValue={alertTextValue} alertThreshold={alertThreshold} alertOperator={alertOperator} alertFor={alertFor} alertInterval={alertInterval} alertEnabled={alertEnabled} alertPreview={alertPreview} alertPreviewResult={alertPreviewResult} selectedContact={selectedContact} queryMode={query.mode || 'builder'} onName={setAlertName} onConditionType={changeAlertConditionType} onField={setAlertField} onTextValue={setAlertTextValue} onThreshold={setAlertThreshold} onOperator={setAlertOperator} onFor={setAlertFor} onInterval={setAlertInterval} onEnabled={setAlertEnabled} onContact={setSelectedContact} onNew={newAlertRule} onOpen={openAlertRule} onLoadQuery={loadAlertRuleQuery} onToggle={(rule) => toggleAlert(rule).catch((err) => setError(err.message))} onDelete={(rule) => deleteAlert(rule).catch((err) => setError(err.message))} onTest={() => testAlert().catch((err) => setError(err.message))} onSave={saveAlert} /> },
    { key: 'secrets', label: 'Secrets', children: <SecretsSection user={user} secrets={secrets} secretUsages={secretUsages} editingSecretId={editingSecretId} secretName={secretName} secretDescription={secretDescription} secretValue={secretValue} onName={setSecretName} onDescription={setSecretDescription} onValue={setSecretValue} onNew={newSecret} onSave={() => saveSecret().catch(reportError)} onOpen={openSecret} onDelete={(secret) => deleteSecret(secret).catch(reportError)} /> },
    { key: 'contacts', label: 'Contacts', children: <ContactsSection user={user} contacts={contacts} notifications={notifications} secrets={secrets} editingContactId={editingContactId} contactName={contactName} contactTarget={contactTarget} contactKind={contactKind} contactStatus={contactStatus} smtpConfigured={Boolean(config?.alertDelivery.smtpConfigured)} smtpHasAuth={Boolean(config?.alertDelivery.smtpHasAuth)} webhookTokenSecretRef={contactWebhookTokenSecretRef} webhookTokenValue={contactWebhookTokenValue} webhookHeaderName={contactWebhookHeaderName} webhookHeaderValueSecretRef={contactWebhookHeaderValueSecretRef} webhookHeaderValue={contactWebhookHeaderValue} webhookBodyTemplate={contactWebhookBodyTemplate} pagerDutyRoutingKeySecretRef={contactRoutingKeySecretRef} pagerDutyRoutingKeyValue={contactRoutingKeyValue} pagerDutyRestApiKeySecretRef={contactRestApiKeySecretRef} pagerDutyRestApiKeyValue={contactRestApiKeyValue} pagerDutySeverity={contactPagerDutySeverity} pagerDutySourceField={contactPagerDutySourceField} pagerDutyComponent={contactPagerDutyComponent} pagerDutyGroup={contactPagerDutyGroup} pagerDutyClass={contactPagerDutyClass} pagerDutyServiceID={contactPagerDutyServiceID} pagerDutyRestSyncEnabled={contactPagerDutyRestSyncEnabled} pagerDutyAutoSyncEnabled={contactPagerDutyAutoSyncEnabled} pagerDutySyncInterval={contactPagerDutySyncInterval} pagerDutyFromEmail={contactPagerDutyFromEmail} pagerDutyApiBaseURL={contactPagerDutyApiBaseURL} onName={setContactName} onTarget={setContactTarget} onKind={changeContactKind} onWebhookTokenSecretRef={setContactWebhookTokenSecretRef} onWebhookTokenValue={setContactWebhookTokenValue} onWebhookHeaderName={setContactWebhookHeaderName} onWebhookHeaderValueSecretRef={setContactWebhookHeaderValueSecretRef} onWebhookHeaderValue={setContactWebhookHeaderValue} onWebhookBodyTemplate={setContactWebhookBodyTemplate} onPagerDutyRoutingKeySecretRef={setContactRoutingKeySecretRef} onPagerDutyRoutingKeyValue={setContactRoutingKeyValue} onPagerDutyRestApiKeySecretRef={setContactRestApiKeySecretRef} onPagerDutyRestApiKeyValue={setContactRestApiKeyValue} onPagerDutySeverity={setContactPagerDutySeverity} onPagerDutySourceField={setContactPagerDutySourceField} onPagerDutyComponent={setContactPagerDutyComponent} onPagerDutyGroup={setContactPagerDutyGroup} onPagerDutyClass={setContactPagerDutyClass} onPagerDutyServiceID={setContactPagerDutyServiceID} onPagerDutyRestSyncEnabled={setContactPagerDutyRestSyncEnabled} onPagerDutyAutoSyncEnabled={setContactPagerDutyAutoSyncEnabled} onPagerDutySyncInterval={setContactPagerDutySyncInterval} onPagerDutyFromEmail={setContactPagerDutyFromEmail} onPagerDutyApiBaseURL={setContactPagerDutyApiBaseURL} onNew={newContact} onOpen={openContact} onUseForAlert={useContactForAlert} onSave={() => saveContact().catch(reportError)} onTest={() => testCurrentContact().catch(reportError)} onValidateSaved={(contact) => testContactAction(contact, 'validate').catch(reportError)} onResolveTestSaved={(contact) => testContactAction(contact, 'resolve').catch(reportError)} onTestSaved={(contact) => testContact(contact).catch(reportError)} onDelete={(contact) => deleteContact(contact).catch(reportError)} /> },
    { key: 'incidents', label: 'Incidents', children: <IncidentsSection incidents={incidents} onAcknowledge={acknowledgeIncident} onResolve={resolveIncident} onSyncPagerDuty={() => syncPagerDutyIncidents().catch(reportError)} /> },
    { key: 'notifications', label: <span>Notifications {failedNotificationCount > 0 && <Tag color="red">{failedNotificationCount}</Tag>}</span>, children: <NotificationsSection notifications={notifications} /> },
    { key: 'invites', label: 'Invites', children: <InvitesSection user={user} invites={invites} inviteEmail={inviteEmail} inviteRole={inviteRole} inviteToken={inviteToken} onEmail={setInviteEmail} onRole={setInviteRole} onToken={setInviteToken} onAccept={acceptInvite} onCreate={createInvite} onDelete={(invite) => deleteInvite(invite).catch((err) => setError(err.message))} /> },
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
      {messageContext}
      <Layout className={`app-shell theme-${themeMode}`}>
        <Sider
          width={sidebarWide ? 560 : 390}
          collapsedWidth={56}
          collapsed={sidebarCollapsed}
          trigger={null}
          className={`app-sidebar ${sidebarCollapsed ? 'app-sidebar-collapsed' : ''}`}
        >
          {sidebarCollapsed ? (
            <Space direction="vertical" size={12} align="center" className="full">
              <div className="mark mark-small">U</div>
              <Tooltip title="Expand sidebar" placement="right">
                <Button icon={<MenuUnfoldOutlined />} onClick={() => setSidebarCollapsed(false)} />
              </Tooltip>
            </Space>
          ) : (
            <Space direction="vertical" size={16} className="full">
              <Flex align="center" gap={12}>
                <div className="mark">U</div>
                <div className="sidebar-title">
                  <Typography.Title level={1}>Uvoo SQViz</Typography.Title>
                  <Typography.Text type="secondary">Tenant-aware ClickHouse analytics</Typography.Text>
                </div>
                <Flex gap={6} className="sidebar-tools">
                  <Tooltip title={sidebarWide ? 'Normal sidebar' : 'Wide sidebar'}>
                    <Button icon={<ColumnWidthOutlined />} type={sidebarWide ? 'primary' : 'default'} onClick={() => setSidebarWide((current) => !current)} />
                  </Tooltip>
                  <Tooltip title="Collapse sidebar">
                    <Button icon={<MenuFoldOutlined />} onClick={() => setSidebarCollapsed(true)} />
                  </Tooltip>
                </Flex>
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
          )}
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
              <Button
                icon={<ReloadOutlined />}
                disabled={!user}
                onClick={() => {
                  if (activeTab === 'dashboard') {
                    setDashboardRefreshKey((current) => current + 1);
                    setLastUpdated(new Date().toISOString());
                    return;
                  }
                  loadData().catch((err) => setError(err.message));
                }}
              >
                {activeTab === 'dashboard' ? 'Refresh panels' : `${rows.length} rows`}
              </Button>
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
              },
              {
                key: 'dashboard',
                label: `Dashboard (${dashboardPanels.length})`,
                children: <DashboardGrid panels={dashboardPanels} activePanelId={editingPanelId} themeMode={themeMode} relativeRange={relativeRange} refreshSeconds={refreshSeconds} refreshKey={dashboardRefreshKey} config={config} onOpen={openPanel} onDuplicate={duplicatePanel} onMove={movePanel} onResize={resizePanel} onRemove={removePanel} />
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
      {activeFilters(props.query.filters).map(([key, value]) => (
        <Tag key={key} icon={<FilterOutlined />}>{key}={value}</Tag>
      ))}
      <Tag>{props.query.limit} event rows</Tag>
      <Tag>{props.refreshSeconds > 0 ? `refresh ${formatRefresh(props.refreshSeconds)}` : 'manual refresh'}</Tag>
      {props.lastUpdated && <Typography.Text type="secondary">Updated {new Date(props.lastUpdated).toLocaleTimeString()}</Typography.Text>}
    </Flex>
  );
}

function DashboardGrid({
  panels,
  activePanelId,
  themeMode,
  relativeRange,
  refreshSeconds,
  refreshKey,
  config,
  onOpen,
  onDuplicate,
  onMove,
  onResize,
  onRemove
}: {
  panels: DashboardChart[];
  activePanelId: string;
  themeMode: ThemeMode;
  relativeRange: RelativeRange;
  refreshSeconds: number;
  refreshKey: number;
  config: PublicConfig | null;
  onOpen: (panel: DashboardChart) => void;
  onDuplicate: (index: number) => void;
  onMove: (index: number, direction: -1 | 1) => void;
  onResize: (panelID: string, width: number) => void;
  onRemove: (index: number) => void;
}) {
  if (panels.length === 0) {
    return <Alert type="info" showIcon message="No panels staged for this dashboard" />;
  }
  return (
    <div className="dashboard-grid">
      {panels.map((panel, index) => (
        <DashboardPanelCard
          key={panel.id || `${panel.title}-${index}`}
          panel={panel}
          index={index}
          panelCount={panels.length}
          active={panel.id === activePanelId}
          themeMode={themeMode}
          relativeRange={relativeRange}
          refreshSeconds={refreshSeconds}
          refreshKey={refreshKey}
          config={config}
          onOpen={onOpen}
          onDuplicate={onDuplicate}
          onMove={onMove}
          onResize={onResize}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}

function DashboardPanelCard({
  panel,
  index,
  panelCount,
  active,
  themeMode,
  relativeRange,
  refreshSeconds,
  refreshKey,
  config,
  onOpen,
  onDuplicate,
  onMove,
  onResize,
  onRemove
}: {
  panel: DashboardChart;
  index: number;
  panelCount: number;
  active: boolean;
  themeMode: ThemeMode;
  relativeRange: RelativeRange;
  refreshSeconds: number;
  refreshKey: number;
  config: PublicConfig | null;
  onOpen: (panel: DashboardChart) => void;
  onDuplicate: (index: number) => void;
  onMove: (index: number, direction: -1 | 1) => void;
  onResize: (panelID: string, width: number) => void;
  onRemove: (index: number) => void;
}) {
  const [panelRows, setPanelRows] = useState<QueryRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [panelError, setPanelError] = useState('');
  const basePanelQuery = useMemo(() => normalizePanelQuery(panel.query, config), [config, panel.query]);
  const displayPanelQuery = useMemo(() => basePanelQuery ? queryWithRelativeRange(basePanelQuery, relativeRange) : null, [basePanelQuery, relativeRange]);
  const queryKey = useMemo(() => JSON.stringify({ query: basePanelQuery, range: relativeRange }), [basePanelQuery, relativeRange]);
  const width = Math.max(1, Math.min(2, Number(panel.position?.w || 1)));

  useEffect(() => {
    let cancelled = false;
    async function run() {
      if (!basePanelQuery) {
        setPanelError('Panel query is incomplete');
        setPanelRows([]);
        return;
      }
      const effectiveQuery = queryWithRelativeRange(basePanelQuery, relativeRange);
      setLoading(true);
      setPanelError('');
      try {
        if ((effectiveQuery.mode || 'builder') === 'sql') {
          const result = await apiPost<{ rows: Record<string, unknown>[] }>('/api/sql', apiQueryPayload(effectiveQuery));
          if (!cancelled) setPanelRows(chartRowsFromCustomSQL(result.rows));
        } else {
          const result = await apiPost<{ rows: QueryRow[] }>('/api/query', apiQueryPayload(effectiveQuery));
          if (!cancelled) setPanelRows(result.rows);
        }
      } catch (err) {
        if (!cancelled) {
          setPanelRows([]);
          setPanelError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    run();
    if (refreshSeconds <= 0) {
      return () => {
        cancelled = true;
      };
    }
    const interval = window.setInterval(run, refreshSeconds * 1000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [queryKey, refreshKey, refreshSeconds]);

  return (
    <div
      className={`dashboard-panel ${active ? 'dashboard-panel-active' : ''}`}
      style={{ gridColumn: `span ${width}` }}
    >
      <div className="dashboard-panel-header">
        <div className="dashboard-panel-title">
          <Typography.Title level={4}>{panel.title}</Typography.Title>
          <Typography.Text type="secondary">{displayPanelQuery ? compactQueryDescription(displayPanelQuery) : 'Incomplete query'}</Typography.Text>
        </div>
        <Space size={6} className="dashboard-panel-tools" wrap>
          <Tag>{panel.visualization?.type || 'line'}</Tag>
          <Segmented
            size="small"
            value={width}
            options={[
              { label: '1x', value: 1 },
              { label: '2x', value: 2 }
            ]}
            onChange={(value) => panel.id && onResize(panel.id, Number(value))}
          />
          <Dropdown
            trigger={['click']}
            menu={{
              items: [
                { key: 'edit', label: 'Edit', icon: <EditOutlined /> },
                { key: 'move-up', label: 'Move up', icon: <UpOutlined />, disabled: index === 0 },
                { key: 'move-down', label: 'Move down', icon: <DownOutlined />, disabled: index === panelCount - 1 },
                { key: 'copy', label: 'Copy', icon: <CopyOutlined /> },
                { key: 'remove', label: 'Remove', icon: <DeleteOutlined />, danger: true }
              ],
              onClick: ({ key }) => {
                if (key === 'edit') onOpen(panel);
                if (key === 'move-up') onMove(index, -1);
                if (key === 'move-down') onMove(index, 1);
                if (key === 'copy') onDuplicate(index);
                if (key === 'remove') {
                  Modal.confirm({
                    title: 'Remove panel?',
                    content: 'This only removes the panel from the dashboard draft.',
                    okText: 'Remove',
                    okButtonProps: { danger: true },
                    onOk: () => onRemove(index)
                  });
                }
              }
            }}
          >
            <Button size="small" className="dashboard-panel-menu" icon={<MoreOutlined />} />
          </Dropdown>
        </Space>
      </div>
      {panelError ? (
        <Alert type="error" showIcon message={panelError} />
      ) : (
        <React.Suspense fallback={<div className="chart dashboard-chart chart-loading"><Spin /></div>}>
          <div className="dashboard-chart-wrap">
            {loading && <Spin className="dashboard-panel-spinner" />}
            <Chart rows={panelRows} themeMode={themeMode} type={(panel.visualization?.type as VisualizationType) || 'line'} />
          </div>
        </React.Suspense>
      )}
    </div>
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

function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  try {
    return JSON.stringify(err);
  } catch {
    return 'Unexpected application error';
  }
}

function capitalize(value: string): string {
  if (!value) return value;
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function buildSecretUsages(dataSources: DataSource[], contacts: ContactEndpoint[]): Record<string, string[]> {
  const usages: Record<string, string[]> = {};
  const add = (secretName: unknown, usage: string) => {
    const name = typeof secretName === 'string' ? secretName.trim() : '';
    if (!name) return;
    usages[name] = [...(usages[name] || []), usage];
  };
  dataSources.forEach((source) => {
    add(source.config.passwordSecretRef, `${source.name} password`);
  });
  contacts.forEach((contact) => {
    add(contact.config.routingKeySecretRef, `${contact.name} PagerDuty Events key`);
    add(contact.config.restApiKeySecretRef, `${contact.name} PagerDuty REST key`);
    add(contact.config.tokenSecretRef, `${contact.name} webhook bearer token`);
    add(contact.config.headerValueSecretRef, `${contact.name} webhook header`);
  });
  return usages;
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
    case 'neq':
      return '!=';
    case 'contains':
      return 'contains';
    case 'not_contains':
      return 'does not contain';
    case 'regex':
      return 'matches regex';
    default:
      return '>';
  }
}

function defaultAlertOperator(type: string): string {
  switch (type) {
    case 'text_match':
      return 'contains';
    case 'any_rows':
    case 'sql_result':
    case 'no_data':
      return 'exists';
    default:
      return 'gt';
  }
}

function defaultAlertField(type: string): string {
  return type === 'text_match' ? 'message' : 'value';
}

function normalizeAlertConditionPayload(condition?: AlertRule['condition']): AlertRule['condition'] {
  const type = condition?.type || 'numeric_threshold';
  return {
    type,
    operator: condition?.operator || defaultAlertOperator(type),
    field: condition?.field || defaultAlertField(type),
    threshold: Number(condition?.threshold ?? 0),
    value: condition?.value || '',
    for: condition?.for || ''
  };
}

function normalizeAlertPreviewResult(result: AlertPreviewResult): AlertPreviewResult {
  return {
    ...result,
    match_count: Number(result.match_count ?? 0),
    rows: Array.isArray(result.rows) ? result.rows : [],
    matches: Array.isArray(result.matches) ? result.matches : []
  };
}

function contactAutoSyncEnabled(config: Record<string, unknown>, fallback: boolean): boolean {
  const raw = String(config.autoSyncEnabled ?? '').trim().toLowerCase();
  if (!raw) return fallback;
  return raw === 'true' || raw === '1' || raw === 'yes' || raw === 'on';
}

function contactSyncInterval(config: Record<string, unknown>, fallback: number): number {
  const raw = Number(config.syncIntervalSeconds ?? fallback);
  if (!Number.isFinite(raw) || raw < 0) return 0;
  return Math.floor(raw);
}

function alertPreviewText(result: { value: number; operator: string; threshold: number; firing: boolean; match_count: number; condition: AlertRule['condition'] }): string {
  const condition = normalizeAlertConditionPayload(result.condition);
  const prefix = result.firing ? 'Firing' : 'OK';
  if (condition.type === 'any_rows' || condition.type === 'sql_result') return `${prefix}: ${result.match_count} matching rows`;
  if (condition.type === 'no_data') return `${prefix}: ${result.match_count > 0 ? 'no rows returned' : 'rows returned'}`;
  if (condition.type === 'row_count') return `${prefix}: row count ${formatNumber(result.value)} ${operatorLabel(result.operator)} ${formatNumber(result.threshold)}`;
  if (condition.type === 'text_match') return `${prefix}: ${result.match_count} text matches`;
  return `${prefix}: ${condition.field || 'value'} ${formatNumber(result.value)} ${operatorLabel(result.operator)} ${formatNumber(result.threshold)}`;
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
    compactFilters(query.filters),
    formatRange(query.from, query.to)
  ].filter(Boolean).join(' - ');
}

function compactFilters(filters: Record<string, string> | undefined): string {
  const active = Object.entries(filters || {}).filter(([, value]) => value.trim() !== '');
  if (active.length === 0) return '';
  return active.map(([key, value]) => `${key}=${value}`).join(', ');
}

function defaultGroupBy(config: PublicConfig | null, datasetID: string): string {
  return config?.datasets.find((item) => item.id === datasetID)?.dimensions[0] || '';
}

function defaultMeasure(config: PublicConfig | null, datasetID: string): string {
  return config?.datasets.find((item) => item.id === datasetID)?.defaultMeasure || '_rows';
}

function defaultAggregation(config: PublicConfig | null, datasetID: string): string {
  return config?.datasets.find((item) => item.id === datasetID)?.defaultAggregation || 'count';
}

function clampEventLimit(value: number): number {
  return Math.max(10, Math.min(1000, Math.trunc(value || 200)));
}

function isAllowedRefresh(value: unknown): value is number {
  return value === 0 || value === 10 || value === 30 || value === 60 || value === 300;
}

function isAllowedRangeUnit(value: unknown): value is RelativeRangeUnit {
  return value === 'minutes' || value === 'hours' || value === 'days' || value === 'weeks' || value === 'months' || value === 'years';
}

function isAllowedVisualization(value: unknown): value is VisualizationType {
  return value === 'line' || value === 'area' || value === 'bar';
}

function activeFilters(filters: Record<string, string> | undefined): [string, string][] {
  return Object.entries(filters || {}).filter(([, value]) => value.trim() !== '');
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

function normalizeDashboardPanels(panels: DashboardChart[]): DashboardChart[] {
  return panels.map((panel, index) => normalizeDashboardPanel(panel, index));
}

function dashboardSignature(name: string, panels: DashboardChart[]): string {
  return JSON.stringify({
    name: name.trim(),
    panels: normalizeDashboardPanels(panels).map((panel) => ({
      id: panel.id,
      title: panel.title,
      query: panel.query,
      visualization: panel.visualization,
      position: panel.position
    }))
  });
}

function dashboardIsDirty(name: string, panels: DashboardChart[], savedSignature: string, editingDashboardID: string): boolean {
  if (savedSignature) return savedSignature !== dashboardSignature(name, panels);
  return panels.length > 0 || Boolean(editingDashboardID);
}

function safeFileName(value: string): string {
  const cleaned = value.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  return cleaned || 'dashboard';
}

function newClientID(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID();
  }
  const bytes = new Uint8Array(16);
  if (typeof globalThis.crypto?.getRandomValues === 'function') {
    globalThis.crypto.getRandomValues(bytes);
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0'));
  return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`;
}

function normalizeDashboardPanel(panel: DashboardChart, index: number): DashboardChart {
  return {
    ...panel,
    id: panel.id || newClientID(),
    title: panel.title || `Panel ${index + 1}`,
    visualization: { type: panel.visualization?.type || 'line', ...(panel.visualization || {}) },
    position: {
      ...(panel.position || {}),
      x: Number(panel.position?.x || 0),
      y: Number(panel.position?.y ?? index),
      w: Math.max(1, Math.min(2, Number(panel.position?.w || 1))),
      h: Math.max(1, Number(panel.position?.h || 1))
    }
  };
}

function normalizePanelQuery(payload: unknown, config: PublicConfig | null): QueryState | null {
  const partial = editableQuery((payload || {}) as Partial<QueryState>);
  if (!partial.dataset) return null;
  const dataset = config?.datasets.find((item) => item.id === partial.dataset);
  const to = new Date();
  const from = new Date(to.getTime() - 60 * 60 * 1000);
  return {
    dataset: partial.dataset,
    sourceId: partial.sourceId || '',
    mode: partial.mode || 'builder',
    sql: partial.sql || defaultSQL(partial.dataset),
    groupBy: partial.groupBy ?? dataset?.dimensions[0] ?? '',
    measure: partial.measure || dataset?.defaultMeasure || '_rows',
    aggregation: partial.aggregation || dataset?.defaultAggregation || 'count',
    from: partial.from || toInput(from),
    to: partial.to || toInput(to),
    search: partial.search || '',
    filters: partial.filters || {},
    filterOps: partial.filterOps || {},
    limit: partial.limit || 200
  };
}

function apiQueryPayload(nextQuery: QueryState): QueryState {
  return {
    ...nextQuery,
    sourceId: nextQuery.sourceId || '',
    from: new Date(nextQuery.from).toISOString(),
    to: new Date(nextQuery.to).toISOString()
  };
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

function canManageTenant(profile: UserProfile | null): boolean {
  return profile?.role === 'owner' || profile?.role === 'admin';
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
