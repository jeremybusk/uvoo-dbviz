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
  Space,
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
import type React from 'react';
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
import { QueryState, VisualizationType } from '../types';

type Role = 'owner' | 'admin' | 'editor' | 'viewer';

export function AccessSection(props: {
  config: PublicConfig | null;
  user: Principal | null;
  profile: UserProfile | null;
  activeTenant: string;
  memberships: TenantMembership[];
  tokenInput: string;
  onTokenInput: (value: string) => void;
  onLogin: (provider: Provider) => void;
  onSaveToken: () => void;
  onSelectTenant: (tenant: string) => void;
  onSignOut: () => void;
}) {
  const tenantValue = props.activeTenant || props.user?.tenantId || '';
  if (props.user) {
    return (
      <Section title="Access">
        <Space direction="vertical" size={8} className="full">
          <Typography.Text strong>{props.user.name || props.user.email}</Typography.Text>
          <Typography.Text type="secondary">
            {props.profile?.role || 'member'} in {props.profile?.tenant_slug || tenantValue}
          </Typography.Text>
          <Select value={tenantValue} onChange={props.onSelectTenant}>
            {props.memberships.length === 0 && <Select.Option value={tenantValue}>{tenantValue}</Select.Option>}
            {props.memberships.map((membership) => (
              <Select.Option key={membership.tenant_slug} value={membership.tenant_slug}>
                {membership.tenant_name} ({membership.role})
              </Select.Option>
            ))}
          </Select>
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
      </Space>
    </Section>
  );
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
  onQuery: (query: QueryState) => void;
  onSource: (source: DataSource) => void;
  onRun: () => void;
}) {
  const q = props.query;
  return (
    <Section title="Query">
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
            groupBy: firstDimension(props.config, value),
            measure: next?.defaultMeasure || '_rows',
            aggregation: next?.defaultAggregation || 'count'
          });
        }}>
          {props.config?.datasets.map((item) => <Select.Option key={item.id} value={item.id}>{item.name}</Select.Option>)}
        </Select>
      </Field>
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
      <Field label="From"><Input type="datetime-local" value={q.from} onChange={(event) => props.onQuery({ ...q, from: event.target.value })} /></Field>
      <Field label="To"><Input type="datetime-local" value={q.to} onChange={(event) => props.onQuery({ ...q, to: event.target.value })} /></Field>
      <Button type="primary" icon={<PlayCircleOutlined />} disabled={!props.user} onClick={props.onRun}>Run</Button>
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
  alertName: string;
  alertThreshold: string;
  selectedContact: string;
  onName: (value: string) => void;
  onThreshold: (value: string) => void;
  onContact: (value: string) => void;
  onSave: () => void;
}) {
  return (
    <Section title="Alerts">
      <Field label="Rule"><Input value={props.alertName} onChange={(event) => props.onName(event.target.value)} /></Field>
      <Field label="Threshold"><InputNumber className="full" min={0} value={Number(props.alertThreshold)} onChange={(value) => props.onThreshold(String(value ?? 0))} /></Field>
      <Field label="Contact">
        <Select value={props.selectedContact} onChange={props.onContact}>
          <Select.Option value="">None</Select.Option>
          {props.contacts.map((contact) => <Select.Option key={contact.id} value={contact.id}>{contact.name}</Select.Option>)}
        </Select>
      </Field>
      <Button icon={<SaveOutlined />} disabled={!props.user} onClick={props.onSave}>Save rule</Button>
      <ActionList items={props.alertRules} empty="No alert rules" render={(rule) => (
        <Button block key={rule.id} onClick={() => {
          props.onName(rule.name);
          props.onThreshold(String(rule.condition?.threshold ?? 0));
          props.onContact(rule.contact_endpoint_id || '');
        }}>{rule.name}</Button>
      )} />
    </Section>
  );
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
