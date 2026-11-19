import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Loader2, RefreshCw } from 'lucide-react';
import {
  ApiError, fetchStats, fetchStatsToday, statsCsvUrl,
  type StatsGroup, type StatsQuery, type StatsRange, type StatsToday,
} from '@/shared/api/client';
import { cn } from '@/shared/lib/utils';
import { Button } from '@/shared/ui/button';
import { addDays, isoDay } from './format';
import { MODERATION_CHARTS, MODERATION_TILES, USAGE_CHARTS, USAGE_TILES, type ChartDef } from './metrics';
import StatChart from './StatChart';
import StatsTable from './StatsTable';
import TodayTiles from './TodayTiles';

const RANGES = [
  { id: '30d', label: '30 days', days: 30 },
  { id: '90d', label: '90 days', days: 90 },
  { id: '1y', label: '1 year', days: 365 },
  { id: '3y', label: '3 years', days: 365 * 3 },
] as const;

type RangeId = (typeof RANGES)[number]['id'];

const GROUPS: { id: StatsGroup; label: string }[] = [
  { id: 'day', label: 'Day' },
  { id: 'week', label: 'Week' },
  { id: 'month', label: 'Month' },
];

interface SegmentedProps<T extends string> {
  label: string;
  value: T;
  options: readonly { id: T; label: string }[];
  onChange: (value: T) => void;
}

const Segmented = <T extends string>({ label, value, options, onChange }: SegmentedProps<T>) => (
  <div role="radiogroup" aria-label={label} className="inline-flex rounded-md border border-border bg-card p-0.5">
    {options.map((o) => (
      <button key={o.id} type="button" role="radio" aria-checked={value === o.id} onClick={() => onChange(o.id)}
        className={cn('rounded px-2.5 py-1 text-sm transition-colors',
          value === o.id ? 'bg-secondary font-medium text-secondary-foreground' : 'text-muted-foreground hover:text-foreground')}>
        {o.label}
      </button>
    ))}
  </div>
);

const ChartSection = ({ id, title, charts, data }: { id: string; title: string; charts: ChartDef[]; data: StatsRange }) => (
  <section aria-labelledby={id} className="space-y-3">
    <h2 id={id} className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</h2>
    <div className="grid gap-4 lg:grid-cols-2">
      {charts.map((chart) => <StatChart key={chart.id} chart={chart} rows={data.rows} group={data.group} />)}
    </div>
    <StatsTable charts={charts} rows={data.rows} group={data.group} />
  </section>
);

const systemNow = () => new Date();

const StatisticsPage = ({ now = systemNow }: { now?: () => Date }) => {
  const [range, setRange] = useState<RangeId>('30d');
  const [group, setGroup] = useState<StatsGroup>('day');
  const [data, setData] = useState<StatsRange | null>(null);
  const [today, setToday] = useState<StatsToday | null>(null);
  const [todayNote, setTodayNote] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const query = useMemo<StatsQuery>(() => {
    const to = addDays(isoDay(now()), -1);
    const days = RANGES.find((r) => r.id === range)!.days;
    return { from: addDays(to, 1 - days), to, group };
  }, [range, group, now]);

  const load = useCallback(async () => {
    setLoading(true);
    const [stats, live] = await Promise.allSettled([fetchStats(query), fetchStatsToday()]);
    if (stats.status === 'fulfilled') {
      setData(stats.value);
      setError('');
    } else {
      setError('Statistics could not be loaded.');
    }
    if (live.status === 'fulfilled') {
      setToday(live.value);
      setTodayNote('');
    } else {
      setToday(null);
      setTodayNote(live.reason instanceof ApiError && live.reason.status === 503
        ? "Today's numbers are not available: the Messenger database is not connected."
        : "Today's numbers could not be loaded.");
    }
    setLoading(false);
  }, [query]);

  useEffect(() => {
    void load();
  }, [load]);

  const admin = (data?.scope ?? today?.scope) === 'all';

  return (
    <section aria-labelledby="statistics-title" className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <h1 id="statistics-title" className="text-2xl font-semibold tracking-tight text-foreground">Statistics</h1>
        <div className="flex-1" />
        <Segmented label="Period" value={range} options={RANGES} onChange={setRange} />
        <Segmented label="Group by" value={group} options={GROUPS} onChange={setGroup} />
        <Button variant="outline" size="sm" asChild>
          <a href={statsCsvUrl(query)} download>
            <Download aria-hidden="true" />
            Download CSV
          </a>
        </Button>
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading} aria-label="Refresh">
          {loading ? <Loader2 aria-hidden="true" className="animate-spin" /> : <RefreshCw aria-hidden="true" />}
        </Button>
      </div>

      {error && <p role="alert" className="rounded-md bg-destructive-soft px-3 py-2 text-sm text-destructive">{error}</p>}
      {todayNote && <p className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">{todayNote}</p>}

      {today && admin && <TodayTiles id="usage-today" title="Usage today" tiles={USAGE_TILES} today={today} />}
      {today && <TodayTiles id="moderation-today" title="Moderation today" tiles={MODERATION_TILES} today={today} />}

      {data && (
        <p className="text-xs text-muted-foreground">
          {data.from} to {data.to} (UTC), {data.rows.length} {data.group === 'day' ? 'days' : `${data.group}s`} with numbers.
          {!admin && ' Moderators see the moderation numbers only.'}
        </p>
      )}
      {data && <ChartSection id="moderation-charts" title="Moderation" charts={MODERATION_CHARTS} data={data} />}
      {data && admin && <ChartSection id="usage-charts" title="Usage" charts={USAGE_CHARTS} data={data} />}
    </section>
  );
};

export default StatisticsPage;
