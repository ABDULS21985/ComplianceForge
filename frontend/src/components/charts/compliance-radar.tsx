'use client';

import {
  RadarChart,
  PolarGrid,
  PolarAngleAxis,
  PolarRadiusAxis,
  Radar,
  ResponsiveContainer,
  Tooltip,
} from 'recharts';
import { FRAMEWORK_COLORS } from '@/lib/constants';

interface ComplianceRadarScore {
  framework_name: string;
  compliance_score: number;
  avg_maturity_level: number;
  target_maturity?: number;
}

interface ComplianceRadarProps {
  scores: ComplianceRadarScore[];
  height?: number;
}

interface CustomTooltipProps {
  active?: boolean;
  payload?: Array<{
    name: string;
    value: number;
    dataKey: string;
  }>;
  label?: string;
}

function CustomTooltip({ active, payload, label }: CustomTooltipProps) {
  if (!active || !payload || payload.length === 0) return null;

  return (
    <div className="rounded-md border bg-popover px-3 py-2 text-sm shadow-md">
      <p className="font-medium">{label}</p>
      {payload.map((entry) => (
        <p key={entry.dataKey} className="text-muted-foreground">
          {entry.dataKey === 'compliance_score'
            ? `Compliance: ${entry.value.toFixed(1)}%`
            : entry.dataKey === 'target_maturity'
              ? `Target Maturity: ${entry.value}`
              : `Maturity: ${entry.value.toFixed(1)}`}
        </p>
      ))}
    </div>
  );
}

export function ComplianceRadar({ scores, height = 400 }: ComplianceRadarProps) {
  if (!scores || scores.length === 0) {
    return (
      <div
        className="flex items-center justify-center text-sm text-muted-foreground"
        style={{ height }}
      >
        No compliance data available
      </div>
    );
  }

  const data = scores.map((s) => ({
    framework: s.framework_name,
    compliance_score: s.compliance_score,
    avg_maturity_level: s.avg_maturity_level,
    ...(s.target_maturity != null ? { target_maturity: s.target_maturity } : {}),
  }));

  const hasTarget = scores.some((s) => s.target_maturity != null);

  return (
    <ResponsiveContainer width="100%" height={height}>
      <RadarChart data={data} cx="50%" cy="50%" outerRadius="75%">
        <PolarGrid stroke="hsl(var(--border))" />
        <PolarAngleAxis
          dataKey="framework"
          tick={{ fontSize: 12, fill: 'hsl(var(--muted-foreground))' }}
        />
        <PolarRadiusAxis
          angle={90}
          domain={[0, 100]}
          tick={{ fontSize: 10, fill: 'hsl(var(--muted-foreground))' }}
          tickCount={6}
        />
        <Radar
          name="Compliance Score"
          dataKey="compliance_score"
          stroke={FRAMEWORK_COLORS.ISO27001 || '#1A56DB'}
          fill={FRAMEWORK_COLORS.ISO27001 || '#1A56DB'}
          fillOpacity={0.25}
          strokeWidth={2}
        />
        {hasTarget && (
          <Radar
            name="Target Maturity"
            dataKey="target_maturity"
            stroke={FRAMEWORK_COLORS.NCSC_CAF || '#059669'}
            fill="none"
            strokeWidth={2}
            strokeDasharray="5 5"
          />
        )}
        <Tooltip content={<CustomTooltip />} />
      </RadarChart>
    </ResponsiveContainer>
  );
}
