'use client';

import { useState } from 'react';
import {
  BarChart3,
  FileText,
  AlertTriangle,
  Shield,
  TrendingUp,
  AlertCircle,
  Download,
  ChevronRight,
  Target,
  PieChart as PieChartIcon,
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

import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { formatPercentage, getRiskLevelColor } from '@/lib/utils';
import { useComplianceReport, useRiskReport } from '@/lib/api-hooks';

// ---------------------------------------------------------------------------
// Types for report data
// ---------------------------------------------------------------------------

interface ComplianceReportData {
  overall_score: number;
  frameworks: Array<{
    framework_code: string;
    framework_name: string;
    compliance_score: number;
    total_controls: number;
    implemented: number;
    gaps: number;
    maturity_level: number;
  }>;
  top_gaps: Array<{
    control_code: string;
    control_title: string;
    framework_code: string;
    status: string;
    priority: string;
  }>;
}

interface RiskReportData {
  risk_distribution: Record<string, number>;
  top_risks: Array<{
    risk_ref: string;
    title: string;
    risk_level: string;
    risk_score: number;
    status: string;
    owner_name?: string;
  }>;
  treatment_progress: {
    total: number;
    completed: number;
    in_progress: number;
    not_started: number;
  };
}

const RISK_LEVEL_COLORS: Record<string, string> = {
  critical: '#DC2626',
  high: '#EA580C',
  medium: '#EAB308',
  low: '#22C55E',
  very_low: '#06B6D4',
};

const MATURITY_LABELS: Record<number, string> = {
  0: 'Non-existent',
  1: 'Initial',
  2: 'Managed',
  3: 'Defined',
  4: 'Quantitatively Managed',
  5: 'Optimizing',
};

// ---------------------------------------------------------------------------
// Compliance Report Section
// ---------------------------------------------------------------------------

function ComplianceReportSection() {
  const [generated, setGenerated] = useState(false);
  const reportQuery = useComplianceReport();

  const handleGenerate = () => {
    setGenerated(true);
    reportQuery.refetch();
  };

  const data = reportQuery.data as ComplianceReportData | undefined;

  return (
    <div className="space-y-6">
      {!generated ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
              <Shield className="h-8 w-8 text-primary" />
            </div>
            <h3 className="mt-4 text-lg font-semibold">Compliance Report</h3>
            <p className="mt-2 max-w-md text-sm text-muted-foreground">
              Generate a comprehensive compliance report covering all adopted frameworks,
              control implementation status, maturity levels, and gap analysis.
            </p>
            <Button className="mt-6" onClick={handleGenerate}>
              <BarChart3 className="mr-2 h-4 w-4" />
              Generate Report
            </Button>
          </CardContent>
        </Card>
      ) : reportQuery.isLoading || reportQuery.isFetching ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
            <p className="mt-4 text-sm text-muted-foreground">Generating compliance report...</p>
          </CardContent>
        </Card>
      ) : reportQuery.error ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 p-6 text-center">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="text-destructive">Failed to generate compliance report.</p>
            <Button variant="outline" onClick={handleGenerate}>
              Retry
            </Button>
          </CardContent>
        </Card>
      ) : data ? (
        <>
          {/* Overall Score */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Overall Compliance Score</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-6">
                <div className="flex h-24 w-24 items-center justify-center rounded-full border-4 border-primary">
                  <span className="text-2xl font-bold">{formatPercentage(data.overall_score)}</span>
                </div>
                <div className="flex-1">
                  <Progress value={data.overall_score} className="h-3" />
                  <p className="mt-2 text-sm text-muted-foreground">
                    Aggregated compliance score across all adopted frameworks.
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Framework Breakdown */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Framework Breakdown</CardTitle>
            </CardHeader>
            <CardContent>
              {data.frameworks?.length > 0 ? (
                <>
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b text-left">
                          <th className="pb-3 pr-4 font-medium text-muted-foreground">Framework</th>
                          <th className="pb-3 pr-4 font-medium text-muted-foreground">Score</th>
                          <th className="pb-3 pr-4 font-medium text-muted-foreground">Total Controls</th>
                          <th className="pb-3 pr-4 font-medium text-muted-foreground">Implemented</th>
                          <th className="pb-3 pr-4 font-medium text-muted-foreground">Gaps</th>
                          <th className="pb-3 font-medium text-muted-foreground">Maturity</th>
                        </tr>
                      </thead>
                      <tbody>
                        {data.frameworks.map((fw) => (
                          <tr key={fw.framework_code} className="border-b last:border-0 hover:bg-muted/50">
                            <td className="py-3 pr-4">
                              <div>
                                <p className="font-medium">{fw.framework_code}</p>
                                <p className="text-xs text-muted-foreground">{fw.framework_name}</p>
                              </div>
                            </td>
                            <td className="py-3 pr-4">
                              <div className="flex items-center gap-2">
                                <Progress value={fw.compliance_score} className="h-2 w-20" />
                                <span className="text-xs font-medium">
                                  {formatPercentage(fw.compliance_score)}
                                </span>
                              </div>
                            </td>
                            <td className="py-3 pr-4">{fw.total_controls}</td>
                            <td className="py-3 pr-4 text-green-600 dark:text-green-400">
                              {fw.implemented}
                            </td>
                            <td className="py-3 pr-4">
                              {fw.gaps > 0 ? (
                                <span className="font-medium text-red-600 dark:text-red-400">
                                  {fw.gaps}
                                </span>
                              ) : (
                                <span className="text-muted-foreground">0</span>
                              )}
                            </td>
                            <td className="py-3">
                              <Badge variant="outline">
                                L{fw.maturity_level} &mdash; {MATURITY_LABELS[fw.maturity_level] ?? 'Unknown'}
                              </Badge>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>

                  {/* Maturity Chart */}
                  <div className="mt-6">
                    <h4 className="mb-4 text-sm font-medium text-muted-foreground">
                      Compliance by Framework
                    </h4>
                    <ResponsiveContainer width="100%" height={300}>
                      <BarChart data={data.frameworks} margin={{ top: 0, right: 20, left: 0, bottom: 0 }}>
                        <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
                        <XAxis dataKey="framework_code" tick={{ fontSize: 12 }} />
                        <YAxis domain={[0, 100]} tickFormatter={(v: number) => `${v}%`} />
                        <Tooltip formatter={(value: number) => [`${value.toFixed(1)}%`, 'Compliance']} />
                        <Bar dataKey="compliance_score" radius={[4, 4, 0, 0]}>
                          {data.frameworks.map((fw, idx) => (
                            <Cell
                              key={idx}
                              fill={
                                fw.compliance_score >= 80
                                  ? '#22C55E'
                                  : fw.compliance_score >= 50
                                    ? '#EAB308'
                                    : '#DC2626'
                              }
                            />
                          ))}
                        </Bar>
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </>
              ) : (
                <p className="py-8 text-center text-muted-foreground">
                  No framework data available.
                </p>
              )}
            </CardContent>
          </Card>

          {/* Top Gaps */}
          {data.top_gaps?.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Top Compliance Gaps</CardTitle>
                <CardDescription>
                  Highest priority controls requiring attention.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {data.top_gaps.map((gap, idx) => (
                    <div
                      key={idx}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <div className="flex items-center gap-3">
                        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-red-100 text-sm font-medium text-red-800 dark:bg-red-900/30 dark:text-red-400">
                          {idx + 1}
                        </div>
                        <div>
                          <p className="text-sm font-medium">
                            {gap.control_code}: {gap.control_title}
                          </p>
                          <p className="text-xs text-muted-foreground">{gap.framework_code}</p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        {gap.priority && (
                          <Badge variant="outline" className="text-xs capitalize">
                            {gap.priority}
                          </Badge>
                        )}
                        <Badge className={cn('capitalize', getRiskLevelColor(gap.status === 'not_implemented' ? 'critical' : 'medium'))}>
                          {gap.status.replace(/_/g, ' ')}
                        </Badge>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}

          {/* Regenerate */}
          <div className="flex justify-center">
            <Button variant="outline" onClick={handleGenerate}>
              Regenerate Report
            </Button>
          </div>
        </>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Risk Report Section
// ---------------------------------------------------------------------------

function RiskReportSection() {
  const [generated, setGenerated] = useState(false);
  const reportQuery = useRiskReport();

  const handleGenerate = () => {
    setGenerated(true);
    reportQuery.refetch();
  };

  const data = reportQuery.data as RiskReportData | undefined;

  return (
    <div className="space-y-6">
      {!generated ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-orange-100 dark:bg-orange-900/30">
              <AlertTriangle className="h-8 w-8 text-orange-600 dark:text-orange-400" />
            </div>
            <h3 className="mt-4 text-lg font-semibold">Risk Report</h3>
            <p className="mt-2 max-w-md text-sm text-muted-foreground">
              Generate a risk report with distribution analysis, top risks, treatment progress,
              and risk trend data for executive review.
            </p>
            <Button className="mt-6" onClick={handleGenerate}>
              <AlertTriangle className="mr-2 h-4 w-4" />
              Generate Report
            </Button>
          </CardContent>
        </Card>
      ) : reportQuery.isLoading || reportQuery.isFetching ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
            <p className="mt-4 text-sm text-muted-foreground">Generating risk report...</p>
          </CardContent>
        </Card>
      ) : reportQuery.error ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 p-6 text-center">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="text-destructive">Failed to generate risk report.</p>
            <Button variant="outline" onClick={handleGenerate}>
              Retry
            </Button>
          </CardContent>
        </Card>
      ) : data ? (
        <>
          {/* Risk Distribution Chart */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Risk Distribution</CardTitle>
            </CardHeader>
            <CardContent>
              {data.risk_distribution && Object.keys(data.risk_distribution).length > 0 ? (
                <ResponsiveContainer width="100%" height={300}>
                  <PieChart>
                    <Pie
                      data={Object.entries(data.risk_distribution).map(([level, count]) => ({
                        name: level.replace('_', ' ').replace(/^\w/, (c) => c.toUpperCase()),
                        value: count,
                        color: RISK_LEVEL_COLORS[level] ?? '#94A3B8',
                      }))}
                      cx="50%"
                      cy="50%"
                      innerRadius={60}
                      outerRadius={110}
                      paddingAngle={2}
                      dataKey="value"
                      nameKey="name"
                      label={({ name, value }: { name: string; value: number }) =>
                        value > 0 ? `${name} (${value})` : ''
                      }
                    >
                      {Object.entries(data.risk_distribution).map(([level], idx) => (
                        <Cell key={idx} fill={RISK_LEVEL_COLORS[level] ?? '#94A3B8'} />
                      ))}
                    </Pie>
                    <Tooltip />
                    <Legend />
                  </PieChart>
                </ResponsiveContainer>
              ) : (
                <p className="py-8 text-center text-muted-foreground">
                  No risk distribution data available.
                </p>
              )}
            </CardContent>
          </Card>

          {/* Top 10 Risks */}
          {data.top_risks?.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Top 10 Risks</CardTitle>
                <CardDescription>
                  Highest-scoring risks requiring management attention.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left">
                        <th className="pb-3 pr-4 font-medium text-muted-foreground">#</th>
                        <th className="pb-3 pr-4 font-medium text-muted-foreground">Ref</th>
                        <th className="pb-3 pr-4 font-medium text-muted-foreground">Title</th>
                        <th className="pb-3 pr-4 font-medium text-muted-foreground">Level</th>
                        <th className="pb-3 pr-4 font-medium text-muted-foreground">Score</th>
                        <th className="pb-3 pr-4 font-medium text-muted-foreground">Status</th>
                        <th className="pb-3 font-medium text-muted-foreground">Owner</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.top_risks.slice(0, 10).map((risk, idx) => (
                        <tr key={risk.risk_ref} className="border-b last:border-0 hover:bg-muted/50">
                          <td className="py-3 pr-4 text-muted-foreground">{idx + 1}</td>
                          <td className="py-3 pr-4 font-mono text-xs">{risk.risk_ref}</td>
                          <td className="py-3 pr-4 font-medium">{risk.title}</td>
                          <td className="py-3 pr-4">
                            <Badge className={cn('capitalize', getRiskLevelColor(risk.risk_level))}>
                              {risk.risk_level?.replace('_', ' ')}
                            </Badge>
                          </td>
                          <td className="py-3 pr-4 font-semibold">{risk.risk_score}</td>
                          <td className="py-3 pr-4 capitalize text-muted-foreground">
                            {risk.status?.replace(/_/g, ' ')}
                          </td>
                          <td className="py-3 text-muted-foreground">
                            {risk.owner_name ?? '—'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CardContent>
            </Card>
          )}

          {/* Treatment Progress */}
          {data.treatment_progress && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Treatment Progress</CardTitle>
                <CardDescription>
                  Status of risk treatment actions across the organisation.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-4">
                  <div className="rounded-lg border p-4 text-center">
                    <p className="text-2xl font-bold">{data.treatment_progress.total}</p>
                    <p className="text-xs text-muted-foreground">Total Treatments</p>
                  </div>
                  <div className="rounded-lg border border-green-200 bg-green-50 p-4 text-center dark:border-green-800 dark:bg-green-950/40">
                    <p className="text-2xl font-bold text-green-700 dark:text-green-400">
                      {data.treatment_progress.completed}
                    </p>
                    <p className="text-xs text-green-600 dark:text-green-500">Completed</p>
                  </div>
                  <div className="rounded-lg border border-yellow-200 bg-yellow-50 p-4 text-center dark:border-yellow-800 dark:bg-yellow-950/40">
                    <p className="text-2xl font-bold text-yellow-700 dark:text-yellow-400">
                      {data.treatment_progress.in_progress}
                    </p>
                    <p className="text-xs text-yellow-600 dark:text-yellow-500">In Progress</p>
                  </div>
                  <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-center dark:border-red-800 dark:bg-red-950/40">
                    <p className="text-2xl font-bold text-red-700 dark:text-red-400">
                      {data.treatment_progress.not_started}
                    </p>
                    <p className="text-xs text-red-600 dark:text-red-500">Not Started</p>
                  </div>
                </div>
                {data.treatment_progress.total > 0 && (
                  <div className="mt-4">
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">Overall Progress</span>
                      <span className="font-medium">
                        {formatPercentage(
                          (data.treatment_progress.completed / data.treatment_progress.total) * 100
                        )}
                      </span>
                    </div>
                    <Progress
                      value={(data.treatment_progress.completed / data.treatment_progress.total) * 100}
                      className="mt-2 h-3"
                    />
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {/* Regenerate */}
          <div className="flex justify-center">
            <Button variant="outline" onClick={handleGenerate}>
              Regenerate Report
            </Button>
          </div>
        </>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function ReportsPage() {
  const [activeReport, setActiveReport] = useState<'compliance' | 'risk' | null>(null);

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Reports</h1>
        <p className="text-muted-foreground">
          Generate compliance and risk reports for executive review and regulatory submissions.
        </p>
      </div>

      {/* Report Selection Cards */}
      {activeReport === null && (
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Card
            className="cursor-pointer transition-shadow hover:shadow-lg"
            onClick={() => setActiveReport('compliance')}
          >
            <CardContent className="p-8">
              <div className="flex items-start gap-4">
                <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-primary/10">
                  <Shield className="h-7 w-7 text-primary" />
                </div>
                <div className="flex-1">
                  <h3 className="text-xl font-semibold">Compliance Report</h3>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Comprehensive overview of compliance posture across all adopted frameworks.
                    Includes overall score, framework breakdown, maturity levels, and gap analysis.
                  </p>
                  <Button className="mt-4" variant="outline">
                    Generate
                    <ChevronRight className="ml-1 h-4 w-4" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card
            className="cursor-pointer transition-shadow hover:shadow-lg"
            onClick={() => setActiveReport('risk')}
          >
            <CardContent className="p-8">
              <div className="flex items-start gap-4">
                <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-orange-100 dark:bg-orange-900/30">
                  <AlertTriangle className="h-7 w-7 text-orange-600 dark:text-orange-400" />
                </div>
                <div className="flex-1">
                  <h3 className="text-xl font-semibold">Risk Report</h3>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Risk distribution analysis, top risks ranked by score, and treatment progress.
                    Suitable for board-level risk reporting and ISO 27001 management review.
                  </p>
                  <Button className="mt-4" variant="outline">
                    Generate
                    <ChevronRight className="ml-1 h-4 w-4" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Active Report */}
      {activeReport !== null && (
        <>
          <Button variant="ghost" onClick={() => setActiveReport(null)}>
            <ChevronRight className="mr-2 h-4 w-4 rotate-180" />
            Back to Reports
          </Button>
          {activeReport === 'compliance' && <ComplianceReportSection />}
          {activeReport === 'risk' && <RiskReportSection />}
        </>
      )}
    </div>
  );
}
