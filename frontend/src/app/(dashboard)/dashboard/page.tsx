'use client';

import {
  Activity,
  AlertCircle,
  AlertOctagon,
  AlertTriangle,
  ArrowRight,
  Building2,
  ClipboardCheck,
  FileText,
  Plus,
  Search,
  Shield,
} from 'lucide-react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type {
  ComplianceScore,
  DashboardSummary,
  Incident,
} from '@/types';
import { formatPercentage, formatRelativeTime } from '@/lib/utils';
import { QUICK_CREATE_ROUTES, ROUTES } from '@/lib/routes';
import { ResourceBoundary, StaleDataNotice } from '@/components/data/resource-state';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

function useDashboard() {
  return useQuery<DashboardSummary>({
    queryKey: ['dashboard'],
    queryFn: () => api.dashboard.summary() as Promise<DashboardSummary>,
    staleTime: 60_000,
  });
}

function useUrgentBreaches() {
  return useQuery<Incident[]>({
    queryKey: ['incidents', 'urgent-breaches'],
    queryFn: () => api.incidents.urgentBreaches() as Promise<Incident[]>,
    refetchInterval: 60000,
    staleTime: 30_000,
  });
}

function useComplianceScores() {
  return useQuery<ComplianceScore[]>({
    queryKey: ['compliance', 'scores'],
    queryFn: () => api.compliance.scores() as Promise<ComplianceScore[]>,
    staleTime: 60_000,
  });
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

const RISK_LEVEL_COLORS: Record<string, string> = {
  critical: '#DC2626',
  high: '#EA580C',
  medium: '#EAB308',
  low: '#22C55E',
  very_low: '#06B6D4',
};

function StatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  className,
}: {
  title: string;
  value: string | number;
  subtitle?: string;
  icon: React.ElementType;
  trend?: 'up' | 'down' | 'neutral';
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardContent className="p-6">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <Icon className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="mt-2">
          <p className="text-3xl font-bold">{value}</p>
          {subtitle && (
            <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Main Dashboard Page
// ---------------------------------------------------------------------------

export default function DashboardPage() {
  const dashboard = useDashboard();
  const urgentBreaches = useUrgentBreaches();
  const complianceScores = useComplianceScores();

  const data = dashboard.data;
  const breaches = urgentBreaches.data;
  const scores = complianceScores.data;

  // Build chart data
  const complianceChartData = scores?.map((s) => ({
    name: s.framework_code,
    score: s.compliance_score,
    framework: s.framework_name,
  })) ?? [];

  const riskChartData = data
    ? (Object.entries(data.risk_summary) as [string, number][]).map(
        ([level, count]) => ({
          name: level.replace('_', ' ').replace(/^\w/, (c) => c.toUpperCase()),
          value: count,
          color: RISK_LEVEL_COLORS[level] ?? '#94A3B8',
        })
      )
    : [];

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground">
          Executive overview of your organisation&apos;s compliance posture.
        </p>
      </div>

      {(dashboard.isError && data) ||
      (complianceScores.isError && scores) ||
      (urgentBreaches.isError && breaches) ? (
        <StaleDataNotice
          isRefreshing={
            dashboard.isFetching ||
            complianceScores.isFetching ||
            urgentBreaches.isFetching
          }
          lastUpdatedAt={Math.min(
            ...[
              dashboard.dataUpdatedAt,
              complianceScores.dataUpdatedAt,
              urgentBreaches.dataUpdatedAt,
            ].filter((value) => value > 0),
          )}
          onRefresh={() => {
            void Promise.all([
              dashboard.refetch(),
              complianceScores.refetch(),
              urgentBreaches.refetch(),
            ]);
          }}
        />
      ) : null}

      {urgentBreaches.isError && !breaches && (
        <ResourceBoundary
          isError
          errorTitle="Urgent breach alerts could not be checked"
          errorDescription="Dashboard metrics remain available, but confirm incident deadlines in the incident register."
          onRetry={() => void urgentBreaches.refetch()}
        >
          {null}
        </ResourceBoundary>
      )}

      {/* Section A: GDPR Breach Alert Banner */}
      {breaches && breaches.length > 0 && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 dark:border-red-800 dark:bg-red-950/40">
          <div className="flex items-start gap-3">
            <AlertCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-red-600 dark:text-red-400" />
            <div className="flex-1">
              <h3 className="font-semibold text-red-800 dark:text-red-300">
                GDPR Breach Alert &mdash; {breaches.length} urgent{' '}
                {breaches.length === 1 ? 'breach' : 'breaches'} requiring
                notification
              </h3>
              <p className="mt-1 text-sm text-red-700 dark:text-red-400">
                Data breaches must be reported to the supervisory authority
                within 72 hours of becoming aware. Act immediately.
              </p>
              <div className="mt-3 flex flex-wrap gap-2">
                {breaches.slice(0, 3).map((b) => (
                  <Link key={b.id} href={`/incidents/${b.id}`}>
                    <Badge
                      variant="destructive"
                      className="cursor-pointer hover:opacity-80"
                    >
                      {b.incident_ref}: {b.title}
                    </Badge>
                  </Link>
                ))}
                {breaches.length > 3 && (
                  <Link href="/incidents?is_data_breach=true">
                    <Badge variant="outline" className="cursor-pointer">
                      +{breaches.length - 3} more
                    </Badge>
                  </Link>
                )}
              </div>
            </div>
            <Link href="/incidents?is_data_breach=true">
              <Button variant="destructive" size="sm">
                View All
              </Button>
            </Link>
          </div>
        </div>
      )}

      {/* Section B: KPI StatCards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <ResourceBoundary
          className="col-span-full"
          isError={dashboard.isError && !data}
          isLoading={dashboard.isLoading}
          loadingLayout="cards"
          loadingTitle="Loading executive metrics"
          errorTitle="Dashboard metrics could not be loaded"
          errorDescription="The service did not return executive metrics. Retry without leaving the dashboard."
          onRetry={() => void dashboard.refetch()}
          retrying={dashboard.isFetching}
        >
          {data ? (
          <>
            <StatCard
              title="Compliance Score"
              value={formatPercentage(data.compliance_score)}
              subtitle="Organisation average"
              icon={Shield}
            />
            <StatCard
              title="Open Risks"
              value={data.total_open_risks}
              subtitle={`${data.critical_risks} critical`}
              icon={AlertTriangle}
            />
            <StatCard
              title="Open Incidents"
              value={data.open_incidents}
              subtitle={`${data.critical_incidents} critical`}
              icon={AlertOctagon}
            />
            <StatCard
              title="Audit Findings"
              value={data.open_audit_findings}
              subtitle={`${data.overdue_findings} overdue`}
              icon={ClipboardCheck}
            />
            <StatCard
              title="Policies Due"
              value={data.policies_due_for_review}
              subtitle="Require review"
              icon={FileText}
            />
            <StatCard
              title="High-Risk Vendors"
              value={data.high_risk_vendors}
              subtitle="Require attention"
              icon={Building2}
            />
          </>
          ) : null}
        </ResourceBoundary>
      </div>

      {/* Section C + D: Charts row */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Section C: Framework Compliance bar chart */}
        <div className="lg:col-span-2">
          <ResourceBoundary
            isEmpty={!complianceScores.isLoading && !complianceScores.isError && complianceChartData.length === 0}
            isError={complianceScores.isError && !scores}
            isLoading={complianceScores.isLoading}
            loadingLayout="detail"
            loadingTitle="Loading framework compliance"
            emptyTitle="No framework compliance data"
            emptyDescription="Adopt and assess a framework to populate compliance scores."
            errorTitle="Framework compliance could not be loaded"
            onRetry={() => void complianceScores.refetch()}
            retrying={complianceScores.isFetching}
          >
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">
                  Framework Compliance
                </CardTitle>
              </CardHeader>
              <CardContent>
                {complianceChartData.length > 0 && (
                  <ResponsiveContainer width="100%" height={320}>
                    <BarChart
                      layout="vertical"
                      data={complianceChartData}
                      margin={{ top: 0, right: 20, left: 0, bottom: 0 }}
                    >
                      <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
                      <XAxis
                        type="number"
                        domain={[0, 100]}
                        tickFormatter={(v: number) => `${v}%`}
                      />
                      <YAxis
                        dataKey="name"
                        type="category"
                        width={100}
                        tick={{ fontSize: 12 }}
                      />
                      <Tooltip
                        formatter={(value: number) => [
                          `${value.toFixed(1)}%`,
                          'Compliance',
                        ]}
                      />
                      <Bar dataKey="score" radius={[0, 4, 4, 0]}>
                        {complianceChartData.map((entry, idx) => (
                          <Cell
                            key={idx}
                            fill={
                              entry.score >= 80
                                ? '#22C55E'
                                : entry.score >= 50
                                  ? '#EAB308'
                                  : '#DC2626'
                            }
                          />
                        ))}
                      </Bar>
                    </BarChart>
                  </ResponsiveContainer>
                )}
              </CardContent>
            </Card>
          </ResourceBoundary>
        </div>

        {/* Section D: Risk Distribution donut chart */}
        <div>
          <ResourceBoundary
            isEmpty={Boolean(data) && (riskChartData.length === 0 || riskChartData.every((entry) => entry.value === 0))}
            isError={dashboard.isError && !data}
            isLoading={dashboard.isLoading}
            loadingLayout="detail"
            loadingTitle="Loading risk distribution"
            emptyTitle="No risk distribution data"
            emptyDescription="Register a risk to populate the distribution."
            errorTitle="Risk distribution could not be loaded"
            onRetry={() => void dashboard.refetch()}
          >
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Risk Distribution</CardTitle>
              </CardHeader>
              <CardContent>
                {riskChartData.length > 0 &&
                !riskChartData.every((d) => d.value === 0) && (
                  <ResponsiveContainer width="100%" height={280}>
                    <PieChart>
                      <Pie
                        data={riskChartData}
                        cx="50%"
                        cy="50%"
                        innerRadius={60}
                        outerRadius={100}
                        paddingAngle={2}
                        dataKey="value"
                        nameKey="name"
                        label={({ name, value }: { name: string; value: number }) =>
                          value > 0 ? `${name} (${value})` : ''
                        }
                      >
                        {riskChartData.map((entry, idx) => (
                          <Cell key={idx} fill={entry.color} />
                        ))}
                      </Pie>
                      <Tooltip />
                      <Legend />
                    </PieChart>
                  </ResponsiveContainer>
                )}
              </CardContent>
            </Card>
          </ResourceBoundary>
        </div>
      </div>

      {/* Section E + F: Activity + Quick Actions */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Section E: Recent Activity */}
        <div className="lg:col-span-2">
          <ResourceBoundary
            isEmpty={Boolean(data) && !data?.recent_activity?.length}
            isError={dashboard.isError && !data}
            isLoading={dashboard.isLoading}
            loadingLayout="table"
            loadingTitle="Loading recent activity"
            emptyTitle="No recent activity"
            emptyDescription="New governance activity will appear here."
            errorTitle="Recent activity could not be loaded"
            onRetry={() => void dashboard.refetch()}
          >
            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <CardTitle className="text-lg">Recent Activity</CardTitle>
                <Link href="/settings?tab=audit-log">
                  <Button variant="ghost" size="sm">
                    View all <ArrowRight className="ml-1 h-4 w-4" />
                  </Button>
                </Link>
              </CardHeader>
              <CardContent>
                {data?.recent_activity && data.recent_activity.length > 0 && (
                  <div className="space-y-4">
                    {data.recent_activity.slice(0, 10).map((entry) => (
                      <div
                        key={entry.id}
                        className="flex items-start gap-3 border-b pb-3 last:border-0"
                      >
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-muted">
                          <Activity className="h-4 w-4 text-muted-foreground" />
                        </div>
                        <div className="flex-1 space-y-1">
                          <p className="text-sm">
                            <span className="font-medium">
                              {entry.user_name ?? 'System'}
                            </span>{' '}
                            <span className="text-muted-foreground">
                              {entry.action.replace(/_/g, ' ')}
                            </span>{' '}
                            <span className="font-medium">
                              {entry.entity_type.replace(/_/g, ' ')}
                            </span>
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {formatRelativeTime(entry.created_at)}
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </ResourceBoundary>
        </div>

        {/* Section F: Quick Actions */}
        <div>
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Quick Actions</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-3">
                <Link href={QUICK_CREATE_ROUTES.risk}>
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <Plus className="h-4 w-4" />
                    Register Risk
                  </Button>
                </Link>
                <Link href={QUICK_CREATE_ROUTES.incident}>
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <AlertOctagon className="h-4 w-4" />
                    Report Incident
                  </Button>
                </Link>
                <Link href={QUICK_CREATE_ROUTES.policy}>
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <FileText className="h-4 w-4" />
                    Draft Policy
                  </Button>
                </Link>
                <Link href={QUICK_CREATE_ROUTES.audit}>
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <ClipboardCheck className="h-4 w-4" />
                    Plan Audit
                  </Button>
                </Link>
                <Link href={ROUTES.frameworks}>
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <Search className="h-4 w-4" />
                    Search Controls
                  </Button>
                </Link>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
