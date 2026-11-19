import type { StatsGroup, StatsRow } from '@/shared/api/client';
import { formatPeriod, formatValue } from './format';
import { chartData, type ChartDef } from './metrics';

interface StatsTableProps {
  charts: ChartDef[];
  rows: StatsRow[];
  group: StatsGroup;
}

const StatsTable = ({ charts, rows, group }: StatsTableProps) => {
  const columns = charts.flatMap((chart) => {
    const { series, points } = chartData(chart, rows);
    return series.map((s) => ({
      id: `${chart.id}-${s.key}`,
      label: series.length > 1 ? `${chart.title}: ${s.label}` : chart.title,
      format: chart.format,
      values: points.map((p) => p[s.key] as number | null),
    }));
  });
  return (
    <details className="rounded-lg border border-border bg-card">
      <summary className="cursor-pointer px-4 py-2 text-sm font-medium text-foreground">Show as table</summary>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/60 text-xs text-muted-foreground">
            <tr>
              <th scope="col" className="px-3 py-2 font-medium">Period</th>
              {columns.map((c) => <th key={c.id} scope="col" className="px-3 py-2 font-medium">{c.label}</th>)}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {rows.map((row, i) => (
              <tr key={row.day}>
                <th scope="row" className="whitespace-nowrap px-3 py-1.5 font-medium text-foreground">
                  {formatPeriod(row.day, group)}{row.late ? ' *' : ''}
                </th>
                {columns.map((c) => <td key={c.id} className="px-3 py-1.5 tabular-nums text-subtle-foreground">{formatValue(c.values[i], c.format)}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
        {rows.some((r) => r.late) && (
          <p className="px-3 py-2 text-xs text-muted-foreground">* Computed late: numbers that describe a single moment, such as active accounts, are missing.</p>
        )}
      </div>
    </details>
  );
};

export default StatsTable;
