import {
  Alert,
  Button,
  Collapse,
  Dropdown,
  Empty,
  Flex,
  Input,
  InputNumber,
  List,
  Modal,
  Select,
  Segmented,
  Space,
  Switch,
  Tag,
  Typography
} from 'antd';
import {
  CheckCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  DownOutlined,
  EditOutlined,
  LoginOutlined,
  MoreOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  SaveOutlined,
  UpOutlined
} from '@ant-design/icons';
import React from 'react';
import {
  AlertIncident,
  AlertNotification,
  AlertRule,
  AuditEvent,
  ContactEndpoint,
  DataSource,
  Dataset,
  Dashboard,
  DashboardChart,
  Provider,
  PublicConfig,
  QueryHistory,
  SavedQuery,
  TenantInvite,
  TenantMember,
  TenantMembership,
  UserProfile,
  Principal
} from '../api';
import { JwtClaims, QueryState, RelativeRange, RelativeRangeUnit, ThemeMode, VisualizationType } from '../types';

type Role = 'owner' | 'admin' | 'editor' | 'viewer';
type RowAction = {
  key: string;
  label: string;
  icon?: React.ReactNode;
  danger?: boolean;
  disabled?: boolean;
  confirm?: {
    title: string;
    content: string;
    okText?: string;
  };
  onClick: () => void;
};

export function AccessSection(props: {
  config: PublicConfig | null;
  user: Principal | null;
  profile: UserProfile | null;
  activeTenant: string;
  memberships: TenantMembership[];
  jwtClaims: JwtClaims | null;
  tokenInput: string;
  onTokenInput: (value: string) => void;
  onLogin: (provider: Provider) => void;
  onSaveToken: () => void;
  onDevLogin: () => void;
  onSelectTenant: (tenant: string) => void;
  onSignOut: () => void;
}) {
  const tenantValue = props.activeTenant || props.user?.tenantId || '';
  const claimEntries = props.jwtClaims ? orderedClaimEntries(props.jwtClaims) : [];
  if (props.user) {
    return (
      <Section title="Access">
        <Space direction="vertical" size={8} className="full">
          <Typography.Text strong>{props.user.name || props.user.email}</Typography.Text>
          {props.user.email && props.user.email !== props.user.name && (
            <Typography.Text type="secondary">{props.user.email}</Typography.Text>
          )}
          <Typography.Text type="secondary">
            {props.profile?.role || 'member'} in {props.profile?.tenant_slug || tenantValue}
          </Typography.Text>
          <Flex gap={6} wrap="wrap">
            <Tag>{props.user.provider || props.profile?.provider || 'auth'}</Tag>
            <Tag>{props.profile?.role || 'member'}</Tag>
            <Tag>{tenantValue}</Tag>
          </Flex>
          <div className="identity-grid">
            <span>Subject</span>
            <code>{props.user.subject || '-'}</code>
            <span>Tenant</span>
            <code>{tenantValue || '-'}</code>
          </div>
          <Select value={tenantValue} onChange={props.onSelectTenant}>
            {props.memberships.length === 0 && <Select.Option value={tenantValue}>{tenantValue}</Select.Option>}
            {props.memberships.map((membership) => (
              <Select.Option key={membership.tenant_slug} value={membership.tenant_slug}>
                {membership.tenant_name} ({membership.role})
              </Select.Option>
            ))}
          </Select>
          {props.jwtClaims ? (
            <Collapse
              size="small"
              ghost
              items={[{
                key: 'jwt',
                label: 'JWT claims',
                children: (
                  <div className="claims-grid">
                    {claimEntries.map(([key, value]) => (
                      <React.Fragment key={key}>
                        <span>{key}</span>
                        <code>{formatClaimValue(key, value)}</code>
                      </React.Fragment>
                    ))}
                  </div>
                )
              }]}
            />
          ) : (
            <Typography.Text type="secondary">No browser JWT stored; using development auth.</Typography.Text>
          )}
          <Button onClick={props.onSignOut}>Sign out</Button>
        </Space>
      </Section>
    );
  }
  return (
    <Section title="Access">
      <Space direction="vertical" size={8} className="full">
        <Flex gap={8} wrap="wrap">
          {props.config?.providers.filter((p) => p.enabled).map((provider) => (
            <Button icon={<LoginOutlined />} key={provider.id} onClick={() => props.onLogin(provider)}>
              {provider.name}
            </Button>
          ))}
        </Flex>
        <Input.TextArea
          autoSize={{ minRows: 3, maxRows: 5 }}
          value={props.tokenInput}
          onChange={(event) => props.onTokenInput(event.target.value)}
          placeholder="Paste an OIDC JWT"
        />
        <Button icon={<CheckCircleOutlined />} onClick={props.onSaveToken}>
          Use token
        </Button>
        {props.config?.devMode && (
          <Button onClick={props.onDevLogin}>
            Use development login
          </Button>
        )}
      </Space>
    </Section>
  );
}

export function SettingsSection(props: {
  user: Principal | null;
  config: PublicConfig | null;
  dataSources: DataSource[];
  preferencesStatus: string;
  themeMode: ThemeMode;
  refreshSeconds: number;
  relativeRange: RelativeRange;
  eventLimit: number;
  dataset: string;
  sourceId: string;
  visualization: VisualizationType;
  onTheme: (value: ThemeMode) => void;
  onRefreshSeconds: (value: number) => void;
  onRelativeRange: (value: number, unit: RelativeRangeUnit) => void;
  onEventLimit: (value: number) => void;
  onDataset: (value: string) => void;
  onSourceId: (value: string) => void;
  onVisualization: (value: VisualizationType) => void;
  onSave: () => void;
}) {
  return (
    <Section title="Settings">
      <Field label="Theme">
        <Segmented
          block
          value={props.themeMode}
          options={[
            { label: 'Light', value: 'light' },
            { label: 'Dark', value: 'dark' }
          ]}
          onChange={(value) => props.onTheme(value as ThemeMode)}
        />
      </Field>
      <Field label="Refresh">
        <Segmented
          block
          value={props.refreshSeconds}
          options={[
            { label: 'Off', value: 0 },
            { label: '10s', value: 10 },
            { label: '30s', value: 30 },
            { label: '1m', value: 60 },
            { label: '5m', value: 300 }
          ]}
          onChange={(value) => props.onRefreshSeconds(Number(value))}
        />
      </Field>
      <Field label="Default range">
        <Space.Compact className="full relative-range">
          <InputNumber
            className="relative-range-value"
            min={1}
            max={999}
            value={props.relativeRange.value}
            onChange={(value) => props.onRelativeRange(Number(value || 1), props.relativeRange.unit)}
          />
          <Select
            className="relative-range-unit"
            value={props.relativeRange.unit}
            onChange={(unit: RelativeRangeUnit) => props.onRelativeRange(props.relativeRange.value, unit)}
          >
            <Select.Option value="minutes">minutes</Select.Option>
            <Select.Option value="hours">hours</Select.Option>
            <Select.Option value="days">days</Select.Option>
            <Select.Option value="weeks">weeks</Select.Option>
            <Select.Option value="months">months</Select.Option>
            <Select.Option value="years">years</Select.Option>
          </Select>
        </Space.Compact>
      </Field>
      <Field label="Event rows">
        <InputNumber className="full" min={10} max={1000} value={props.eventLimit} onChange={(value) => props.onEventLimit(Number(value || 200))} />
      </Field>
      <Field label="Dataset">
        <Select value={props.dataset} onChange={props.onDataset}>
          {props.config?.datasets.map((item) => <Select.Option key={item.id} value={item.id}>{item.name}</Select.Option>)}
        </Select>
      </Field>
      <Field label="Source">
        <Select value={props.sourceId} onChange={props.onSourceId}>
          <Select.Option value="">Server default</Select.Option>
          {props.dataSources.map((source) => <Select.Option key={source.id} value={source.id}>{source.name}</Select.Option>)}
        </Select>
      </Field>
      <Field label="Visualization">
        <Select value={props.visualization} onChange={props.onVisualization}>
          <Select.Option value="line">Line</Select.Option>
          <Select.Option value="area">Area</Select.Option>
          <Select.Option value="bar">Bar</Select.Option>
        </Select>
      </Field>
      <Flex align="center" gap={8} wrap="wrap">
        <Button icon={<SaveOutlined />} disabled={!props.user} onClick={props.onSave}>Save now</Button>
        {props.preferencesStatus && <Typography.Text type="secondary">{props.preferencesStatus}</Typography.Text>}
      </Flex>
    </Section>
  );
}

function orderedClaimEntries(claims: JwtClaims): [string, unknown][] {
  const preferred = ['iss', 'sub', 'email', 'name', 'preferred_username', 'tenant_id', 'role', 'roles', 'aud', 'azp', 'exp', 'iat', 'nbf'];
  return Object.entries(claims).sort(([left], [right]) => {
    const leftIndex = preferred.indexOf(left);
    const rightIndex = preferred.indexOf(right);
    if (leftIndex !== -1 || rightIndex !== -1) return (leftIndex === -1 ? preferred.length : leftIndex) - (rightIndex === -1 ? preferred.length : rightIndex);
    return left.localeCompare(right);
  });
}

function formatClaimValue(key: string, value: unknown): string {
  if ((key === 'exp' || key === 'iat' || key === 'nbf') && typeof value === 'number') {
    return `${new Date(value * 1000).toLocaleString()} (${value})`;
  }
  if (value === null || value === undefined) return '-';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}

export function SourceSection(props: {
  user: Principal | null;
  dataSources: DataSource[];
  sourceName: string;
  sourceURL: string;
  sourceDatabase: string;
  sourceUser: string;
  sourceSecretRef: string;
  sourceStatus: string;
  editingSourceId: string;
  onName: (value: string) => void;
  onURL: (value: string) => void;
  onDatabase: (value: string) => void;
  onUser: (value: string) => void;
  onSecretRef: (value: string) => void;
  onNew: () => void;
  onSave: () => void;
  onTest: () => void;
  onOpen: (source: DataSource) => void;
  onDelete: (source: DataSource) => void;
}) {
  const [sourceSearch, setSourceSearch] = React.useState('');
  const filteredSources = React.useMemo(() => {
    const term = sourceSearch.trim().toLowerCase();
    if (!term) return props.dataSources;
    return props.dataSources.filter((source) => [
      source.name,
      source.kind,
      String(source.config.url || ''),
      String(source.config.database || '')
    ].some((value) => value.toLowerCase().includes(term)));
  }, [props.dataSources, sourceSearch]);

  return (
    <Section title="Sources">
      {props.editingSourceId && <Tag color="blue">Editing existing source</Tag>}
      <Field label="Name"><Input value={props.sourceName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="URL"><Input value={props.sourceURL} onChange={(event) => props.onURL(event.target.value)} /></Field>
      <Field label="Database"><Input value={props.sourceDatabase} onChange={(event) => props.onDatabase(event.target.value)} /></Field>
      <Field label="User"><Input value={props.sourceUser} onChange={(event) => props.onUser(event.target.value)} /></Field>
      <Field label="Secret"><Input value={props.sourceSecretRef} onChange={(event) => props.onSecretRef(event.target.value)} /></Field>
      <Flex gap={8} wrap="wrap">
        <Button onClick={props.onNew}>New</Button>
        <Button icon={<SaveOutlined />} disabled={!props.user || !props.sourceURL} onClick={props.onSave}>{props.editingSourceId ? 'Update source' : 'Save source'}</Button>
        <Button disabled={!props.user || !props.editingSourceId} onClick={props.onTest}>Test</Button>
      </Flex>
      {props.sourceStatus && <Alert type="info" showIcon message={props.sourceStatus} />}
      <Field label="Find source">
        <Input.Search allowClear value={sourceSearch} onChange={(event) => setSourceSearch(event.target.value)} />
      </Field>
      <ActionList items={filteredSources} empty="No sources" render={(source) => (
        <Flex key={source.id} gap={6} align="stretch" className="action-row">
          <RowActions items={[
            { key: 'edit', label: 'Edit', onClick: () => props.onOpen(source) },
            {
              key: 'delete',
              label: 'Delete',
              danger: true,
              confirm: {
                title: 'Delete source?',
                content: 'Saved queries and dashboards that reference it may fall back to the server default.',
                okText: 'Delete'
              },
              onClick: () => props.onDelete(source)
            }
          ]} />
          <Button block className="stack-button" type={source.id === props.editingSourceId ? 'primary' : 'default'} onClick={() => props.onOpen(source)}>
            <span>{source.name}</span>
            <small>{source.kind} - {String(source.config.url || 'server default')}</small>
          </Button>
        </Flex>
      )} />
    </Section>
  );
}

export function QuerySection(props: {
  config: PublicConfig | null;
  user: Principal | null;
  query: QueryState;
  dataset?: Dataset;
  dataSources: DataSource[];
  relativeRange: RelativeRange;
  onQuery: (query: QueryState) => void;
  onSource: (source: DataSource) => void;
  onRun: () => void;
  onRelativeRange: (value: number, unit: RelativeRangeUnit) => void;
}) {
  const q = props.query;
  return (
    <Section title="Query">
      <Field label="Mode">
        <Segmented
          block
          value={q.mode || 'builder'}
          options={[
            { label: 'Builder', value: 'builder' },
            { label: 'SQL', value: 'sql' }
          ]}
          onChange={(value) => props.onQuery({ ...q, mode: value as QueryState['mode'] })}
        />
      </Field>
      <Field label="Source">
        <Select value={q.sourceId} onChange={(value) => {
          const selected = props.dataSources.find((source) => source.id === value);
          props.onQuery({ ...q, sourceId: value });
          if (selected) props.onSource(selected);
        }}>
          <Select.Option value="">Server default</Select.Option>
          {props.dataSources.map((source) => <Select.Option key={source.id} value={source.id}>{source.name}</Select.Option>)}
        </Select>
      </Field>
      <Field label="Dataset">
        <Select value={q.dataset} onChange={(value) => {
          const next = props.config?.datasets.find((item) => item.id === value);
          props.onQuery({
            ...q,
            dataset: value,
            sql: q.mode === 'sql' ? defaultDatasetSQL(value) : q.sql,
            groupBy: firstDimension(props.config, value),
            measure: next?.defaultMeasure || '_rows',
            aggregation: next?.defaultAggregation || 'count',
            filters: {},
            filterOps: {}
          });
        }}>
          {props.config?.datasets.map((item) => <Select.Option key={item.id} value={item.id}>{item.name}</Select.Option>)}
        </Select>
      </Field>
      {(q.mode || 'builder') === 'builder' && (
        <>
          <Field label="Group">
            <Select value={q.groupBy} onChange={(value) => props.onQuery({ ...q, groupBy: value })}>
              <Select.Option value="">All</Select.Option>
              {props.dataset?.dimensions.map((item) => <Select.Option key={item} value={item}>{item}</Select.Option>)}
            </Select>
          </Field>
          <Field label="Measure">
            <Select value={q.measure} onChange={(value) => props.onQuery({ ...q, measure: value })}>
              {props.dataset?.measures.map((item) => <Select.Option key={item} value={item}>{item}</Select.Option>)}
            </Select>
          </Field>
          <Field label="Aggregation">
            <Select value={q.aggregation} onChange={(value) => props.onQuery({ ...q, aggregation: value })}>
              {props.dataset?.aggregations.map((item) => <Select.Option key={item} value={item}>{item}</Select.Option>)}
            </Select>
          </Field>
          {(props.dataset?.filters.length || 0) > 0 && (
            <Field label="Filters">
              <Space direction="vertical" size={6} className="full">
                {props.dataset?.filters.map((filter) => (
                  <Space.Compact key={filter} className="full query-filter">
                    <Input className="query-filter-name" value={filter} disabled />
                    <Select
                      className="query-filter-operator"
                      value={q.filterOps?.[filter] || 'eq'}
                      onChange={(operator) => props.onQuery({
                        ...q,
                        filterOps: { ...(q.filterOps || {}), [filter]: operator }
                      })}
                    >
                      {filterOperators(props.dataset, filter).map((operator) => (
                        <Select.Option key={operator} value={operator}>{operatorLabel(operator)}</Select.Option>
                      ))}
                    </Select>
                    <Input
                      allowClear
                      value={q.filters?.[filter] || ''}
                      placeholder="value"
                      onChange={(event) => props.onQuery({
                        ...q,
                        filters: { ...(q.filters || {}), [filter]: event.target.value }
                      })}
                    />
                  </Space.Compact>
                ))}
              </Space>
            </Field>
          )}
        </>
      )}
      <Field label="Last">
        <Space.Compact className="full relative-range">
          <InputNumber
            className="relative-range-value"
            min={1}
            max={999}
            value={props.relativeRange.value}
            onChange={(value) => props.onRelativeRange(Number(value || 1), props.relativeRange.unit)}
          />
          <Select
            className="relative-range-unit"
            value={props.relativeRange.unit}
            onChange={(unit: RelativeRangeUnit) => props.onRelativeRange(props.relativeRange.value, unit)}
          >
            <Select.Option value="minutes">minutes</Select.Option>
            <Select.Option value="hours">hours</Select.Option>
            <Select.Option value="days">days</Select.Option>
            <Select.Option value="weeks">weeks</Select.Option>
            <Select.Option value="months">months</Select.Option>
            <Select.Option value="years">years</Select.Option>
          </Select>
        </Space.Compact>
      </Field>
      <Field label="From"><Input type="datetime-local" value={q.from} onChange={(event) => props.onQuery({ ...q, from: event.target.value })} /></Field>
      <Field label="To"><Input type="datetime-local" value={q.to} onChange={(event) => props.onQuery({ ...q, to: event.target.value })} /></Field>
      {(q.mode || 'builder') === 'sql' ? (
        <Field label="SQL">
          <Input.TextArea
            autoSize={{ minRows: 8, maxRows: 16 }}
            value={q.sql || ''}
            onChange={(event) => props.onQuery({ ...q, sql: event.target.value })}
            placeholder="SELECT count() AS value FROM otel_logs WHERE tenant_id = {tenant:String} AND timestamp >= {from:DateTime} AND timestamp < {to:DateTime}"
          />
        </Field>
      ) : (
        <Field label="Search">
          <Input.Search
            allowClear
            enterButton="Run"
            value={q.search}
            onChange={(event) => props.onQuery({ ...q, search: event.target.value })}
            onSearch={() => props.onRun()}
            placeholder="Search log body, service, trace id"
          />
        </Field>
      )}
      <Field label="Event rows">
        <InputNumber className="full" min={10} max={1000} value={q.limit} onChange={(value) => props.onQuery({ ...q, limit: Number(value || 100) })} />
      </Field>
      <Flex gap={8}>
        <Button type="primary" icon={<PlayCircleOutlined />} disabled={!props.user} onClick={() => props.onRun()}>Run</Button>
      </Flex>
    </Section>
  );
}

export function HistorySection({ queryHistory, onOpen }: { queryHistory: QueryHistory[]; onOpen: (history: QueryHistory) => void }) {
  return (
    <Section title="History">
      <ActionList items={queryHistory.slice(0, 8)} empty="No history" render={(history) => (
        <Button block className="stack-button" key={history.id} danger={history.status === 'failed'} onClick={() => onOpen(history)}>
          <span>{history.dataset}</span>
          <small>{history.rows_count} rows - {history.duration_ms} ms</small>
        </Button>
      )} />
    </Section>
  );
}

export function SavedQueriesSection(props: {
  user: Principal | null;
  savedQueries: SavedQuery[];
  editingSavedQueryId: string;
  savedQueryName: string;
  savedQueryDescription: string;
  onName: (value: string) => void;
  onDescription: (value: string) => void;
  onNew: () => void;
  onSave: () => void;
  onOpen: (query: SavedQuery) => void;
  onDelete: (query: SavedQuery) => void;
}) {
  const [querySearch, setQuerySearch] = React.useState('');
  const filteredQueries = React.useMemo(() => {
    const term = querySearch.trim().toLowerCase();
    if (!term) return props.savedQueries;
    return props.savedQueries.filter((query) => [
      query.name,
      query.description,
      describePanelQuery(query.query)
    ].some((value) => value.toLowerCase().includes(term)));
  }, [props.savedQueries, querySearch]);

  return (
    <Section title="Saved Queries">
      {props.editingSavedQueryId && <Tag color="blue">Editing saved query</Tag>}
      <Field label="Name"><Input value={props.savedQueryName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="Description"><Input.TextArea value={props.savedQueryDescription} onChange={(event) => props.onDescription(event.target.value)} /></Field>
      <Flex gap={8} wrap="wrap">
        <Button onClick={props.onNew}>New</Button>
        <Button icon={<SaveOutlined />} disabled={!props.user || !props.savedQueryName} onClick={props.onSave}>{props.editingSavedQueryId ? 'Update query' : 'Save query'}</Button>
      </Flex>
      <Field label="Find query">
        <Input.Search allowClear value={querySearch} onChange={(event) => setQuerySearch(event.target.value)} />
      </Field>
      <ActionList items={filteredQueries} empty="No saved queries" render={(query) => (
        <Flex key={query.id} gap={6} align="stretch" className="action-row">
          <RowActions items={[
            { key: 'edit', label: 'Edit', onClick: () => props.onOpen(query) },
            {
              key: 'delete',
              label: 'Delete',
              danger: true,
              confirm: {
                title: 'Delete saved query?',
                content: 'This does not change dashboards or alert rules already using copied query payloads.',
                okText: 'Delete'
              },
              onClick: () => props.onDelete(query)
            }
          ]} />
          <Button block className="stack-button" type={query.id === props.editingSavedQueryId ? 'primary' : 'default'} onClick={() => props.onOpen(query)}>
            <span>{query.name}</span>
            <small>{query.description || describePanelQuery(query.query)}</small>
          </Button>
        </Flex>
      )} />
    </Section>
  );
}

export function DashboardsSection(props: {
  user: Principal | null;
  dashboards: Dashboard[];
  dashboardPanels: DashboardChart[];
  editingDashboardId: string;
  activePanelId: string;
  dashboardName: string;
  dashboardDirty: boolean;
  dashboardImportText: string;
  panelTitle: string;
  panelVisualization: VisualizationType;
  onDashboardName: (value: string) => void;
  onPanelTitle: (value: string) => void;
  onPanelVisualization: (value: VisualizationType) => void;
  onDashboardImportText: (value: string) => void;
  onNewDashboard: () => void;
  onDiscardDashboard: () => void;
  onAddPanel: () => void;
  onUpdatePanel: () => void;
  onSave: () => void;
  onSaveAs: () => void;
  onExport: () => void;
  onImport: () => void;
  onOpen: (dashboard: Dashboard) => void;
  onDuplicateDashboard: (dashboard: Dashboard) => void;
  onDeleteDashboard: (dashboard: Dashboard) => void;
  onOpenPanel: (panel: DashboardChart) => void;
  onDuplicatePanel: (index: number) => void;
  onMovePanel: (index: number, direction: -1 | 1) => void;
  onRemovePanel: (index: number) => void;
}) {
  const [dashboardSearch, setDashboardSearch] = React.useState('');
  const filteredDashboards = React.useMemo(() => {
    const term = dashboardSearch.trim().toLowerCase();
    if (!term) return props.dashboards;
    return props.dashboards.filter((dashboard) => dashboard.name.toLowerCase().includes(term));
  }, [dashboardSearch, props.dashboards]);

  return (
    <Section title="Dashboards">
      <Flex gap={6} wrap="wrap">
        {props.editingDashboardId && <Tag color="blue">Editing saved dashboard</Tag>}
        {props.dashboardDirty && <Tag color="orange">Unsaved changes</Tag>}
      </Flex>
      <Field label="Name"><Input value={props.dashboardName} onChange={(event) => props.onDashboardName(event.target.value)} /></Field>
      <Field label="Panel title"><Input value={props.panelTitle} onChange={(event) => props.onPanelTitle(event.target.value)} /></Field>
      <Field label="Visualization">
        <Select value={props.panelVisualization} onChange={props.onPanelVisualization}>
          <Select.Option value="line">Line</Select.Option>
          <Select.Option value="area">Area</Select.Option>
          <Select.Option value="bar">Bar</Select.Option>
        </Select>
      </Field>
      <Flex gap={8} wrap="wrap">
        <Button onClick={props.onNewDashboard}>New</Button>
        <Button disabled={!props.dashboardDirty} onClick={props.onDiscardDashboard}>Discard</Button>
        <Button icon={<PlusOutlined />} disabled={!props.user} onClick={props.onAddPanel}>Add</Button>
        <Button icon={<EditOutlined />} disabled={!props.user || !props.activePanelId} onClick={props.onUpdatePanel}>Update</Button>
        <Button icon={<SaveOutlined />} disabled={!props.user || !props.dashboardName || props.dashboardPanels.length === 0} onClick={props.onSave}>Save</Button>
        <Button disabled={!props.user || props.dashboardPanels.length === 0} onClick={props.onSaveAs}>Save as</Button>
        <Button disabled={props.dashboardPanels.length === 0} onClick={props.onExport}>Export</Button>
      </Flex>
      <Field label="Import JSON">
        <Input.TextArea
          autoSize={{ minRows: 3, maxRows: 7 }}
          value={props.dashboardImportText}
          onChange={(event) => props.onDashboardImportText(event.target.value)}
        />
      </Field>
      <Button disabled={!props.dashboardImportText.trim()} onClick={props.onImport}>Import</Button>
      <ActionList items={props.dashboardPanels} empty="No staged panels" render={(panel, index) => (
        <Flex key={panel.id || `${panel.title}-${index}`} gap={6} align="stretch" className="action-row">
          <RowActions items={[
            { key: 'edit', label: 'Edit', onClick: () => props.onOpenPanel(panel) },
            { key: 'move-up', label: 'Move up', disabled: index === 0, onClick: () => props.onMovePanel(index, -1) },
            { key: 'move-down', label: 'Move down', disabled: index === props.dashboardPanels.length - 1, onClick: () => props.onMovePanel(index, 1) },
            { key: 'copy', label: 'Copy', onClick: () => props.onDuplicatePanel(index) },
            {
              key: 'remove',
              label: 'Remove',
              danger: true,
              confirm: {
                title: 'Remove panel?',
                content: 'This only removes the panel from the dashboard draft.',
                okText: 'Remove'
              },
              onClick: () => props.onRemovePanel(index)
            }
          ]} />
          <Button className="grow stack-button" type={panel.id === props.activePanelId ? 'primary' : 'default'} onClick={() => props.onOpenPanel(panel)}>
            <span>{panel.title}</span>
            <small>{describePanelQuery(panel.query)}</small>
          </Button>
        </Flex>
      )} />
      <Field label="Find dashboard">
        <Input.Search allowClear value={dashboardSearch} onChange={(event) => setDashboardSearch(event.target.value)} />
      </Field>
      <ActionList items={filteredDashboards} empty="No dashboards" render={(dashboard) => (
        <Flex key={dashboard.id} gap={6} align="stretch" className="action-row">
          <RowActions items={[
            { key: 'open', label: 'Open', onClick: () => props.onOpen(dashboard) },
            { key: 'duplicate', label: 'Duplicate', onClick: () => props.onDuplicateDashboard(dashboard) },
            {
              key: 'delete',
              label: 'Delete',
              danger: true,
              confirm: {
                title: 'Delete dashboard?',
                content: 'This deletes the saved dashboard for this tenant.',
                okText: 'Delete'
              },
              onClick: () => props.onDeleteDashboard(dashboard)
            }
          ]} />
          <Button block className="stack-button" type={dashboard.id === props.editingDashboardId ? 'primary' : 'default'} onClick={() => props.onOpen(dashboard)}>
            <span>{dashboard.name}</span>
            <small>{dashboard.layout?.charts?.length || 0} panels - updated {new Date(dashboard.updated_at).toLocaleString()}</small>
          </Button>
        </Flex>
      )} />
    </Section>
  );
}

function describePanelQuery(query: unknown): string {
  if (!query || typeof query !== 'object') return 'No query';
  const q = query as { dataset?: unknown; groupBy?: unknown; mode?: unknown };
  if (q.mode === 'sql') return `${String(q.dataset || 'dataset')} SQL`;
  return [q.dataset, q.groupBy ? `by ${q.groupBy}` : 'all'].filter(Boolean).join(' ');
}

export function AlertsSection(props: {
  user: Principal | null;
  alertRules: AlertRule[];
  contacts: ContactEndpoint[];
  editingAlertId: string;
  alertName: string;
  alertThreshold: string;
  alertOperator: string;
  alertFor: string;
  alertInterval: number;
  alertEnabled: boolean;
  alertPreview: string;
  selectedContact: string;
  queryMode: string;
  onName: (value: string) => void;
  onThreshold: (value: string) => void;
  onOperator: (value: string) => void;
  onFor: (value: string) => void;
  onInterval: (value: number) => void;
  onEnabled: (value: boolean) => void;
  onContact: (value: string) => void;
  onNew: () => void;
  onOpen: (rule: AlertRule) => void;
  onLoadQuery: (rule: AlertRule) => void;
  onToggle: (rule: AlertRule) => void;
  onDelete: (rule: AlertRule) => void;
  onTest: () => void;
  onSave: () => void;
}) {
  const [alertSearch, setAlertSearch] = React.useState('');
  const filteredRules = React.useMemo(() => {
    const term = alertSearch.trim().toLowerCase();
    if (!term) return props.alertRules;
    return props.alertRules.filter((rule) => [
      rule.name,
      describeAlertCondition(rule),
      describeQueryMode(rule.query),
      rule.enabled ? 'enabled' : 'disabled'
    ].some((value) => value.toLowerCase().includes(term)));
  }, [alertSearch, props.alertRules]);

  return (
    <Section title="Alerts">
      {props.editingAlertId && <Tag color="blue">Editing existing rule</Tag>}
      <Field label="Rule"><Input value={props.alertName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="Query"><Tag>{props.queryMode === 'sql' ? 'Current SQL query' : 'Current builder query'}</Tag></Field>
      <Field label="Condition">
        <Space.Compact className="full">
          <Select className="operator-select" value={props.alertOperator} onChange={props.onOperator}>
            <Select.Option value="gt">&gt;</Select.Option>
            <Select.Option value="gte">&gt;=</Select.Option>
            <Select.Option value="lt">&lt;</Select.Option>
            <Select.Option value="lte">&lt;=</Select.Option>
            <Select.Option value="eq">=</Select.Option>
          </Select>
          <InputNumber className="full" min={0} value={Number(props.alertThreshold)} onChange={(value) => props.onThreshold(String(value ?? 0))} />
        </Space.Compact>
      </Field>
      <Field label="For">
        <Input value={props.alertFor} onChange={(event) => props.onFor(event.target.value)} placeholder="0s, 5m, 1h" />
      </Field>
      <Field label="Interval">
        <InputNumber className="full" min={10} max={86400} value={props.alertInterval} onChange={(value) => props.onInterval(Number(value || 60))} addonAfter="seconds" />
      </Field>
      <Field label="Enabled">
        <Switch checked={props.alertEnabled} onChange={props.onEnabled} />
      </Field>
      <Field label="Contact">
        <Select value={props.selectedContact} onChange={props.onContact}>
          <Select.Option value="">None</Select.Option>
          {props.contacts.map((contact) => <Select.Option key={contact.id} value={contact.id}>{contact.name}</Select.Option>)}
        </Select>
      </Field>
      {props.alertPreview && <Alert type={props.alertPreview.startsWith('Firing') ? 'warning' : 'success'} showIcon message={props.alertPreview} />}
      <Flex gap={8} wrap="wrap">
        <Button onClick={props.onNew}>New</Button>
        <Button disabled={!props.user} onClick={props.onTest}>Test</Button>
        <Button icon={<SaveOutlined />} disabled={!props.user} onClick={props.onSave}>{props.editingAlertId ? 'Update rule' : 'Save rule'}</Button>
      </Flex>
      <Field label="Find alert">
        <Input.Search allowClear value={alertSearch} onChange={(event) => setAlertSearch(event.target.value)} />
      </Field>
      <ActionList items={filteredRules} empty="No alert rules" list render={(rule) => (
        <List.Item key={rule.id}>
          <Flex gap={8} align="start" className="action-row full">
            <RowActions items={[
              { key: 'edit', label: 'Edit', onClick: () => props.onOpen(rule) },
              { key: 'load', label: 'Load query', onClick: () => props.onLoadQuery(rule) },
              { key: 'toggle', label: rule.enabled ? 'Disable' : 'Enable', onClick: () => props.onToggle(rule) },
              {
                key: 'delete',
                label: 'Delete',
                danger: true,
                confirm: {
                  title: 'Delete alert rule?',
                  content: 'Existing incidents remain for audit history.',
                  okText: 'Delete'
                },
                onClick: () => props.onDelete(rule)
              }
            ]} />
            <List.Item.Meta
              title={<Flex gap={6} align="center" wrap="wrap"><span>{rule.name}</span><Tag color={rule.enabled ? 'green' : undefined}>{rule.enabled ? 'enabled' : 'disabled'}</Tag></Flex>}
              description={`${describeAlertCondition(rule)} - every ${rule.interval_seconds || 60}s - ${describeQueryMode(rule.query)}`}
            />
          </Flex>
        </List.Item>
      )} />
    </Section>
  );
}

function describeAlertCondition(rule: AlertRule): string {
  const hold = rule.condition?.for ? ` for ${rule.condition.for}` : '';
  return `value ${operatorSymbol(rule.condition?.operator || 'gt')} ${rule.condition?.threshold ?? 0}${hold}`;
}

function operatorSymbol(operator: string): string {
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

function describeQueryMode(query: unknown): string {
  if (query && typeof query === 'object' && 'mode' in query && (query as { mode?: unknown }).mode === 'sql') return 'SQL';
  return 'Builder';
}

export function ContactsSection(props: {
  user: Principal | null;
  contacts: ContactEndpoint[];
  editingContactId: string;
  contactName: string;
  contactTarget: string;
  contactKind: ContactEndpoint['kind'];
  onName: (value: string) => void;
  onTarget: (value: string) => void;
  onKind: (value: ContactEndpoint['kind']) => void;
  onNew: () => void;
  onOpen: (contact: ContactEndpoint) => void;
  onUseForAlert: (contact: ContactEndpoint) => void;
  onSave: () => void;
  onDelete: (contact: ContactEndpoint) => void;
}) {
  const [contactSearch, setContactSearch] = React.useState('');
  const filteredContacts = React.useMemo(() => {
    const term = contactSearch.trim().toLowerCase();
    if (!term) return props.contacts;
    return props.contacts.filter((contact) => [
      contact.name,
      contact.kind,
      contact.target
    ].some((value) => value.toLowerCase().includes(term)));
  }, [contactSearch, props.contacts]);

  return (
    <Section title="Contacts">
      {props.editingContactId && <Tag color="blue">Editing existing contact</Tag>}
      <Field label="Name"><Input value={props.contactName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="Kind">
        <Select value={props.contactKind} onChange={props.onKind}>
          <Select.Option value="webhook">Webhook</Select.Option>
          <Select.Option value="pagerduty">PagerDuty</Select.Option>
          <Select.Option value="email">Email</Select.Option>
        </Select>
      </Field>
      <Field label="Target"><Input value={props.contactTarget} onChange={(event) => props.onTarget(event.target.value)} placeholder={contactTargetPlaceholder(props.contactKind)} /></Field>
      <Flex gap={8} wrap="wrap">
        <Button onClick={props.onNew}>New</Button>
        <Button icon={<SaveOutlined />} disabled={!props.user || !props.contactTarget} onClick={props.onSave}>{props.editingContactId ? 'Update contact' : 'Save contact'}</Button>
      </Flex>
      <Field label="Find contact">
        <Input.Search allowClear value={contactSearch} onChange={(event) => setContactSearch(event.target.value)} />
      </Field>
      <ActionList items={filteredContacts} empty="No contacts" list render={(contact) => (
        <List.Item key={contact.id}>
          <Flex gap={8} align="start" className="action-row full">
            <RowActions items={[
              { key: 'edit', label: 'Edit', onClick: () => props.onOpen(contact) },
              { key: 'use', label: 'Use in alert', onClick: () => props.onUseForAlert(contact) },
              {
                key: 'delete',
                label: 'Delete',
                danger: true,
                confirm: {
                  title: 'Delete contact?',
                  content: 'Alert rules using this contact will keep running without a contact.',
                  okText: 'Delete'
                },
                onClick: () => props.onDelete(contact)
              }
            ]} />
            <List.Item.Meta
              title={<Flex gap={6} align="center" wrap="wrap"><span>{contact.name}</span><Tag>{contact.kind}</Tag></Flex>}
              description={contact.target}
            />
          </Flex>
        </List.Item>
      )} />
    </Section>
  );
}

function contactTargetPlaceholder(kind: ContactEndpoint['kind']): string {
  switch (kind) {
    case 'email':
      return 'alerts@example.com';
    case 'pagerduty':
      return 'PagerDuty integration URL';
    default:
      return 'https://example.com/alerts';
  }
}

export function IncidentsSection({ incidents, onResolve }: { incidents: AlertIncident[]; onResolve: (incident: AlertIncident) => void }) {
  return (
    <Section title="Incidents">
      <ActionList items={incidents.slice(0, 8)} empty="No incidents" render={(incident) => (
        <List.Item key={incident.id} actions={incident.status === 'firing' ? [<Button key="resolve" size="small" onClick={() => onResolve(incident)}>Resolve</Button>] : []}>
          <List.Item.Meta
            title={<Space><Tag color={incident.status === 'firing' ? 'red' : 'green'}>{incident.status}</Tag><span>{incident.value} x{incident.occurrence_count || 1}</span></Space>}
            description={new Date(incident.last_seen_at || incident.created_at).toLocaleString()}
          />
        </List.Item>
      )} list />
    </Section>
  );
}

export function NotificationsSection({ notifications }: { notifications: AlertNotification[] }) {
  return (
    <Section title="Notifications">
      <ActionList items={notifications.slice(0, 8)} empty="No notifications" render={(notification) => (
        <List.Item key={notification.id}>
          <List.Item.Meta
            title={<Space><Tag color={notification.status === 'failed' ? 'red' : 'green'}>{notification.status}</Tag><span>{notification.contact_kind}</span></Space>}
            description={[notification.contact_target, notification.error, new Date(notification.created_at).toLocaleString()].filter(Boolean).join(' - ')}
          />
        </List.Item>
      )} list />
    </Section>
  );
}

export function InvitesSection(props: {
  user: Principal | null;
  invites: TenantInvite[];
  inviteEmail: string;
  inviteRole: TenantInvite['role'];
  inviteToken: string;
  onEmail: (value: string) => void;
  onRole: (value: TenantInvite['role']) => void;
  onToken: (value: string) => void;
  onAccept: () => void;
  onCreate: () => void;
  onDelete: (invite: TenantInvite) => void;
}) {
  const [inviteSearch, setInviteSearch] = React.useState('');
  const filteredInvites = React.useMemo(() => {
    const term = inviteSearch.trim().toLowerCase();
    if (!term) return props.invites;
    return props.invites.filter((invite) => [
      invite.email,
      invite.role,
      invite.accepted_at ? 'accepted' : 'pending',
      invite.token || ''
    ].some((value) => value.toLowerCase().includes(term)));
  }, [inviteSearch, props.invites]);

  return (
    <Section title="Invites">
      <Field label="Accept token"><Input value={props.inviteToken} onChange={(event) => props.onToken(event.target.value)} /></Field>
      <Button disabled={!props.user || !props.inviteToken} onClick={props.onAccept}>Accept invite</Button>
      <Field label="Email"><Input value={props.inviteEmail} onChange={(event) => props.onEmail(event.target.value)} /></Field>
      <Field label="Role">
        <Select value={props.inviteRole} onChange={props.onRole}>
          <Select.Option value="viewer">Viewer</Select.Option>
          <Select.Option value="editor">Editor</Select.Option>
          <Select.Option value="admin">Admin</Select.Option>
        </Select>
      </Field>
      <Button disabled={!props.user || !props.inviteEmail} onClick={props.onCreate}>Create invite</Button>
      <Field label="Find invite">
        <Input.Search allowClear value={inviteSearch} onChange={(event) => setInviteSearch(event.target.value)} />
      </Field>
      <ActionList items={filteredInvites} empty="No invites" render={(invite) => (
        <Flex key={invite.id} gap={6} align="stretch" className="action-row">
          <RowActions items={[
            {
              key: 'delete',
              label: 'Delete',
              danger: true,
              confirm: {
                title: 'Delete invite?',
                content: 'The invite token will no longer be accepted.',
                okText: 'Delete'
              },
              onClick: () => props.onDelete(invite)
            }
          ]} />
          <Button block className="stack-button">
            <span>{invite.email} - {invite.role}</span>
            <small>{invite.accepted_at ? `accepted ${new Date(invite.accepted_at).toLocaleString()}` : invite.token || `expires ${new Date(invite.expires_at).toLocaleString()}`}</small>
          </Button>
        </Flex>
      )} />
    </Section>
  );
}

export function MembersSection(props: {
  members: TenantMember[];
  onRole: (member: TenantMember, role: Role) => void;
  onDeactivate: (member: TenantMember) => void;
}) {
  const [memberSearch, setMemberSearch] = React.useState('');
  const filteredMembers = React.useMemo(() => {
    const term = memberSearch.trim().toLowerCase();
    if (!term) return props.members;
    return props.members.filter((member) => [
      member.email,
      member.display_name,
      member.provider,
      member.role,
      member.disabled_at ? 'disabled' : 'active'
    ].some((value) => value.toLowerCase().includes(term)));
  }, [memberSearch, props.members]);

  return (
    <Section title="Members">
      <Field label="Find member">
        <Input.Search allowClear value={memberSearch} onChange={(event) => setMemberSearch(event.target.value)} />
      </Field>
      <ActionList items={filteredMembers} empty="No members" render={(member) => (
        <List.Item key={member.id}>
          <Flex gap={8} align="start" className="action-row full">
            <RowActions items={[
              {
                key: 'deactivate',
                label: 'Deactivate',
                danger: true,
                disabled: Boolean(member.disabled_at),
                confirm: {
                  title: 'Deactivate member?',
                  content: 'The member will lose access to this tenant.',
                  okText: 'Deactivate'
                },
                onClick: () => props.onDeactivate(member)
              }
            ]} />
            <List.Item.Meta
              title={member.display_name || member.email}
              description={`${member.provider}${member.disabled_at ? ' - disabled' : ''}`}
            />
            <Select size="small" disabled={Boolean(member.disabled_at)} value={member.role} onChange={(role) => props.onRole(member, role)} className="role-select">
              <Select.Option value="owner">Owner</Select.Option>
              <Select.Option value="admin">Admin</Select.Option>
              <Select.Option value="editor">Editor</Select.Option>
              <Select.Option value="viewer">Viewer</Select.Option>
            </Select>
          </Flex>
        </List.Item>
      )} list />
    </Section>
  );
}

export function AuditSection({ auditEvents }: { auditEvents: AuditEvent[] }) {
  return (
    <Section title="Audit">
      <ActionList items={auditEvents.slice(0, 10)} empty="No audit events" render={(event) => (
        <List.Item key={event.id}>
          <List.Item.Meta
            title={event.action}
            description={`${event.actor_email || 'system'} - ${event.target_type}${event.target_id ? ` - ${event.target_id}` : ''} - ${new Date(event.created_at).toLocaleString()}`}
          />
        </List.Item>
      )} list />
    </Section>
  );
}

export function ControlSections(props: {
  items: { key: string; label: string; children: React.ReactNode }[];
}) {
  return <Collapse bordered={false} defaultActiveKey={['access', 'settings', 'query']} items={props.items} />;
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Space direction="vertical" size={10} className="section-body">
      <Typography.Text strong>{title}</Typography.Text>
      {children}
    </Space>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
    </label>
  );
}

function ActionList<T>({
  items,
  empty,
  render,
  list
}: {
  items: T[];
  empty: string;
  render: (item: T, index: number) => React.ReactNode;
  list?: boolean;
}) {
  if (items.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={empty} />;
  if (list) return <List size="small" dataSource={items} renderItem={(item, index) => render(item, index) as React.ReactElement} />;
  return <Space direction="vertical" size={6} className="full">{items.map(render)}</Space>;
}

function RowActions({ items }: { items: RowAction[] }) {
  return (
    <Dropdown
      trigger={['click']}
      menu={{
        items: items.map((item) => ({
          key: item.key,
          label: item.label,
          icon: item.icon || defaultRowActionIcon(item.key),
          danger: item.danger,
          disabled: item.disabled
        })),
        onClick: ({ key }) => {
          const action = items.find((item) => item.key === key);
          if (!action || action.disabled) return;
          if (action.confirm) {
            Modal.confirm({
              title: action.confirm.title,
              content: action.confirm.content,
              okText: action.confirm.okText || 'Confirm',
              okButtonProps: { danger: action.danger },
              onOk: action.onClick
            });
            return;
          }
          action.onClick();
        }
      }}
    >
      <Button className="row-action-trigger" icon={<MoreOutlined />} />
    </Dropdown>
  );
}

function defaultRowActionIcon(key: string): React.ReactNode {
  if (key.includes('delete') || key.includes('remove') || key.includes('deactivate')) return <DeleteOutlined />;
  if (key.includes('copy') || key.includes('duplicate')) return <CopyOutlined />;
  if (key.includes('edit') || key.includes('open')) return <EditOutlined />;
  if (key.includes('move-up')) return <UpOutlined />;
  if (key.includes('move-down')) return <DownOutlined />;
  if (key.includes('load') || key.includes('use')) return <PlayCircleOutlined />;
  if (key.includes('toggle')) return <CheckCircleOutlined />;
  return undefined;
}

function firstDimension(config: PublicConfig | null, datasetID: string): string {
  return config?.datasets.find((item) => item.id === datasetID)?.dimensions[0] || '';
}

function defaultDatasetSQL(datasetID: string) {
  if (datasetID === 'metrics') {
    return 'SELECT service_name, avg(value) AS value\nFROM otel_metrics\nWHERE tenant_id = {tenant:String}\n  AND timestamp >= {from:DateTime}\n  AND timestamp < {to:DateTime}\nGROUP BY service_name\nORDER BY value DESC';
  }
  if (datasetID === 'traces') {
    return 'SELECT service_name, count() AS value\nFROM otel_traces\nWHERE tenant_id = {tenant:String}\n  AND timestamp >= {from:DateTime}\n  AND timestamp < {to:DateTime}\nGROUP BY service_name\nORDER BY value DESC';
  }
  return 'SELECT service_name, severity, count() AS value\nFROM otel_logs\nWHERE tenant_id = {tenant:String}\n  AND timestamp >= {from:DateTime}\n  AND timestamp < {to:DateTime}\nGROUP BY service_name, severity\nORDER BY value DESC';
}

function filterOperators(dataset: Dataset | undefined, field: string): string[] {
  return dataset?.filterOperators?.[field] || ['eq'];
}

function operatorLabel(operator: string): string {
  switch (operator) {
    case 'contains':
      return 'contains';
    case 'prefix':
      return 'starts';
    default:
      return '=';
  }
}
