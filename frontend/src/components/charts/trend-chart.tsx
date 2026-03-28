'use client';

import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import { format, parseISO } from 'date-fns';

interface TrendDataPoint {
  date: string;
  [key: string]: string | number | undefined;
}

interface TrendLine {
  key: string;
  color: string;
  label: string;
}

interface TrendChartProps {
  data: TrendDataPoint[];
  lines: TrendLine[];
  height?: number;
}

interface CustomTooltipProps {
  active?: boolean;
  payload?: Array<{
    name: string;
    value: number;
    color: string;
    dataKey: string;
  }>;
  label?: string;
  lines: TrendLine[];
}

function CustomTooltip({ active, payload, label, lines }: CustomTooltipProps) {
  if (!active || !payload || payload.length === 0 || !label) return null;

  let formattedDate: string;
  try {
    formattedDate = format(parseISO(label), 'dd MMM yyyy');
  } catch {
    formattedDate = label;
  }

  const lineMap = new Map(lines.map((l) => [l.key, l.label]));

  return (
    <div className="rounded-md border bg-popover px-3 py-2 text-sm shadow-md">
      <p className="mb-1 font-medium">{formattedDate}</p>
      {payload.map((entry) => (
        <p key={entry.dataKey} style={{ color: entry.color }}>
          {lineMap.get(entry.dataKey) || entry.name}: {entry.value}
        </p>
      ))}
    </div>
  );
}

function formatXAxisTick(dateStr: string): string {
  try {
    return format(parseISO(dateStr), 'MMM yy');
  } catch {
    return dateStr;
  }
}

export function TrendChart({ data, lines, height = 300 }: TrendChartProps) {
  if (!data || data.length === 0 || !lines || lines.length === 0) {
    return (
      <div
        className="flex items-center justify-center text-sm text-muted-foreground"
        style={{ height }}
      >
        No trend data available
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
        <XAxis
          dataKey="date"
          tickFormatter={formatXAxisTick}
          tick={{ fontSize: 12, fill: 'hsl(var(--muted-foreground))' }}
          stroke="hsl(var(--border))"
        />
        <YAxis
          tick={{ fontSize: 12, fill: 'hsl(var(--muted-foreground))' }}
          stroke="hsl(var(--border))"
        />
        <Tooltip content={<CustomTooltip lines={lines} />} />
        <Legend
          formatter={(value: string) => {
            const line = lines.find((l) => l.key === value);
            return line?.label || value;
          }}
          wrapperStyle={{ fontSize: 12 }}
        />
        {lines.map((line) => (
          <Line
            key={line.key}
            type="monotone"
            dataKey={line.key}
            name={line.key}
            stroke={line.color}
            strokeWidth={2}
            dot={{ r: 3, fill: line.color }}
            activeDot={{ r: 5 }}
            connectNulls
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
}
