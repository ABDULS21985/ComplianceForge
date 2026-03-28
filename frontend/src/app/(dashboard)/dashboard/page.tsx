'use client';

import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import {
  AlertTriangle,
  Shield,
  AlertOctagon,
  ClipboardCheck,
  FileText,
  Building2,
  Plus,
  Search,
  ArrowRight,
  AlertCircle,
  Activity,
  TrendingUp,
} from 'lucide-react';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
  PieChart,
  Pie,
  Legend,
} from 'recharts';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { formatRelativeTime, formatPercentage } from '@/lib/utils';
import api from '@/lib/api';
import type {
  DashboardSummary,
  ComplianceScore,
  Incident,
  AuditLogEntry,
  RiskLevel,
} from '@/types';

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

function useDashboard() {
  return useQuery<DashboardSummary>({
    queryKey: ['dashboard'],
    queryFn: () => api.dashboard.summary() as Promise<DashboardSummary>,
  });
}

function useUrgentBreaches() {
  return useQuery<Incident[]>({
    queryKey: ['incidents', 'urgent-breaches'],
    queryFn: () => api.incidents.urgentBreaches() as Promise<Incident[]>,
    refetchInterval: 60000,
  });
}

function useComplianceScores() {
  return useQuery<ComplianceScore[]>({
    queryKey: ['compliance', 'scores'],
    queryFn: () => api.compliance.scores() as Promise<ComplianceScore[]>,
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

function StatCardSkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-3">
          <div className="h-4 w-24 rounded bg-muted" />
          <div className="h-8 w-16 rounded bg-muted" />
          <div className="h-3 w-32 rounded bg-muted" />
        </div>
      </CardContent>
    </Card>
  );
}

function StatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  trend,
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

function ChartSkeleton({ height = 300 }: { height?: number }) {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-5 w-40 rounded bg-muted" />
          <div
            className="w-full rounded bg-muted"
            style={{ height: `${height}px` }}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function ActivitySkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-5 w-32 rounded bg-muted" />
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex items-start gap-3">
              <div className="h-8 w-8 rounded-full bg-muted" />
              <div className="flex-1 space-y-2">
                <div className="h-4 w-3/4 rounded bg-muted" />
                <div className="h-3 w-1/4 rounded bg-muted" />
              </div>
            </div>
          ))}
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
        {dashboard.isLoading ? (
          Array.from({ length: 6 }).map((_, i) => <StatCardSkeleton key={i} />)
        ) : dashboard.error ? (
          <Card className="col-span-full">
            <CardContent className="flex items-center gap-2 p-6 text-destructive">
              <AlertCircle className="h-5 w-5" />
              <span>Failed to load dashboard data. Please try again.</span>
            </CardContent>
          </Card>
        ) : data ? (
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
      </div>

      {/* Section C + D: Charts row */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Section C: Framework Compliance bar chart */}
        <div className="lg:col-span-2">
          {complianceScores.isLoading ? (
            <ChartSkeleton height={320} />
          ) : complianceScores.error ? (
            <Card>
              <CardContent className="flex items-center gap-2 p-6 text-destructive">
                <AlertCircle className="h-5 w-5" />
                <span>Failed to load compliance scores.</span>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">
                  Framework Compliance
                </CardTitle>
              </CardHeader>
              <CardContent>
                {complianceChartData.length === 0 ? (
                  <p className="py-8 text-center text-muted-foreground">
                    No compliance data available yet.
                  </p>
                ) : (
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
          )}
        </div>

        {/* Section D: Risk Distribution donut chart */}
        <div>
          {dashboard.isLoading ? (
            <ChartSkeleton height={320} />
          ) : (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Risk Distribution</CardTitle>
              </CardHeader>
              <CardContent>
                {riskChartData.length === 0 ||
                riskChartData.every((d) => d.value === 0) ? (
                  <p className="py-8 text-center text-muted-foreground">
                    No risk data available yet.
                  </p>
                ) : (
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
          )}
        </div>
      </div>

      {/* Section E + F: Activity + Quick Actions */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Section E: Recent Activity */}
        <div className="lg:col-span-2">
          {dashboard.isLoading ? (
            <ActivitySkeleton />
          ) : (
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
                {!data?.recent_activity ||
                data.recent_activity.length === 0 ? (
                  <p className="py-8 text-center text-muted-foreground">
                    No recent activity.
                  </p>
                ) : (
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
          )}
        </div>

        {/* Section F: Quick Actions */}
        <div>
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Quick Actions</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-3">
                <Link href="/risks/new">
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <Plus className="h-4 w-4" />
                    Register Risk
                  </Button>
                </Link>
                <Link href="/incidents/new">
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <AlertOctagon className="h-4 w-4" />
                    Report Incident
                  </Button>
                </Link>
                <Link href="/policies/new">
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <FileText className="h-4 w-4" />
                    Draft Policy
                  </Button>
                </Link>
                <Link href="/audits/new">
                  <Button
                    variant="outline"
                    className="w-full justify-start gap-2"
                  >
                    <ClipboardCheck className="h-4 w-4" />
                    Plan Audit
                  </Button>
                </Link>
                <Link href="/frameworks">
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
