'use client';

import {
  ArrowRight,
  CheckCircle2,
  FileCheck2,
  FileSearch,
  ScanSearch,
  ShieldCheck,
} from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn, getStatusColor } from '@/lib/utils';
import { evidenceStatusLabel, formatEvidenceError } from '@/lib/control-evidence';
import { ResourceBoundary, ResourceState, StaleDataNotice } from '@/components/data/resource-state';
import { useDeferredValue, useState } from 'react';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import Link from 'next/link';
import { useControlPermissions } from '@/hooks/use-control-permissions';
import { useQuery } from '@tanstack/react-query';

const PAGE_SIZE = 12;

export default function EvidenceOverviewPage() {
  const [filter, setFilter] = useState('');
  const [page, setPage] = useState(1);
  const deferredFilter = useDeferredValue(filter.trim().toLocaleLowerCase());
  const permission = useControlPermissions();
  const query = useQuery({
    queryKey: ['controls', 'evidence-overview', { page, page_size: PAGE_SIZE }],
    queryFn: () => api.controls.list({ page, page_size: PAGE_SIZE }),
    enabled: permission.canRead,
  });

  if (permission.isLoading) {
    return (
      <ResourceState
        headingLevel={1}
        kind="loading"
        loadingLayout="detail"
        title="Checking evidence access"
      />
    );
  }
  if (permission.isError) {
    return (
      <ResourceState
        headingLevel={1}
        kind="error"
        title="Evidence access could not be verified"
        description="The permission service is unavailable. No control or evidence data was loaded."
        onRetry={() => void permission.retry()}
      />
    );
  }
  if (!permission.canRead) {
    return (
      <ResourceState
        headingLevel={1}
        kind="forbidden"
        title="Evidence workspace unavailable"
        description="Your role does not grant controls:read access."
      />
    );
  }

  const controls = query.data?.data ?? [];
  const visibleControls = controls.filter((control) =>
    `${control.code} ${control.title} ${control.category ?? ''}`
      .toLocaleLowerCase()
      .includes(deferredFilter),
  );
  const adopted = controls.filter((control) => control.implementation).length;
  const effective = controls.filter((control) =>
    ['implemented', 'effective'].includes(control.implementation?.status ?? ''),
  ).length;

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Evidence workspace</h1>
          <p className="mt-1 max-w-3xl text-muted-foreground">
            Select an adopted control to upload, review, and securely export its evidence. Results
            and counts are scoped to your tenant and current page.
          </p>
        </div>
        <Button asChild variant="outline">
          <Link href="/frameworks">
            Browse frameworks
            <ArrowRight aria-hidden="true" className="ml-2 h-4 w-4" />
          </Link>
        </Button>
      </header>

      {query.isError && query.data && (
        <StaleDataNotice
          isRefreshing={query.isFetching}
          lastUpdatedAt={query.dataUpdatedAt}
          onRefresh={() => void query.refetch()}
        />
      )}

      <section aria-labelledby="evidence-workflow-title" className="grid gap-4 md:grid-cols-3">
        <h2 id="evidence-workflow-title" className="sr-only">
          Secure evidence workflow
        </h2>
        <WorkflowCard
          icon={FileCheck2}
          step="1"
          title="One trusted file"
          description="Add a supported file and bounded metadata to an adopted control."
        />
        <WorkflowCard
          icon={ScanSearch}
          step="2"
          title="Scan before storage"
          description="Type, structure, checksum, and malware checks must pass before registration."
        />
        <WorkflowCard
          icon={ShieldCheck}
          step="3"
          title="Review and renew"
          description="Authorized reviewers accept or reject evidence and monitor validity dates."
        />
      </section>

      {query.isSuccess && (
        <section aria-label="Current controls page summary" className="grid gap-4 sm:grid-cols-3">
          <SummaryCard label="Controls on this page" value={controls.length} />
          <SummaryCard label="Adopted on this page" value={adopted} />
          <SummaryCard label="Implemented or effective" value={effective} />
        </section>
      )}

      <Card>
        <CardHeader className="gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <CardTitle>Controls</CardTitle>
            <CardDescription>
              Evidence records are listed within each control because the mounted API does not
              expose a cross-control evidence collection.
            </CardDescription>
          </div>
          <div className="w-full lg:max-w-sm">
            <label
              htmlFor="control-page-filter"
              className="mb-1 block text-xs font-medium text-muted-foreground"
            >
              Filter controls on this page
            </label>
            <div className="relative">
              <FileSearch
                aria-hidden="true"
                className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground"
              />
              <Input
                id="control-page-filter"
                type="search"
                className="pl-9"
                value={filter}
                placeholder="Code, title, or category"
                onChange={(event) => setFilter(event.target.value)}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          {!permission.canUpdate && (
            <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
              You can inspect evidence. Uploading requires controls:update, downloading requires
              controls:export, and review decisions require controls:approve.
            </p>
          )}
          <ResourceBoundary
            surface="plain"
            isEmpty={query.isSuccess && controls.length === 0}
            isError={query.isError && !query.data}
            isLoading={query.isLoading}
            loadingLayout="cards"
            loadingTitle="Loading evidence controls"
            emptyTitle="No controls available"
            emptyDescription="Adopt a framework to create tenant implementation records and begin collecting evidence."
            emptyAction={
              <Button asChild variant="outline">
                <Link href="/frameworks">Browse frameworks</Link>
              </Button>
            }
            errorTitle="Evidence controls could not be loaded"
            errorDescription={formatEvidenceError(query.error, 'list')}
            onRetry={() => void query.refetch()}
            retrying={query.isFetching}
          >
            <>
            {query.isSuccess && controls.length > 0 && visibleControls.length === 0 && (
              <ResourceState
                surface="plain"
                kind="empty"
                title="No controls match this page filter"
                description={`No control on this page matches “${filter}”. Clear the filter or move to another page.`}
                action={
                  <Button type="button" variant="outline" onClick={() => setFilter('')}>
                    Clear filter
                  </Button>
                }
              />
            )}
            {visibleControls.length > 0 && (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {visibleControls.map((control) => (
                <Link
                  key={control.id}
                  href={`/controls/${encodeURIComponent(control.id)}`}
                  className="group rounded-lg border p-4 transition-colors hover:border-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  aria-label={`Open evidence for ${control.code}: ${control.title}`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <Badge variant="outline" className="font-mono">
                      {control.code}
                    </Badge>
                    {control.implementation ? (
                      <Badge className={cn(getStatusColor(control.implementation.status))}>
                        <CheckCircle2 aria-hidden="true" className="mr-1 h-3.5 w-3.5" />
                        {evidenceStatusLabel(control.implementation.status)}
                      </Badge>
                    ) : (
                      <Badge variant="secondary">Not adopted</Badge>
                    )}
                  </div>
                  <h3 className="mt-4 font-semibold group-hover:text-primary">{control.title}</h3>
                  <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">
                    {control.description ||
                      'No description is available to your field-access policy.'}
                  </p>
                  <div className="mt-4 flex items-center justify-between gap-2 border-t pt-3 text-xs text-muted-foreground">
                    <span>{control.category || 'Uncategorised'}</span>
                    <span className="inline-flex items-center font-medium text-foreground">
                      Open evidence <ArrowRight aria-hidden="true" className="ml-1 h-3.5 w-3.5" />
                    </span>
                  </div>
                </Link>
              ))}
            </div>
            )}
            {query.isSuccess && query.data.pagination.total_pages > 1 && (
            <nav
              aria-label="Control pages"
              className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between"
            >
              <p className="text-sm text-muted-foreground">
                Page {query.data.pagination.page} of {query.data.pagination.total_pages} ·{' '}
                {query.data.pagination.total_items} controls
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page <= 1 || query.isFetching}
                  onClick={() => {
                    setFilter('');
                    setPage((value) => Math.max(1, value - 1));
                  }}
                >
                  Previous
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page >= query.data.pagination.total_pages || query.isFetching}
                  onClick={() => {
                    setFilter('');
                    setPage((value) => value + 1);
                  }}
                >
                  Next
                </Button>
              </div>
            </nav>
            )}
            </>
          </ResourceBoundary>
        </CardContent>
      </Card>
    </div>
  );
}

function WorkflowCard({
  description,
  icon: Icon,
  step,
  title,
}: {
  description: string;
  icon: typeof FileCheck2;
  step: string;
  title: string;
}) {
  return (
    <Card>
      <CardContent className="flex gap-3 py-5">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
          <Icon aria-hidden="true" className="h-5 w-5" />
        </div>
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Step {step}
          </p>
          <h3 className="font-semibold">{title}</h3>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function SummaryCard({ label, value }: { label: string; value: number }) {
  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-sm text-muted-foreground">{label}</p>
        <p className="mt-1 text-2xl font-semibold">{value}</p>
      </CardContent>
    </Card>
  );
}
