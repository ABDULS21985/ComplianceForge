'use client';

import { useState, useCallback } from 'react';
import { useParams, useSearchParams, useRouter } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import {
  Shield,
  AlertCircle,
  CheckCircle2,
  XCircle,
  MinusCircle,
  ChevronLeft,
  Search,
  ArrowUpDown,
} from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn, getStatusColor, formatPercentage } from '@/lib/utils';
import api from '@/lib/api';
import type { PaginatedResponse } from '@/lib/api';
import type {
  ComplianceFramework,
  FrameworkControl,
  ControlImplementation,
  GapAnalysisEntry,
  CrossFrameworkMapping,
  ComplianceScore,
} from '@/types';
import Link from 'next/link';

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

function useFramework(id: string) {
  return useQuery<ComplianceFramework>({
    queryKey: ['frameworks', id],
    queryFn: () => api.frameworks.get(id) as Promise<ComplianceFramework>,
    enabled: !!id,
  });
}

function useFrameworkControls(
  id: string,
  page: number,
  pageSize: number,
  search?: string
) {
  return useQuery<PaginatedResponse<FrameworkControl>>({
    queryKey: ['frameworks', id, 'controls', page, pageSize, search],
    queryFn: () => {
      if (search) {
        return api.frameworks.searchControls(id, search, {
          page,
          page_size: pageSize,
        }) as Promise<PaginatedResponse<FrameworkControl>>;
      }
      return api.frameworks.getControls(id, {
        page,
        page_size: pageSize,
      }) as Promise<PaginatedResponse<FrameworkControl>>;
    },
    enabled: !!id,
  });
}

function useFrameworkImplementations(id: string, page: number, pageSize: number) {
  return useQuery<PaginatedResponse<ControlImplementation>>({
    queryKey: ['frameworks', id, 'implementations', page, pageSize],
    queryFn: () =>
      api.frameworks.getImplementations(id, {
        page,
        page_size: pageSize,
      }) as Promise<PaginatedResponse<ControlImplementation>>,
    enabled: !!id,
  });
}

function useGapAnalysis(frameworkId: string) {
  return useQuery<GapAnalysisEntry[]>({
    queryKey: ['compliance', 'gaps', frameworkId],
    queryFn: () =>
      api.compliance.gaps({
        framework_id: frameworkId,
      }) as Promise<GapAnalysisEntry[]>,
    enabled: !!frameworkId,
  });
}

function useCrossMapping(frameworkId: string) {
  return useQuery<CrossFrameworkMapping[]>({
    queryKey: ['compliance', 'cross-mapping', frameworkId],
    queryFn: () =>
      api.compliance.crossMapping(frameworkId, '') as Promise<
        CrossFrameworkMapping[]
      >,
    enabled: !!frameworkId,
  });
}

function useComplianceScoreForFramework(frameworkId: string) {
  return useQuery<ComplianceScore[]>({
    queryKey: ['compliance', 'scores', frameworkId],
    queryFn: () =>
      api.compliance.scores({
        framework_id: frameworkId,
      }) as Promise<ComplianceScore[]>,
    enabled: !!frameworkId,
  });
}

// ---------------------------------------------------------------------------
// Tab constants
// ---------------------------------------------------------------------------

const TABS = [
  { key: 'controls', label: 'Controls' },
  { key: 'implementation', label: 'Implementation Status' },
  { key: 'gaps', label: 'Gap Analysis' },
  { key: 'mapping', label: 'Cross-Mapping' },
] as const;

type TabKey = (typeof TABS)[number]['key'];

// ---------------------------------------------------------------------------
// Skeleton
// ---------------------------------------------------------------------------

function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="animate-pulse space-y-3">
      <div className="flex gap-4 border-b pb-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="h-4 flex-1 rounded bg-muted" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex gap-4 py-2">
          {Array.from({ length: 5 }).map((_, j) => (
            <div key={j} className="h-4 flex-1 rounded bg-muted" />
          ))}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tab 1: Controls
// ---------------------------------------------------------------------------

function ControlsTab({ frameworkId }: { frameworkId: string }) {
  const searchParams = useSearchParams();
  const router = useRouter();
  const page = parseInt(searchParams.get('page') ?? '1', 10);
  const pageSize = 20;
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');

  const { data, isLoading, error } = useFrameworkControls(
    frameworkId,
    page,
    pageSize,
    search
  );

  const controls = data?.items ?? [];
  const totalPages = data?.total_pages ?? 1;

  const handleSearch = useCallback(() => {
    setSearch(searchInput);
    router.push(`?tab=controls&page=1`, { scroll: false });
  }, [searchInput, router]);

  const goToPage = useCallback(
    (p: number) => {
      router.push(`?tab=controls&page=${p}`, { scroll: false });
    },
    [router]
  );

  return (
    <div className="space-y-4">
      {/* Search bar */}
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search controls..."
            className="pl-9"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
          />
        </div>
        <Button onClick={handleSearch} variant="secondary">
          Search
        </Button>
      </div>

      {/* Error */}
      {error && (
        <div className="flex items-center gap-2 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>Failed to load controls.</span>
        </div>
      )}

      {/* Loading */}
      {isLoading && <TableSkeleton />}

      {/* Table */}
      {!isLoading && !error && (
        <>
          {controls.length === 0 ? (
            <p className="py-8 text-center text-muted-foreground">
              No controls found.
            </p>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Code</th>
                    <th className="px-4 py-3 text-left font-medium">Title</th>
                    <th className="px-4 py-3 text-left font-medium">Type</th>
                    <th className="px-4 py-3 text-left font-medium">
                      Implementation
                    </th>
                    <th className="px-4 py-3 text-left font-medium">Priority</th>
                  </tr>
                </thead>
                <tbody>
                  {controls.map((ctrl) => (
                    <tr
                      key={ctrl.id}
                      className="border-b transition-colors hover:bg-muted/30 cursor-pointer"
                      onClick={() =>
                        router.push(`/controls/${ctrl.id}`)
                      }
                    >
                      <td className="px-4 py-3 font-mono text-xs font-medium">
                        {ctrl.code}
                      </td>
                      <td className="px-4 py-3 max-w-sm truncate">
                        {ctrl.title}
                      </td>
                      <td className="px-4 py-3">
                        {ctrl.control_type && (
                          <Badge variant="outline" className="text-xs">
                            {ctrl.control_type}
                          </Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        {ctrl.implementation_type && (
                          <Badge variant="secondary" className="text-xs">
                            {ctrl.implementation_type}
                          </Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        {ctrl.priority && (
                          <Badge
                            className={cn(
                              'text-xs',
                              ctrl.priority === 'high' || ctrl.priority === 'critical'
                                ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
                                : ctrl.priority === 'medium'
                                  ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
                                  : 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                            )}
                          >
                            {ctrl.priority}
                          </Badge>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                Page {page} of {totalPages} ({data?.total ?? 0} controls)
              </p>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => goToPage(page - 1)}
                >
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => goToPage(page + 1)}
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tab 2: Implementation Status
// ---------------------------------------------------------------------------

function ImplementationTab({ frameworkId }: { frameworkId: string }) {
  const searchParams = useSearchParams();
  const router = useRouter();
  const page = parseInt(searchParams.get('page') ?? '1', 10);
  const pageSize = 20;

  const { data, isLoading, error } = useFrameworkImplementations(
    frameworkId,
    page,
    pageSize
  );
  const scores = useComplianceScoreForFramework(frameworkId);

  const implementations = data?.items ?? [];
  const totalPages = data?.total_pages ?? 1;
  const score = scores.data?.[0];

  const goToPage = useCallback(
    (p: number) => {
      router.push(`?tab=implementation&page=${p}`, { scroll: false });
    },
    [router]
  );

  return (
    <div className="space-y-6">
      {/* Summary cards */}
      {scores.isLoading ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <div className="animate-pulse space-y-2">
                  <div className="h-4 w-20 rounded bg-muted" />
                  <div className="h-8 w-12 rounded bg-muted" />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : score ? (
        <>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
            <Card>
              <CardContent className="p-4">
                <p className="text-sm text-muted-foreground">Implemented</p>
                <p className="text-2xl font-bold text-green-600">
                  {score.implemented_count + score.effective_count}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-sm text-muted-foreground">Partial</p>
                <p className="text-2xl font-bold text-yellow-600">
                  {score.partial_count}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-sm text-muted-foreground">
                  Not Implemented
                </p>
                <p className="text-2xl font-bold text-red-600">
                  {score.not_implemented_count}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-sm text-muted-foreground">
                  Not Applicable
                </p>
                <p className="text-2xl font-bold text-gray-500">
                  {score.not_applicable_count}
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Progress bar */}
          <div className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="font-medium">Overall Compliance</span>
              <span className="font-semibold">
                {formatPercentage(score.compliance_score)}
              </span>
            </div>
            <div className="h-3 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-green-500 transition-all"
                style={{ width: `${score.compliance_score}%` }}
              />
            </div>
          </div>
        </>
      ) : null}

      {/* Error */}
      {error && (
        <div className="flex items-center gap-2 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>Failed to load implementations.</span>
        </div>
      )}

      {/* Loading */}
      {isLoading && <TableSkeleton />}

      {/* Table */}
      {!isLoading && !error && (
        <>
          {implementations.length === 0 ? (
            <p className="py-8 text-center text-muted-foreground">
              No implementation records found.
            </p>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">Control</th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">
                      Maturity
                    </th>
                    <th className="px-4 py-3 text-left font-medium">
                      Impl. Status
                    </th>
                    <th className="px-4 py-3 text-left font-medium">Owner</th>
                  </tr>
                </thead>
                <tbody>
                  {implementations.map((impl) => (
                    <tr
                      key={impl.id}
                      className="border-b transition-colors hover:bg-muted/30 cursor-pointer"
                      onClick={() => router.push(`/controls/${impl.id}`)}
                    >
                      <td className="px-4 py-3 font-mono text-xs">
                        {impl.control?.code ?? impl.framework_control_id}
                      </td>
                      <td className="px-4 py-3">
                        <Badge
                          className={cn(
                            'text-xs',
                            getStatusColor(impl.status)
                          )}
                        >
                          {impl.status.replace(/_/g, ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">{impl.maturity_level}/5</td>
                      <td className="px-4 py-3">
                        <Badge
                          className={cn(
                            'text-xs',
                            getStatusColor(impl.implementation_status)
                          )}
                        >
                          {impl.implementation_status.replace(/_/g, ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">
                        {impl.owner_user_id ?? '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                Page {page} of {totalPages}
              </p>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => goToPage(page - 1)}
                >
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => goToPage(page + 1)}
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tab 3: Gap Analysis
// ---------------------------------------------------------------------------

function GapAnalysisTab({ frameworkId }: { frameworkId: string }) {
  const { data, isLoading, error } = useGapAnalysis(frameworkId);
  const router = useRouter();

  const gaps = (data ?? []).sort((a, b) => {
    const priorityOrder: Record<string, number> = {
      critical: 0,
      high: 1,
      medium: 2,
      low: 3,
    };
    const pa = priorityOrder[a.control_priority ?? 'low'] ?? 99;
    const pb = priorityOrder[b.control_priority ?? 'low'] ?? 99;
    return pa - pb;
  });

  return (
    <div className="space-y-4">
      {error && (
        <div className="flex items-center gap-2 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>Failed to load gap analysis.</span>
        </div>
      )}

      {isLoading && <TableSkeleton rows={8} />}

      {!isLoading && !error && (
        <>
          {gaps.length === 0 ? (
            <div className="flex flex-col items-center py-12 text-center">
              <CheckCircle2 className="h-12 w-12 text-green-500" />
              <p className="mt-4 text-lg font-semibold">No gaps identified</p>
              <p className="text-sm text-muted-foreground">
                All controls appear to be adequately addressed.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">
                      Control
                    </th>
                    <th className="px-4 py-3 text-left font-medium">Title</th>
                    <th className="px-4 py-3 text-left font-medium">
                      Priority
                    </th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">
                      Maturity
                    </th>
                    <th className="px-4 py-3 text-left font-medium">
                      Overdue
                    </th>
                    <th className="px-4 py-3 text-left font-medium">Gap</th>
                  </tr>
                </thead>
                <tbody>
                  {gaps.map((gap) => (
                    <tr
                      key={gap.control_implementation_id}
                      className="border-b transition-colors hover:bg-muted/30 cursor-pointer"
                      onClick={() =>
                        router.push(
                          `/controls/${gap.control_implementation_id}`
                        )
                      }
                    >
                      <td className="px-4 py-3 font-mono text-xs font-medium">
                        {gap.control_code}
                      </td>
                      <td className="px-4 py-3 max-w-xs truncate">
                        {gap.control_title}
                      </td>
                      <td className="px-4 py-3">
                        {gap.control_priority && (
                          <Badge
                            className={cn(
                              'text-xs',
                              gap.control_priority === 'critical' ||
                                gap.control_priority === 'high'
                                ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
                                : gap.control_priority === 'medium'
                                  ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
                                  : 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                            )}
                          >
                            {gap.control_priority}
                          </Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <Badge
                          className={cn(
                            'text-xs',
                            getStatusColor(gap.status)
                          )}
                        >
                          {gap.status.replace(/_/g, ' ')}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">{gap.maturity_level}/5</td>
                      <td className="px-4 py-3">
                        {gap.is_overdue ? (
                          <Badge variant="destructive" className="text-xs">
                            Overdue
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3 max-w-xs truncate text-muted-foreground">
                        {gap.gap_description ?? '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tab 4: Cross-Mapping
// ---------------------------------------------------------------------------

function CrossMappingTab({ frameworkId }: { frameworkId: string }) {
  const { data, isLoading, error } = useCrossMapping(frameworkId);

  const mappings = (data ?? []).filter(
    (m) =>
      m.source_framework_code === frameworkId ||
      m.target_framework_code === frameworkId
  );

  function StrengthBar({ value }: { value: number }) {
    return (
      <div className="flex items-center gap-2">
        <div className="h-2 w-20 overflow-hidden rounded-full bg-muted">
          <div
            className={cn(
              'h-full rounded-full transition-all',
              value >= 0.8
                ? 'bg-green-500'
                : value >= 0.5
                  ? 'bg-yellow-500'
                  : 'bg-red-500'
            )}
            style={{ width: `${(value * 100).toFixed(0)}%` }}
          />
        </div>
        <span className="text-xs text-muted-foreground">
          {(value * 100).toFixed(0)}%
        </span>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {error && (
        <div className="flex items-center gap-2 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>Failed to load cross-mappings.</span>
        </div>
      )}

      {isLoading && <TableSkeleton rows={6} />}

      {!isLoading && !error && (
        <>
          {mappings.length === 0 ? (
            <p className="py-8 text-center text-muted-foreground">
              No cross-framework mappings available.
            </p>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left font-medium">
                      Source
                    </th>
                    <th className="px-4 py-3 text-left font-medium" />
                    <th className="px-4 py-3 text-left font-medium">
                      Target
                    </th>
                    <th className="px-4 py-3 text-left font-medium">Type</th>
                    <th className="px-4 py-3 text-left font-medium">
                      Strength
                    </th>
                    <th className="px-4 py-3 text-left font-medium">
                      Verified
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {mappings.map((m, idx) => (
                    <tr
                      key={idx}
                      className="border-b transition-colors hover:bg-muted/30"
                    >
                      <td className="px-4 py-3">
                        <div>
                          <span className="font-mono text-xs font-medium">
                            {m.source_control_code}
                          </span>
                          <p className="text-xs text-muted-foreground truncate max-w-xs">
                            {m.source_control_title}
                          </p>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">
                        &rarr;
                      </td>
                      <td className="px-4 py-3">
                        <div>
                          <span className="font-mono text-xs font-medium">
                            {m.target_control_code}
                          </span>
                          <p className="text-xs text-muted-foreground truncate max-w-xs">
                            {m.target_control_title}
                          </p>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant="outline" className="text-xs">
                          {m.mapping_type}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <StrengthBar value={m.mapping_strength} />
                      </td>
                      <td className="px-4 py-3">
                        {m.is_verified ? (
                          <CheckCircle2 className="h-4 w-4 text-green-500" />
                        ) : (
                          <MinusCircle className="h-4 w-4 text-muted-foreground" />
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function FrameworkDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const router = useRouter();
  const id = params.id as string;

  const activeTab = (searchParams.get('tab') ?? 'controls') as TabKey;

  const { data: framework, isLoading, error } = useFramework(id);

  const setTab = useCallback(
    (tab: TabKey) => {
      router.push(`?tab=${tab}&page=1`, { scroll: false });
    },
    [router]
  );

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="animate-pulse space-y-4">
          <div className="h-4 w-24 rounded bg-muted" />
          <div className="h-8 w-64 rounded bg-muted" />
          <div className="h-5 w-96 rounded bg-muted" />
          <div className="flex gap-2">
            <div className="h-6 w-20 rounded-full bg-muted" />
            <div className="h-6 w-24 rounded-full bg-muted" />
          </div>
        </div>
        <div className="h-px w-full bg-border" />
        <TableSkeleton rows={8} />
      </div>
    );
  }

  if (error || !framework) {
    return (
      <div className="space-y-4">
        <Link href="/frameworks">
          <Button variant="ghost" size="sm">
            <ChevronLeft className="mr-1 h-4 w-4" /> Back to Frameworks
          </Button>
        </Link>
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>
              {error
                ? 'Failed to load framework details.'
                : 'Framework not found.'}
            </span>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link href="/frameworks">
        <Button variant="ghost" size="sm">
          <ChevronLeft className="mr-1 h-4 w-4" /> Back to Frameworks
        </Button>
      </Link>

      {/* Header */}
      <div className="space-y-2">
        <div className="flex items-center gap-3">
          <h1 className="text-3xl font-bold tracking-tight">
            {framework.name}
          </h1>
          {framework.color_hex && (
            <div
              className="h-4 w-4 rounded-full"
              style={{ backgroundColor: framework.color_hex }}
            />
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <span>Version {framework.version}</span>
          {framework.issuing_body && (
            <>
              <span>&middot;</span>
              <span>{framework.issuing_body}</span>
            </>
          )}
          {framework.category && (
            <>
              <span>&middot;</span>
              <Badge variant="secondary">{framework.category}</Badge>
            </>
          )}
        </div>
        {framework.description && (
          <p className="text-sm text-muted-foreground max-w-3xl">
            {framework.description}
          </p>
        )}
      </div>

      {/* Tabs */}
      <div className="border-b">
        <nav className="flex gap-4 -mb-px">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setTab(tab.key)}
              className={cn(
                'px-1 pb-3 text-sm font-medium transition-colors border-b-2',
                activeTab === tab.key
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground hover:border-muted-foreground'
              )}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      <div>
        {activeTab === 'controls' && <ControlsTab frameworkId={id} />}
        {activeTab === 'implementation' && (
          <ImplementationTab frameworkId={id} />
        )}
        {activeTab === 'gaps' && <GapAnalysisTab frameworkId={id} />}
        {activeTab === 'mapping' && <CrossMappingTab frameworkId={id} />}
      </div>
    </div>
  );
}
