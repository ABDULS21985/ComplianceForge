'use client';

import { useState, useCallback, useMemo } from 'react';
import type { RiskHeatmapEntry } from '@/types';
import { cn } from '@/lib/utils';

interface RiskHeatmapProps {
  risks: RiskHeatmapEntry[];
  mode: 'inherent' | 'residual';
  onCellClick?: (likelihood: number, impact: number, risksInCell: RiskHeatmapEntry[]) => void;
}

const LIKELIHOOD_LABELS = ['Rare', 'Unlikely', 'Possible', 'Likely', 'Almost Certain'];
const IMPACT_LABELS = ['Insignificant', 'Minor', 'Moderate', 'Major', 'Catastrophic'];

function getCellColor(score: number): string {
  if (score >= 15) return 'bg-red-500/80 hover:bg-red-500';
  if (score >= 9) return 'bg-orange-500/80 hover:bg-orange-500';
  if (score >= 4) return 'bg-yellow-500/80 hover:bg-yellow-500';
  return 'bg-green-500/80 hover:bg-green-500';
}

function getCellTextColor(score: number): string {
  if (score >= 9) return 'text-white';
  return 'text-gray-900 dark:text-white';
}

export function RiskHeatmap({ risks, mode, onCellClick }: RiskHeatmapProps) {
  const [hoveredCell, setHoveredCell] = useState<{ l: number; i: number } | null>(null);

  const riskMap = useMemo(() => {
    const map = new Map<string, RiskHeatmapEntry[]>();
    for (const risk of risks) {
      const likelihood = mode === 'inherent' ? risk.inherent_likelihood : risk.residual_likelihood;
      const impact = mode === 'inherent' ? risk.inherent_impact : risk.residual_impact;
      if (likelihood == null || impact == null) continue;
      const key = `${likelihood}-${impact}`;
      const arr = map.get(key) || [];
      arr.push(risk);
      map.set(key, arr);
    }
    return map;
  }, [risks, mode]);

  const handleCellClick = useCallback(
    (likelihood: number, impact: number) => {
      const key = `${likelihood}-${impact}`;
      const cellRisks = riskMap.get(key) || [];
      onCellClick?.(likelihood, impact, cellRisks);
    },
    [riskMap, onCellClick]
  );

  if (!risks || risks.length === 0) {
    return (
      <div className="flex h-[360px] items-center justify-center text-sm text-muted-foreground">
        No risk data available for heatmap
      </div>
    );
  }

  return (
    <div className="w-full overflow-x-auto">
      <div className="min-w-[400px]">
        {/* Title row */}
        <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
          <span className="font-medium uppercase tracking-wider">
            {mode === 'inherent' ? 'Inherent' : 'Residual'} Risk Heatmap
          </span>
        </div>

        <div className="flex">
          {/* Y-axis label */}
          <div className="flex w-6 shrink-0 items-center justify-center">
            <span className="-rotate-90 whitespace-nowrap text-xs font-medium text-muted-foreground">
              Impact
            </span>
          </div>

          {/* Y-axis labels column */}
          <div className="flex w-24 shrink-0 flex-col-reverse">
            {IMPACT_LABELS.map((label, idx) => (
              <div
                key={idx}
                className="flex h-16 items-center justify-end pr-2 text-xs text-muted-foreground"
              >
                <span className="text-right leading-tight">
                  {idx + 1}. {label}
                </span>
              </div>
            ))}
          </div>

          {/* Grid */}
          <div className="flex-1">
            <div className="flex flex-col-reverse">
              {[1, 2, 3, 4, 5].map((impact) => (
                <div key={impact} className="flex">
                  {[1, 2, 3, 4, 5].map((likelihood) => {
                    const score = likelihood * impact;
                    const key = `${likelihood}-${impact}`;
                    const cellRisks = riskMap.get(key) || [];
                    const count = cellRisks.length;
                    const isHovered =
                      hoveredCell?.l === likelihood && hoveredCell?.i === impact;

                    return (
                      <div
                        key={key}
                        className="relative flex-1 p-0.5"
                        onMouseEnter={() => setHoveredCell({ l: likelihood, i: impact })}
                        onMouseLeave={() => setHoveredCell(null)}
                      >
                        <button
                          type="button"
                          onClick={() => handleCellClick(likelihood, impact)}
                          className={cn(
                            'flex h-16 w-full flex-col items-center justify-center rounded-md border border-background/20 transition-colors',
                            getCellColor(score),
                            getCellTextColor(score),
                            count > 0 ? 'cursor-pointer' : 'cursor-default'
                          )}
                        >
                          {count > 0 && (
                            <span className="text-lg font-bold">{count}</span>
                          )}
                          <span className="text-[10px] opacity-70">{score}</span>
                        </button>

                        {/* Tooltip popover */}
                        {isHovered && cellRisks.length > 0 && (
                          <div className="absolute bottom-full left-1/2 z-50 mb-2 w-56 -translate-x-1/2 rounded-md border bg-popover p-2 text-sm shadow-lg">
                            <p className="mb-1 font-medium text-popover-foreground">
                              {cellRisks.length} risk{cellRisks.length !== 1 ? 's' : ''}
                            </p>
                            <ul className="max-h-32 space-y-0.5 overflow-y-auto">
                              {cellRisks.map((r) => (
                                <li
                                  key={r.risk_id}
                                  className="truncate text-xs text-muted-foreground"
                                  title={r.title}
                                >
                                  {r.risk_ref}: {r.title}
                                </li>
                              ))}
                            </ul>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>

            {/* X-axis labels */}
            <div className="mt-1 flex">
              {LIKELIHOOD_LABELS.map((label, idx) => (
                <div
                  key={idx}
                  className="flex-1 text-center text-xs text-muted-foreground"
                >
                  <span className="leading-tight">
                    {idx + 1}. {label}
                  </span>
                </div>
              ))}
            </div>

            {/* X-axis title */}
            <div className="mt-1 text-center text-xs font-medium text-muted-foreground">
              Likelihood
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
