import type { PermissionMap } from '@/types/access';

export type { PermissionMap } from '@/types/access';

export type NavigationIconName =
  | 'LayoutDashboard'
  | 'GitPullRequest'
  | 'CalendarDays'
  | 'History'
  | 'Library'
  | 'ScrollText'
  | 'FolderCheck'
  | 'ClipboardCheck'
  | 'ShieldOff'
  | 'Wrench'
  | 'AlertTriangle'
  | 'AlertOctagon'
  | 'Activity'
  | 'Zap'
  | 'ShieldCheck'
  | 'Scale'
  | 'Building2'
  | 'ClipboardList'
  | 'Server'
  | 'Database'
  | 'UserCheck'
  | 'Users'
  | 'LineChart'
  | 'BarChart3'
  | 'BookOpen'
  | 'Store'
  | 'Inbox'
  | 'Bell'
  | 'PlugZap'
  | 'Settings';

export interface NavigationContext {
  isSuperAdmin?: boolean;
  permissions?: PermissionMap;
  roleSlugs: readonly string[];
}

export interface NavigationItem {
  id: string;
  label: string;
  href: string;
  icon: NavigationIconName;
  keywords: readonly string[];
  permission?: {
    resource: string;
    action?: string;
  };
  fallbackRoles?: readonly string[];
}

export interface NavigationGroup {
  id: string;
  label: string;
  defaultOpen?: boolean;
  items: readonly NavigationItem[];
}

export const NAVIGATION_GROUPS: readonly NavigationGroup[] = [
  {
    id: 'workspace',
    label: 'Workspace',
    defaultOpen: true,
    items: [
      { id: 'dashboard', label: 'Dashboard', href: '/dashboard', icon: 'LayoutDashboard', keywords: ['home', 'overview'], permission: { resource: 'reports' } },
      { id: 'workflows', label: 'My Workflows', href: '/workflows', icon: 'GitPullRequest', keywords: ['approvals', 'tasks'], permission: { resource: 'policies' } },
      { id: 'calendar', label: 'Compliance Calendar', href: '/calendar', icon: 'CalendarDays', keywords: ['deadlines', 'schedule'], permission: { resource: 'audits' } },
      { id: 'activity', label: 'Activity', href: '/activity', icon: 'History', keywords: ['audit trail', 'recent'], permission: { resource: 'reports' } },
      { id: 'notification-center', label: 'Notifications', href: '/notifications', icon: 'Inbox', keywords: ['alerts', 'inbox', 'unread'], permission: { resource: 'users', action: 'read' } },
    ],
  },
  {
    id: 'assurance',
    label: 'Compliance & Assurance',
    defaultOpen: true,
    items: [
      { id: 'frameworks', label: 'Frameworks', href: '/frameworks', icon: 'Library', keywords: ['standards', 'controls'], permission: { resource: 'frameworks' } },
      { id: 'policies', label: 'Policies', href: '/policies', icon: 'ScrollText', keywords: ['documents', 'attestation'], permission: { resource: 'policies' } },
      { id: 'evidence', label: 'Evidence', href: '/evidence', icon: 'FolderCheck', keywords: ['artifacts', 'collection'], permission: { resource: 'controls' } },
      { id: 'audits', label: 'Audits', href: '/audits', icon: 'ClipboardCheck', keywords: ['findings', 'assurance'], permission: { resource: 'audits' } },
      { id: 'exceptions', label: 'Exceptions', href: '/exceptions', icon: 'ShieldOff', keywords: ['waivers', 'acceptance'], permission: { resource: 'risks' } },
      { id: 'remediation', label: 'Remediation', href: '/remediation', icon: 'Wrench', keywords: ['actions', 'findings'], permission: { resource: 'controls' } },
    ],
  },
  {
    id: 'risk-resilience',
    label: 'Risk & Resilience',
    items: [
      { id: 'risks', label: 'Risk Register', href: '/risks', icon: 'AlertTriangle', keywords: ['treatment', 'residual'], permission: { resource: 'risks' } },
      { id: 'incidents', label: 'Incidents', href: '/incidents', icon: 'AlertOctagon', keywords: ['breach', 'security'], permission: { resource: 'incidents' } },
      { id: 'monitoring', label: 'Continuous Monitoring', href: '/monitoring', icon: 'Activity', keywords: ['drift', 'alerts'], permission: { resource: 'controls' } },
      { id: 'bia', label: 'Business Impact', href: '/bia', icon: 'Zap', keywords: ['continuity', 'recovery'], permission: { resource: 'risks' } },
      { id: 'nis2', label: 'NIS2', href: '/nis2', icon: 'ShieldCheck', keywords: ['directive', 'cyber'], permission: { resource: 'controls' } },
      { id: 'regulatory', label: 'Regulatory Change', href: '/regulatory', icon: 'Scale', keywords: ['obligations', 'change'], permission: { resource: 'frameworks' } },
    ],
  },
  {
    id: 'ecosystem-data',
    label: 'Third Parties & Data',
    items: [
      { id: 'vendors', label: 'Vendors', href: '/vendors', icon: 'Building2', keywords: ['third party', 'supplier'], permission: { resource: 'vendors' } },
      { id: 'vendor-assessments', label: 'Vendor Assessments', href: '/vendor-assessments', icon: 'ClipboardList', keywords: ['questionnaires', 'due diligence'], permission: { resource: 'vendors' } },
      { id: 'assets', label: 'Assets', href: '/assets', icon: 'Server', keywords: ['inventory', 'systems'], permission: { resource: 'assets' } },
      { id: 'data', label: 'Data Governance', href: '/data', icon: 'Database', keywords: ['ropa', 'processing'], permission: { resource: 'incidents' } },
      { id: 'dsr', label: 'Data Subject Requests', href: '/dsr', icon: 'UserCheck', keywords: ['privacy', 'gdpr'], permission: { resource: 'incidents' } },
    ],
  },
  {
    id: 'leadership',
    label: 'Leadership & Insights',
    items: [
      { id: 'board', label: 'Board Governance', href: '/board', icon: 'Users', keywords: ['executive', 'decisions'], permission: { resource: 'reports' }, fallbackRoles: ['org_admin', 'ciso', 'compliance_manager', 'risk_manager'] },
      { id: 'analytics', label: 'Analytics', href: '/analytics', icon: 'LineChart', keywords: ['trends', 'metrics'], permission: { resource: 'reports' } },
      { id: 'reports', label: 'Reports', href: '/reports', icon: 'BarChart3', keywords: ['export', 'insights'], permission: { resource: 'reports' } },
      { id: 'knowledge', label: 'Knowledge Base', href: '/knowledge', icon: 'BookOpen', keywords: ['guidance', 'articles'], permission: { resource: 'policies' } },
      { id: 'marketplace', label: 'Framework Marketplace', href: '/marketplace', icon: 'Store', keywords: ['catalog', 'standards'], permission: { resource: 'frameworks' } },
    ],
  },
  {
    id: 'administration',
    label: 'Administration',
    items: [
      { id: 'notifications', label: 'Notification Administration', href: '/settings/notifications', icon: 'Bell', keywords: ['alerts', 'rules', 'channels', 'templates'], permission: { resource: 'settings', action: 'read' }, fallbackRoles: ['org_admin'] },
      { id: 'integrations', label: 'Integration Hub', href: '/settings/integrations', icon: 'PlugZap', keywords: ['connectors', 'sso', 'api keys'], permission: { resource: 'settings', action: 'read' }, fallbackRoles: ['org_admin'] },
      { id: 'settings', label: 'Organization Settings', href: '/settings', icon: 'Settings', keywords: ['users', 'roles', 'configuration'], permission: { resource: 'settings', action: 'read' }, fallbackRoles: ['org_admin'] },
    ],
  },
] as const;

export const NAVIGATION_ITEMS: readonly NavigationItem[] =
  NAVIGATION_GROUPS.flatMap((group) => group.items);

export function canViewNavigationItem(
  item: NavigationItem,
  context: NavigationContext
): boolean {
  if (context.isSuperAdmin) return true;

  if (item.permission && context.permissions !== undefined) {
    const actions = context.permissions[item.permission.resource];
    if (actions !== undefined) {
      return actions.includes(item.permission.action ?? 'read');
    }
    if (!item.fallbackRoles) return false;
  }

  if (item.fallbackRoles) {
    return item.fallbackRoles.some((role) => context.roleSlugs.includes(role));
  }

  return true;
}

export function getVisibleNavigationGroups(
  context: NavigationContext
): NavigationGroup[] {
  return NAVIGATION_GROUPS.map((group) => ({
    ...group,
    items: group.items.filter((item) => canViewNavigationItem(item, context)),
  })).filter((group) => group.items.length > 0);
}

export function getNavigationItemForPath(
  pathname: string,
  items: readonly NavigationItem[] = NAVIGATION_ITEMS
): NavigationItem | undefined {
  return [...items]
    .sort((left, right) => right.href.length - left.href.length)
    .find(
      (item) => pathname === item.href || pathname.startsWith(`${item.href}/`)
    );
}

export function normalizePermissionMap(value: unknown): PermissionMap | undefined {
  if (!value || typeof value !== 'object') return undefined;

  const record = value as Record<string, unknown>;
  const candidate =
    record.data && typeof record.data === 'object'
      ? (record.data as Record<string, unknown>)
      : record;
  const permissions: PermissionMap = {};

  for (const [resource, actions] of Object.entries(candidate)) {
    if (Array.isArray(actions)) {
      permissions[resource] = actions.filter(
        (action): action is string => typeof action === 'string'
      );
    }
  }

  return permissions;
}

export function getRoleSlugs(
  roles: readonly ({ slug?: string } | string)[] | undefined
): string[] {
  return (roles ?? [])
    .map((role) => (typeof role === 'string' ? role : role.slug))
    .filter((slug): slug is string => Boolean(slug));
}

export interface GlobalSearchResult {
  id: string;
  entity_type: string;
  entity_ref?: string;
  title: string;
  snippet?: string;
  status?: string;
  severity?: string;
  framework?: string;
  updated_at?: string;
  score?: number;
}

export interface GlobalSearchResponse {
  items: GlobalSearchResult[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
  query_time_ms?: number;
  suggestions?: string[];
}

export interface GlobalAutocompleteResult {
  entity_type: string;
  entity_id: string;
  title: string;
  subtitle?: string;
  entity_ref?: string;
}

export function normalizeGlobalAutocompleteResults(
  value: unknown
): GlobalAutocompleteResult[] {
  const response =
    value && typeof value === 'object'
      ? (value as Record<string, unknown>)
      : {};
  const rawItems = Array.isArray(value)
    ? value
    : Array.isArray(response.data)
      ? response.data
      : [];

  return rawItems
    .filter(
      (item): item is Record<string, unknown> =>
        Boolean(item) && typeof item === 'object'
    )
    .map((item) => ({
      entity_type: String(item.entity_type ?? ''),
      entity_id: String(item.entity_id ?? item.id ?? ''),
      title: String(item.title ?? ''),
      subtitle:
        typeof item.subtitle === 'string' ? item.subtitle : undefined,
      entity_ref:
        typeof item.entity_ref === 'string' ? item.entity_ref : undefined,
    }))
    .filter((item) => item.entity_type && item.entity_id && item.title);
}

export function normalizeGlobalSearchResponse(value: unknown): GlobalSearchResponse {
  const response =
    value && typeof value === 'object'
      ? (value as Record<string, unknown>)
      : {};
  const rawItems = Array.isArray(response.items)
    ? response.items
    : Array.isArray(response.results)
      ? response.results
      : [];
  const items = rawItems
    .filter(
      (item): item is Record<string, unknown> =>
        Boolean(item) && typeof item === 'object'
    )
    .map((item) => ({
      id: String(item.id ?? item.entity_id ?? ''),
      entity_type: String(item.entity_type ?? ''),
      entity_ref:
        typeof item.entity_ref === 'string' ? item.entity_ref : undefined,
      title: String(item.title ?? ''),
      snippet:
        typeof item.snippet === 'string'
          ? item.snippet
          : typeof item.description === 'string'
            ? item.description
            : undefined,
      status: typeof item.status === 'string' ? item.status : undefined,
      severity: typeof item.severity === 'string' ? item.severity : undefined,
      framework:
        typeof item.framework === 'string' ? item.framework : undefined,
      updated_at:
        typeof item.updated_at === 'string' ? item.updated_at : undefined,
      score: typeof item.score === 'number' ? item.score : undefined,
    }))
    .filter((item) => item.id && item.entity_type && item.title);
  const total = Number(
    response.total ?? response.total_hits ?? response.total_count ?? items.length
  );
  const page = Number(response.page ?? 1);
  const pageSize = Number(response.page_size ?? Math.max(items.length, 1));

  return {
    items,
    total,
    page,
    page_size: pageSize,
    total_pages: Number(
      response.total_pages ?? Math.ceil(total / Math.max(pageSize, 1))
    ),
    query_time_ms:
      typeof response.query_time_ms === 'number'
        ? response.query_time_ms
        : typeof response.time_taken_ms === 'number'
          ? response.time_taken_ms
          : undefined,
    suggestions: Array.isArray(response.suggestions)
      ? response.suggestions.filter(
          (suggestion): suggestion is string => typeof suggestion === 'string'
        )
      : undefined,
  };
}

const ENTITY_DETAIL_ROUTES: Record<string, string> = {
  framework: '/frameworks',
  control: '/controls',
  risk: '/risks',
  policy: '/policies',
  audit: '/audits',
  incident: '/incidents',
  vendor: '/vendors',
  dsr: '/dsr',
  asset: '/assets',
};

export function buildGlobalSearchRoute(
  query: string,
  entityType?: string
): string {
  const params = new URLSearchParams({ q: query });
  if (entityType) params.set('entity_type', entityType);
  return `/search?${params.toString()}`;
}

export function getEntityRoute(
  result: GlobalSearchResult | GlobalAutocompleteResult
): string {
  const baseRoute = ENTITY_DETAIL_ROUTES[result.entity_type];
  const id = 'id' in result ? result.id : result.entity_id;
  if (baseRoute && id) {
    return `${baseRoute}/${encodeURIComponent(id)}`;
  }

  return buildGlobalSearchRoute(result.title, result.entity_type);
}
