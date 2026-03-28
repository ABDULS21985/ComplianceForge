'use client';

import { useState } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface KPICard {
  key: string;
  label: string;
  value: number | string;
  previous_value?: number;
  trend: 'up' | 'down' | 'flat';
  trend_percent?: number;
  sparkline?: number[];
  color: string;
}

interface ComplianceTrend {
  month: string;
  frameworks: Record<string, number>;
}

interface RiskHeatmapCell {
  likelihood: number;
  impact: number;
  count: number;
}

interface IncidentTrend {
  month: string;
  volume: number;
  mttr_hours: number;
}

interface BenchmarkMetric {
  metric: string;
  your_value: number;
  peer_average: number;
  peer_p75: number;
  peer_p90: number;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AnalyticsDashboardPage() {
  const [compliancePeriod, setCompliancePeriod] = useState('12m');
  const [exportFormat, setExportFormat] = useState('pdf');

  // Fetch snapshots (KPIs)
  const { data: snapshotsData, isLoading: kpiLoading } = useQuery({
    queryKey: ['analytics-snapshots'],
    queryFn: () => api.analytics.snapshots(),
  });

  // Fetch compliance trends
  const { data: complianceTrendsData } = useQuery({
    queryKey: ['analytics-compliance-trends', compliancePeriod],
    queryFn: () => api.analytics.complianceTrends({ period: compliancePeriod }),
  });

  // Fetch risk trends
  const { data: riskTrendsData } = useQuery({
    queryKey: ['analytics-risk-trends'],
    queryFn: () => api.analytics.riskTrends(),
  });

  // Fetch benchmarks
  const { data: benchmarksData } = useQuery({
    queryKey: ['analytics-benchmarks'],
    queryFn: () => api.analytics.benchmarks(),
  });

  // Fetch metrics for incidents
  const { data: incidentMetricsData } = useQuery({
    queryKey: ['analytics-incident-metrics'],
    queryFn: () => api.analytics.metrics('incidents', { period: '12m' }),
  });

  // Export mutation
  const exportMutation = useMutation({
    mutationFn: (data: any) => api.analytics.exportData(data),
  });

  // Parse data
  const kpis: KPICard[] = snapshotsData?.kpis ?? snapshotsData?.items ?? buildDefaultKPIs(snapshotsData);
  const complianceTrends: ComplianceTrend[] = complianceTrendsData?.trends ?? complianceTrendsData?.items ?? complianceTrendsData ?? [];
  const riskHeatmap: RiskHeatmapCell[] = riskTrendsData?.heatmap ?? [];
  const riskTrends: { month: string; count: number }[] = riskTrendsData?.trends ?? riskTrendsData?.items ?? [];
  const incidentTrends: IncidentTrend[] = incidentMetricsData?.trends ?? incidentMetricsData?.items ?? incidentMetricsData ?? [];
  const benchmarks: BenchmarkMetric[] = benchmarksData?.metrics ?? benchmarksData?.items ?? benchmarksData ?? [];

  function handleExport() {
    exportMutation.mutate({ format: exportFormat, sections: ['kpis', 'compliance', 'risks', 'incidents', 'benchmarks'] });
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Advanced Analytics</h1>
        <div className="flex items-center gap-2">
          <select
            value={exportFormat}
            onChange={(e) => setExportFormat(e.target.value)}
            className="border rounded px-3 py-2 text-sm"
          >
            <option value="pdf">PDF</option>
            <option value="xlsx">Excel</option>
            <option value="csv">CSV</option>
          </select>
          <button
            onClick={handleExport}
            disabled={exportMutation.isPending}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {exportMutation.isPending ? 'Exporting...' : 'Export'}
          </button>
        </div>
      </div>

      {/* KPI Row */}
      {kpiLoading ? (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-32 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {kpis.map((kpi) => (
            <KPICardComponent key={kpi.key} kpi={kpi} />
          ))}
        </div>
      )}

      {/* Compliance Trend */}
      <div className="bg-white border rounded-lg p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Compliance Trends</h2>
          <select
            value={compliancePeriod}
            onChange={(e) => setCompliancePeriod(e.target.value)}
            className="border rounded px-3 py-1.5 text-sm"
          >
            <option value="6m">6 Months</option>
            <option value="12m">12 Months</option>
            <option value="24m">24 Months</option>
          </select>
        </div>
        {complianceTrends.length > 0 ? (
          <div className="space-y-3">
            {/* Chart header: framework legend */}
            {complianceTrends[0]?.frameworks && (
              <div className="flex flex-wrap gap-3 mb-2">
                {Object.keys(complianceTrends[0].frameworks).map((fw, idx) => (
                  <div key={fw} className="flex items-center gap-1.5">
                    <div className={`w-3 h-3 rounded-full ${CHART_COLORS[idx % CHART_COLORS.length]}`} />
                    <span className="text-xs text-gray-600">{fw}</span>
                  </div>
                ))}
              </div>
            )}
            {/* Simplified bar chart representation */}
            <div className="overflow-x-auto">
              <div className="flex items-end gap-1 min-w-[600px] h-48">
                {complianceTrends.map((point, idx) => {
                  const frameworks = point.frameworks ?? {};
                  const values = Object.values(frameworks);
                  const avg = values.length > 0 ? values.reduce((a, b) => a + b, 0) / values.length : 0;
                  return (
                    <div key={idx} className="flex-1 flex flex-col items-center gap-1">
                      <div className="w-full flex flex-col items-center justify-end h-40">
                        {Object.entries(frameworks).map(([fw, val], fwIdx) => (
                          <div
                            key={fw}
                            className={`w-full max-w-[20px] rounded-t ${CHART_BG_COLORS[fwIdx % CHART_BG_COLORS.length]}`}
                            style={{ height: `${(val / 100) * 140}px`, marginLeft: fwIdx * 6 }}
                            title={`${fw}: ${val}%`}
                          />
                        ))}
                      </div>
                      <span className="text-xs text-gray-400 truncate w-full text-center">{point.month}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        ) : (
          <div className="text-center py-12 text-gray-400 text-sm">No compliance trend data available</div>
        )}
      </div>

      {/* Risk Heatmap + Risk Trend */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Risk Heatmap */}
        <div className="bg-white border rounded-lg p-5">
          <h2 className="text-lg font-semibold mb-4">Risk Heatmap</h2>
          <div className="space-y-1">
            {[5, 4, 3, 2, 1].map((likelihood) => (
              <div key={likelihood} className="flex items-center gap-1">
                <span className="text-xs text-gray-400 w-6 text-right">{likelihood}</span>
                {[1, 2, 3, 4, 5].map((impact) => {
                  const cell = riskHeatmap.find((c) => c.likelihood === likelihood && c.impact === impact);
                  const count = cell?.count ?? 0;
                  const score = likelihood * impact;
                  return (
                    <div
                      key={impact}
                      className={`w-14 h-10 rounded flex items-center justify-center text-xs font-medium ${
                        score >= 20 ? 'bg-red-500 text-white'
                          : score >= 12 ? 'bg-orange-400 text-white'
                          : score >= 6 ? 'bg-amber-300 text-gray-800'
                          : 'bg-green-200 text-gray-700'
                      }`}
                      title={`L:${likelihood} I:${impact} Count:${count}`}
                    >
                      {count > 0 ? count : ''}
                    </div>
                  );
                })}
              </div>
            ))}
            <div className="flex items-center gap-1 mt-1">
              <span className="w-6" />
              {[1, 2, 3, 4, 5].map((i) => (
                <span key={i} className="w-14 text-center text-xs text-gray-400">{i}</span>
              ))}
            </div>
            <div className="flex justify-between text-xs text-gray-400 mt-1">
              <span className="ml-8">Impact &rarr;</span>
              <span>Likelihood &uarr;</span>
            </div>
          </div>
        </div>

        {/* Risk Trend Bar Chart */}
        <div className="bg-white border rounded-lg p-5">
          <h2 className="text-lg font-semibold mb-4">Risk Trend</h2>
          {riskTrends.length > 0 ? (
            <div className="flex items-end gap-2 h-48">
              {riskTrends.map((point, idx) => {
                const maxCount = Math.max(...riskTrends.map((p) => p.count), 1);
                return (
                  <div key={idx} className="flex-1 flex flex-col items-center gap-1">
                    <span className="text-xs text-gray-500">{point.count}</span>
                    <div
                      className="w-full max-w-[32px] bg-blue-500 rounded-t"
                      style={{ height: `${(point.count / maxCount) * 140}px` }}
                    />
                    <span className="text-xs text-gray-400 truncate w-full text-center">{point.month}</span>
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="text-center py-12 text-gray-400 text-sm">No risk trend data available</div>
          )}
        </div>
      </div>

      {/* Incident Volume + MTTR */}
      <div className="bg-white border rounded-lg p-5">
        <h2 className="text-lg font-semibold mb-4">Incident Volume &amp; MTTR Trend</h2>
        {incidentTrends.length > 0 ? (
          <div className="space-y-4">
            {/* Legend */}
            <div className="flex gap-4">
              <div className="flex items-center gap-1.5">
                <div className="w-3 h-3 rounded bg-blue-500" />
                <span className="text-xs text-gray-600">Volume</span>
              </div>
              <div className="flex items-center gap-1.5">
                <div className="w-3 h-3 rounded bg-amber-500" />
                <span className="text-xs text-gray-600">MTTR (hours)</span>
              </div>
            </div>
            <div className="flex items-end gap-2 h-48">
              {incidentTrends.map((point, idx) => {
                const maxVol = Math.max(...incidentTrends.map((p) => p.volume), 1);
                const maxMttr = Math.max(...incidentTrends.map((p) => p.mttr_hours), 1);
                return (
                  <div key={idx} className="flex-1 flex flex-col items-center gap-1">
                    <div className="flex gap-0.5 items-end h-36">
                      <div
                        className="w-3 bg-blue-500 rounded-t"
                        style={{ height: `${(point.volume / maxVol) * 130}px` }}
                        title={`Volume: ${point.volume}`}
                      />
                      <div
                        className="w-3 bg-amber-500 rounded-t"
                        style={{ height: `${(point.mttr_hours / maxMttr) * 130}px` }}
                        title={`MTTR: ${point.mttr_hours}h`}
                      />
                    </div>
                    <span className="text-xs text-gray-400 truncate w-full text-center">{point.month}</span>
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
          <div className="text-center py-12 text-gray-400 text-sm">No incident data available</div>
        )}
      </div>

      {/* Peer Benchmarking Radar Chart (simplified) */}
      <div className="bg-white border rounded-lg p-5">
        <h2 className="text-lg font-semibold mb-4">Peer Benchmarking</h2>
        {benchmarks.length > 0 ? (
          <div className="space-y-3">
            <div className="flex gap-4 mb-2">
              <div className="flex items-center gap-1.5">
                <div className="w-3 h-3 rounded bg-blue-500" />
                <span className="text-xs text-gray-600">Your Score</span>
              </div>
              <div className="flex items-center gap-1.5">
                <div className="w-3 h-3 rounded bg-gray-300" />
                <span className="text-xs text-gray-600">Peer Average</span>
              </div>
              <div className="flex items-center gap-1.5">
                <div className="w-3 h-3 rounded bg-green-400" />
                <span className="text-xs text-gray-600">Peer P75</span>
              </div>
            </div>
            <div className="space-y-2">
              {benchmarks.map((bm) => (
                <div key={bm.metric} className="space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-700 font-medium">{bm.metric}</span>
                    <span className="text-sm font-bold text-blue-600">{bm.your_value}%</span>
                  </div>
                  <div className="relative h-4 bg-gray-100 rounded-full overflow-hidden">
                    {/* Peer average marker */}
                    <div
                      className="absolute top-0 h-full w-0.5 bg-gray-400"
                      style={{ left: `${bm.peer_average}%` }}
                      title={`Peer avg: ${bm.peer_average}%`}
                    />
                    {/* Peer P75 marker */}
                    <div
                      className="absolute top-0 h-full w-0.5 bg-green-500"
                      style={{ left: `${bm.peer_p75}%` }}
                      title={`P75: ${bm.peer_p75}%`}
                    />
                    {/* Your value bar */}
                    <div
                      className={`h-full rounded-full ${bm.your_value >= bm.peer_average ? 'bg-blue-500' : 'bg-amber-500'}`}
                      style={{ width: `${bm.your_value}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className="text-center py-12 text-gray-400 text-sm">No benchmarking data available</div>
        )}
      </div>

      {/* Export success */}
      {exportMutation.isSuccess && (
        <div className="fixed bottom-6 right-6 bg-green-600 text-white px-4 py-3 rounded-lg shadow-lg text-sm font-medium">
          Export generated successfully!
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const CHART_COLORS = ['bg-blue-500', 'bg-purple-500', 'bg-green-500', 'bg-orange-500', 'bg-cyan-500', 'bg-red-500'];
const CHART_BG_COLORS = ['bg-blue-400', 'bg-purple-400', 'bg-green-400', 'bg-orange-400', 'bg-cyan-400', 'bg-red-400'];

function buildDefaultKPIs(data: any): KPICard[] {
  if (!data) return [];
  return [
    {
      key: 'compliance_score',
      label: 'Compliance Score',
      value: data.compliance_score ?? '--',
      trend: data.compliance_trend ?? 'flat',
      trend_percent: data.compliance_change,
      sparkline: data.compliance_sparkline,
      color: 'blue',
    },
    {
      key: 'risks',
      label: 'Open Risks',
      value: data.open_risks ?? '--',
      trend: data.risks_trend ?? 'flat',
      trend_percent: data.risks_change,
      sparkline: data.risks_sparkline,
      color: 'red',
    },
    {
      key: 'incidents',
      label: 'Open Incidents',
      value: data.open_incidents ?? '--',
      trend: data.incidents_trend ?? 'flat',
      trend_percent: data.incidents_change,
      sparkline: data.incidents_sparkline,
      color: 'amber',
    },
    {
      key: 'findings',
      label: 'Open Findings',
      value: data.open_findings ?? '--',
      trend: data.findings_trend ?? 'flat',
      trend_percent: data.findings_change,
      sparkline: data.findings_sparkline,
      color: 'purple',
    },
    {
      key: 'policies',
      label: 'Active Policies',
      value: data.active_policies ?? '--',
      trend: data.policies_trend ?? 'flat',
      trend_percent: data.policies_change,
      sparkline: data.policies_sparkline,
      color: 'green',
    },
    {
      key: 'vendors',
      label: 'Active Vendors',
      value: data.active_vendors ?? '--',
      trend: data.vendors_trend ?? 'flat',
      trend_percent: data.vendors_change,
      sparkline: data.vendors_sparkline,
      color: 'cyan',
    },
  ];
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function KPICardComponent({ kpi }: { kpi: KPICard }) {
  const colorMap: Record<string, { bg: string; text: string; border: string }> = {
    blue: { bg: 'bg-blue-50', text: 'text-blue-700', border: 'border-blue-200' },
    red: { bg: 'bg-red-50', text: 'text-red-700', border: 'border-red-200' },
    amber: { bg: 'bg-amber-50', text: 'text-amber-700', border: 'border-amber-200' },
    purple: { bg: 'bg-purple-50', text: 'text-purple-700', border: 'border-purple-200' },
    green: { bg: 'bg-green-50', text: 'text-green-700', border: 'border-green-200' },
    cyan: { bg: 'bg-cyan-50', text: 'text-cyan-700', border: 'border-cyan-200' },
  };

  const colors = colorMap[kpi.color] ?? colorMap.blue;
  const trendArrow = kpi.trend === 'up' ? '\u2191' : kpi.trend === 'down' ? '\u2193' : '\u2192';
  const trendColor = kpi.key === 'risks' || kpi.key === 'incidents' || kpi.key === 'findings'
    ? (kpi.trend === 'down' ? 'text-green-600' : kpi.trend === 'up' ? 'text-red-600' : 'text-gray-400')
    : (kpi.trend === 'up' ? 'text-green-600' : kpi.trend === 'down' ? 'text-red-600' : 'text-gray-400');

  return (
    <div className={`rounded-lg border p-4 ${colors.bg} ${colors.border}`}>
      <p className={`text-xs font-medium opacity-80 ${colors.text}`}>{kpi.label}</p>
      <div className="flex items-end justify-between mt-1">
        <p className={`text-2xl font-bold ${colors.text}`}>{kpi.value}</p>
        <div className="flex items-center gap-0.5">
          <span className={`text-sm font-semibold ${trendColor}`}>{trendArrow}</span>
          {kpi.trend_percent !== undefined && (
            <span className={`text-xs ${trendColor}`}>{Math.abs(kpi.trend_percent)}%</span>
          )}
        </div>
      </div>
      {/* Sparkline */}
      {kpi.sparkline && kpi.sparkline.length > 0 && (
        <div className="flex items-end gap-px mt-2 h-6">
          {kpi.sparkline.map((val, idx) => {
            const max = Math.max(...kpi.sparkline!, 1);
            return (
              <div
                key={idx}
                className={`flex-1 rounded-sm ${colors.text} opacity-40`}
                style={{ height: `${(val / max) * 100}%`, minHeight: '2px', backgroundColor: 'currentColor' }}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}
