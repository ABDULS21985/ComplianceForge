'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { ChevronRight, Home } from 'lucide-react';

import { cn } from '@/lib/utils';

const SEGMENT_LABELS: Record<string, string> = {
  dashboard: 'Dashboard',
  frameworks: 'Frameworks',
  risks: 'Risk Register',
  policies: 'Policies',
  audits: 'Audits',
  incidents: 'Incidents',
  vendors: 'Vendors',
  assets: 'Assets',
  reports: 'Reports',
  settings: 'Settings',
  profile: 'Profile',
  new: 'New',
  edit: 'Edit',
  controls: 'Controls',
  findings: 'Findings',
  treatments: 'Treatments',
  versions: 'Versions',
  attestations: 'Attestations',
  'audit-log': 'Audit Log',
  'gap-analysis': 'Gap Analysis',
};

function isUuid(segment: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    segment
  );
}

interface BreadcrumbsProps {
  /** Override the label for a dynamic segment (e.g., an entity name) keyed by segment value */
  dynamicLabels?: Record<string, string>;
  className?: string;
}

export function Breadcrumbs({ dynamicLabels, className }: BreadcrumbsProps) {
  const pathname = usePathname();
  const segments = pathname.split('/').filter(Boolean);

  if (segments.length === 0) return null;

  const crumbs = segments.map((segment, index) => {
    const href = '/' + segments.slice(0, index + 1).join('/');
    const label =
      dynamicLabels?.[segment] ??
      SEGMENT_LABELS[segment] ??
      (isUuid(segment) ? 'Detail' : segment.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));

    return { label, href, isLast: index === segments.length - 1 };
  });

  return (
    <nav aria-label="Breadcrumb" className={cn('flex items-center text-sm text-muted-foreground', className)}>
      <Link
        href="/dashboard"
        className="hover:text-foreground transition-colors"
      >
        <Home className="h-4 w-4" />
      </Link>
      {crumbs.map((crumb) => (
        <React.Fragment key={crumb.href}>
          <ChevronRight className="mx-2 h-3.5 w-3.5 shrink-0" />
          {crumb.isLast ? (
            <span className="font-medium text-foreground truncate max-w-[200px]">
              {crumb.label}
            </span>
          ) : (
            <Link
              href={crumb.href}
              className="hover:text-foreground transition-colors truncate max-w-[200px]"
            >
              {crumb.label}
            </Link>
          )}
        </React.Fragment>
      ))}
    </nav>
  );
}
