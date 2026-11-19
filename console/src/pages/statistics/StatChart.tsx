import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import type { StatsGroup, StatsRow } from '@/shared/api/client';
import { formatPeriod, formatTick, formatValue } from './format';
import { chartData, type ChartDef, type ChartSeries } from './metrics';

const AXIS_TICK = { fill: 'hsl(var(--muted-foreground))', fontSize: 12 };
const GRID = 'hsl(var(--border))';
const SURFACE = 'hsl(var(--card))';

interface TooltipEntry {
  dataKey?: string | number;
  value?: number | string | null;
  color?: string;
}

interface TooltipView {
  active?: boolean;
  label?: string | number;
  payload?: TooltipEntry[];
}

interface StatTooltipProps extends TooltipView {
  group: StatsGroup;
  chart: ChartDef;
  series: ChartSeries[];
}

const StatTooltip = ({ active, label, payload, group, chart, series }: StatTooltipProps) => {
  if (!active || !payload?.length || typeof label !== 'string') return null;
  const byKey = new Map(payload.map((p) => [String(p.dataKey), p]));
  const shown = [...series].reverse().filter((s) => byKey.has(s.key));
  const total = shown.reduce((sum, s) => sum + (Number(byKey.get(s.key)?.value) || 0), 0);
  return (
    <div className="min-w-40 rounded-md border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
      <p className="mb-1 font-medium">{formatPeriod(label, group)}</p>
      <ul className="space-y-0.5">
        {shown.map((s) => (
          <li key={s.key} className="flex items-center gap-2">
            <span aria-hidden="true" className="h-2 w-2 rounded-sm" style={{ background: s.color }} />
            <span className="text-muted-foreground">{s.label}</span>
            <span className="ml-auto pl-3 tabular-nums">{formatValue(byKey.get(s.key)?.value as number | null, chart.format)}</span>
          </li>
        ))}
      </ul>
      {shown.length > 1 && chart.kind === 'bar' && (
        <p className="mt-1 flex border-t border-border pt-1 font-medium">
          <span>Total</span>
          <span className="ml-auto tabular-nums">{formatValue(total, chart.format)}</span>
        </p>
      )}
    </div>
  );
};

interface StatChartProps {
  chart: ChartDef;
  rows: StatsRow[];
  group: StatsGroup;
}

const StatChart = ({ chart, rows, group }: StatChartProps) => {
  const { series, points } = chartData(chart, rows);
  const empty = series.length === 0 || points.every((p) => series.every((s) => p[s.key] === null || p[s.key] === undefined));
  const axes = (
    <>
      <CartesianGrid vertical={false} stroke={GRID} strokeDasharray="0" />
      <XAxis dataKey="day" tick={AXIS_TICK} tickLine={false} axisLine={{ stroke: GRID }} tickFormatter={(d: string) => formatTick(d, group)} minTickGap={24} />
      <YAxis tick={AXIS_TICK} tickLine={false} axisLine={false} width={64} allowDecimals={false} tickFormatter={(v: number) => formatValue(v, chart.format)} />
      <Tooltip
        cursor={chart.kind === 'bar' ? { fill: 'hsl(var(--muted))', opacity: 0.6 } : { stroke: 'hsl(var(--muted-foreground))', strokeWidth: 1 }}
        content={(props) => <StatTooltip {...(props as unknown as TooltipView)} group={group} chart={chart} series={series} />}
      />
    </>
  );

  return (
    <figure aria-labelledby={`chart-${chart.id}`} className="rounded-lg border border-border bg-card p-4">
      <figcaption className="mb-3">
        <h3 id={`chart-${chart.id}`} className="text-sm font-medium text-foreground">{chart.title}</h3>
        <p className="text-xs text-muted-foreground">{chart.description}</p>
      </figcaption>
      {empty ? (
        <p className="flex h-56 items-center justify-center text-sm text-muted-foreground">No data for this period yet.</p>
      ) : (
        <div className="h-56" role="img" aria-label={`${chart.title} chart; the table view below lists the same numbers`}>
          <ResponsiveContainer width="100%" height="100%">
            {chart.kind === 'line' ? (
              <LineChart data={points} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
                {axes}
                {series.map((s) => (
                  <Line key={s.key} type="monotone" dataKey={s.key} name={s.label} stroke={s.color} strokeWidth={2}
                    dot={false} activeDot={{ r: 4, stroke: SURFACE, strokeWidth: 2 }} connectNulls={false} isAnimationActive={false} />
                ))}
              </LineChart>
            ) : (
              <BarChart data={points} margin={{ top: 4, right: 8, bottom: 0, left: 0 }} barCategoryGap="20%">
                {axes}
                {series.map((s, i) => (
                  <Bar key={s.key} dataKey={s.key} name={s.label} stackId="stack" fill={s.color} stroke={SURFACE} strokeWidth={1}
                    radius={i === series.length - 1 ? [4, 4, 0, 0] : 0} maxBarSize={32} isAnimationActive={false} />
                ))}
              </BarChart>
            )}
          </ResponsiveContainer>
        </div>
      )}
      {!empty && series.length > 1 && (
        <ul aria-label={`${chart.title} legend`} className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          {series.map((s) => (
            <li key={s.key} className="flex items-center gap-1.5">
              <span aria-hidden="true" className="h-2 w-2 rounded-sm" style={{ background: s.color }} />
              {s.label}
            </li>
          ))}
        </ul>
      )}
    </figure>
  );
};

export default StatChart;
