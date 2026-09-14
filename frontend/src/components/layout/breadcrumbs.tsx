'use client';

import { ChevronRight, Home } from 'lucide-react';
import { getNavigationItemForPath, NAVIGATION_ITEMS } from '@/lib/navigation';
import { cn } from '@/lib/utils';
import { Fragment } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';

const SEGMENT_LABELS: Record<string, string> = {
  activity: 'Activity',
  analytics: 'Analytics',
  assets: 'Assets',
  attestations: 'Attestations',
  'audit-log': 'Audit log',
  audits: 'Audits',
  bia: 'Business impact',
  board: 'Board governance',
  branding: 'Branding',
  calendar: 'Compliance calendar',
  controls: 'Controls',
  dashboard: 'Dashboard',
  data: 'Data governance',
  dsr: 'Data subject requests',
  edit: 'Edit',
  evidence: 'Evidence',
  exceptions: 'Exceptions',
  findings: 'Findings',
  frameworks: 'Frameworks',
  'gap-analysis': 'Gap analysis',
  incidents: 'Incidents',
  integrations: 'Integrations',
  knowledge: 'Knowledge base',
  marketplace: 'Framework marketplace',
  monitoring: 'Continuous monitoring',
  new: 'New',
  nis2: 'NIS2',
  notifications: 'Notification settings',
  policies: 'Policies',
  profile: 'Profile',
  regulatory: 'Regulatory change',
  remediation: 'Remediation',
  reports: 'Reports',
  risks: 'Risk register',
  search: 'Search',
  settings: 'Settings',
  subscription: 'Subscription',
  treatments: 'Treatments',
  'vendor-assessments': 'Vendor assessments',
  vendors: 'Vendors',
  versions: 'Versions',
  workflows: 'My workflows',
};

const DETAIL_LABELS: Record<string, string> = {
  audits: 'Audit details',
  controls: 'Control details',
  dsr: 'Request details',
  frameworks: 'Framework details',
  incidents: 'Incident details',
  policies: 'Policy details',
  risks: 'Risk details',
  vendors: 'Vendor details',
};

function isUuid(segment: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    segment
  );
}

function titleCaseSegment(segment: string): string {
  return decodeURIComponent(segment)
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

export interface BreadcrumbItem {
  href: string;
  label: string;
}

export function buildBreadcrumbItems(
  pathname: string,
  dynamicLabels?: Record<string, string>
): BreadcrumbItem[] {
  const segments = pathname.split('/').filter(Boolean);
  if (segments.length === 0) return [];

  return segments.map((segment, index) => {
    const href = `/${segments.slice(0, index + 1).join('/')}`;
    const navigationItem = getNavigationItemForPath(
      href,
      NAVIGATION_ITEMS.filter((item) => item.href === href)
    );
    const label =
      dynamicLabels?.[segment] ??
      navigationItem?.label ??
      SEGMENT_LABELS[segment] ??
      (isUuid(segment)
        ? DETAIL_LABELS[segments[0]] ?? 'Record details'
        : titleCaseSegment(segment));

    return { href, label };
  });
}

interface BreadcrumbsProps {
  className?: string;
  compact?: boolean;
  /** Override a dynamic path segment with its loaded entity name. */
  dynamicLabels?: Record<string, string>;
}

export function Breadcrumbs({
  className,
  compact = false,
  dynamicLabels,
}: BreadcrumbsProps) {
  const pathname = usePathname();
  const crumbs = buildBreadcrumbItems(pathname, dynamicLabels);
  const current = crumbs.at(-1);

  if (!current) return null;

  if (compact) {
    return (
      <nav aria-label="Breadcrumb" className={cn('min-w-0', className)}>
        <span
          aria-current="page"
          title={current.label}
          className="block truncate text-sm font-medium text-foreground"
        >
          {current.label}
        </span>
      </nav>
    );
  }

  const isDashboard = pathname === '/dashboard';

  return (
    <nav
      aria-label="Breadcrumb"
      className={cn('min-w-0 text-sm text-muted-foreground', className)}
    >
      <ol className="flex min-w-0 items-center">
        <li className="shrink-0">
          {isDashboard ? (
            <span
              aria-current="page"
              className="flex items-center gap-2 font-medium text-foreground"
            >
              <Home aria-hidden="true" className="h-4 w-4" />
              <span>Dashboard</span>
            </span>
          ) : (
            <Link
              href="/dashboard"
              aria-label="Dashboard"
              className="rounded-sm transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Home aria-hidden="true" className="h-4 w-4" />
            </Link>
          )}
        </li>
        {!isDashboard &&
          crumbs.map((crumb, index) => {
            const isLast = index === crumbs.length - 1;
            return (
              <Fragment key={crumb.href}>
                <li aria-hidden="true" className="shrink-0">
                  <ChevronRight className="mx-2 h-3.5 w-3.5" />
                </li>
                <li className="min-w-0">
                  {isLast ? (
                    <span
                      aria-current="page"
                      title={crumb.label}
                      className="block max-w-[14rem] truncate font-medium text-foreground"
                    >
                      {crumb.label}
                    </span>
                  ) : (
                    <Link
                      href={crumb.href}
                      title={crumb.label}
                      className="block max-w-[12rem] truncate rounded-sm transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      {crumb.label}
                    </Link>
                  )}
                </li>
              </Fragment>
            );
          })}
      </ol>
    </nav>
  );
}
