import {
  Alert,
  Button,
  Collapse,
  Empty,
  Flex,
  Input,
  InputNumber,
  List,
  Select,
  Segmented,
  Space,
  Switch,
  Tag,
  Typography
} from 'antd';
import {
  CheckCircleOutlined,
  DeleteOutlined,
  LoginOutlined,
  PlayCircleOutlined,
  SaveOutlined
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
import { JwtClaims, QueryState, RelativeRange, RelativeRangeUnit, VisualizationType } from '../types';

type Role = 'owner' | 'admin' | 'editor' | 'viewer';

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
  onSave: () => void;
  onTest: () => void;
  onOpen: (source: DataSource) => void;
}) {
  return (
    <Section title="Sources">
      <Field label="Name"><Input value={props.sourceName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="URL"><Input value={props.sourceURL} onChange={(event) => props.onURL(event.target.value)} /></Field>
      <Field label="Database"><Input value={props.sourceDatabase} onChange={(event) => props.onDatabase(event.target.value)} /></Field>
      <Field label="User"><Input value={props.sourceUser} onChange={(event) => props.onUser(event.target.value)} /></Field>
      <Field label="Secret"><Input value={props.sourceSecretRef} onChange={(event) => props.onSecretRef(event.target.value)} /></Field>
      <Flex gap={8}>
        <Button icon={<SaveOutlined />} disabled={!props.user || !props.sourceURL} onClick={props.onSave}>Save</Button>
        <Button disabled={!props.user || !props.editingSourceId} onClick={props.onTest}>Test</Button>
      </Flex>
      {props.sourceStatus && <Alert type="info" showIcon message={props.sourceStatus} />}
      <ActionList items={props.dataSources} empty="No sources" render={(source) => (
        <Button block key={source.id} onClick={() => props.onOpen(source)}>{source.name} - {source.kind}</Button>
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
            aggregation: next?.defaultAggregation || 'count'
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
  savedQueryName: string;
  savedQueryDescription: string;
  onName: (value: string) => void;
  onDescription: (value: string) => void;
  onSave: () => void;
  onOpen: (query: SavedQuery) => void;
}) {
  return (
    <Section title="Saved Queries">
      <Field label="Name"><Input value={props.savedQueryName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="Description"><Input.TextArea value={props.savedQueryDescription} onChange={(event) => props.onDescription(event.target.value)} /></Field>
      <Button icon={<SaveOutlined />} disabled={!props.user || !props.savedQueryName} onClick={props.onSave}>Save query</Button>
      <ActionList items={props.savedQueries} empty="No saved queries" render={(query) => (
        <Button block key={query.id} onClick={() => props.onOpen(query)}>{query.name}</Button>
      )} />
    </Section>
  );
}

export function DashboardsSection(props: {
  user: Principal | null;
  dashboards: Dashboard[];
  dashboardPanels: DashboardChart[];
  dashboardName: string;
  panelTitle: string;
  panelVisualization: VisualizationType;
  onDashboardName: (value: string) => void;
  onPanelTitle: (value: string) => void;
  onPanelVisualization: (value: VisualizationType) => void;
  onAddPanel: () => void;
  onSave: () => void;
  onOpen: (dashboard: Dashboard) => void;
  onOpenPanel: (panel: DashboardChart) => void;
  onRemovePanel: (index: number) => void;
}) {
  return (
    <Section title="Dashboards">
      <Field label="Name"><Input value={props.dashboardName} onChange={(event) => props.onDashboardName(event.target.value)} /></Field>
      <Field label="Panel title"><Input value={props.panelTitle} onChange={(event) => props.onPanelTitle(event.target.value)} /></Field>
      <Field label="Visualization">
        <Select value={props.panelVisualization} onChange={props.onPanelVisualization}>
          <Select.Option value="line">Line</Select.Option>
          <Select.Option value="area">Area</Select.Option>
          <Select.Option value="bar">Bar</Select.Option>
        </Select>
      </Field>
      <Flex gap={8}>
        <Button disabled={!props.user} onClick={props.onAddPanel}>Add panel</Button>
        <Button icon={<SaveOutlined />} disabled={!props.user || !props.dashboardName} onClick={props.onSave}>Save</Button>
      </Flex>
      <ActionList items={props.dashboardPanels} empty="No staged panels" render={(panel, index) => (
        <Flex key={panel.id || `${panel.title}-${index}`} gap={8}>
          <Button className="grow" onClick={() => props.onOpenPanel(panel)}>{panel.title}</Button>
          <Button icon={<DeleteOutlined />} danger onClick={() => props.onRemovePanel(index)} />
        </Flex>
      )} />
      <ActionList items={props.dashboards} empty="No dashboards" render={(dashboard) => (
        <Button block key={dashboard.id} onClick={() => props.onOpen(dashboard)}>{dashboard.name}</Button>
      )} />
    </Section>
  );
}

export function AlertsSection(props: {
  user: Principal | null;
  alertRules: AlertRule[];
  contacts: ContactEndpoint[];
  editingAlertId: string;
  alertName: string;
  alertThreshold: string;
  alertOperator: string;
  alertInterval: number;
  alertEnabled: boolean;
  alertPreview: string;
  selectedContact: string;
  queryMode: string;
  onName: (value: string) => void;
  onThreshold: (value: string) => void;
  onOperator: (value: string) => void;
  onInterval: (value: number) => void;
  onEnabled: (value: boolean) => void;
  onContact: (value: string) => void;
  onNew: () => void;
  onOpen: (rule: AlertRule) => void;
  onLoadQuery: (rule: AlertRule) => void;
  onToggle: (rule: AlertRule) => void;
  onTest: () => void;
  onSave: () => void;
}) {
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
      <ActionList items={props.alertRules} empty="No alert rules" list render={(rule) => (
        <List.Item
          key={rule.id}
          actions={[
            <Button key="edit" size="small" onClick={() => props.onOpen(rule)}>Edit</Button>,
            <Button key="load" size="small" onClick={() => props.onLoadQuery(rule)}>Load query</Button>,
            <Button key="toggle" size="small" onClick={() => props.onToggle(rule)}>{rule.enabled ? 'Disable' : 'Enable'}</Button>
          ]}
        >
          <List.Item.Meta
            title={<Flex gap={6} align="center" wrap="wrap"><span>{rule.name}</span><Tag color={rule.enabled ? 'green' : undefined}>{rule.enabled ? 'enabled' : 'disabled'}</Tag></Flex>}
            description={`${describeAlertCondition(rule)} - every ${rule.interval_seconds || 60}s - ${describeQueryMode(rule.query)}`}
          />
        </List.Item>
      )} />
    </Section>
  );
}

function describeAlertCondition(rule: AlertRule): string {
  return `value ${operatorSymbol(rule.condition?.operator || 'gt')} ${rule.condition?.threshold ?? 0}`;
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
  contactName: string;
  contactTarget: string;
  contactKind: ContactEndpoint['kind'];
  onName: (value: string) => void;
  onTarget: (value: string) => void;
  onKind: (value: ContactEndpoint['kind']) => void;
  onSave: () => void;
}) {
  return (
    <Section title="Contacts">
      <Field label="Name"><Input value={props.contactName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="Kind">
        <Select value={props.contactKind} onChange={props.onKind}>
          <Select.Option value="webhook">Webhook</Select.Option>
          <Select.Option value="pagerduty">PagerDuty</Select.Option>
          <Select.Option value="email">Email</Select.Option>
        </Select>
      </Field>
      <Field label="Target"><Input value={props.contactTarget} onChange={(event) => props.onTarget(event.target.value)} /></Field>
      <Button icon={<SaveOutlined />} disabled={!props.user || !props.contactTarget} onClick={props.onSave}>Save contact</Button>
    </Section>
  );
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
}) {
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
      <ActionList items={props.invites} empty="No invites" render={(invite) => (
        <Button block className="stack-button" key={invite.id}>
          <span>{invite.email} - {invite.role}</span>
          {invite.token && <small>{invite.token}</small>}
        </Button>
      )} />
    </Section>
  );
}

export function MembersSection(props: {
  members: TenantMember[];
  onRole: (member: TenantMember, role: Role) => void;
  onDeactivate: (member: TenantMember) => void;
}) {
  return (
    <Section title="Members">
      <ActionList items={props.members} empty="No members" render={(member) => (
        <List.Item
          key={member.id}
          actions={[
            <Select key="role" size="small" disabled={Boolean(member.disabled_at)} value={member.role} onChange={(role) => props.onRole(member, role)} className="role-select">
              <Select.Option value="owner">Owner</Select.Option>
              <Select.Option value="admin">Admin</Select.Option>
              <Select.Option value="editor">Editor</Select.Option>
              <Select.Option value="viewer">Viewer</Select.Option>
            </Select>,
            !member.disabled_at && <Button key="disable" size="small" danger onClick={() => props.onDeactivate(member)}>Deactivate</Button>
          ]}
        >
          <List.Item.Meta
            title={member.display_name || member.email}
            description={`${member.provider}${member.disabled_at ? ' - disabled' : ''}`}
          />
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
  return <Collapse bordered={false} defaultActiveKey={['access', 'query']} items={props.items} />;
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
