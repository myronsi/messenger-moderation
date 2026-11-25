import type { StatsRow } from '@/shared/api/client';
import type { ValueFormat } from './format';

export interface TileDef {
  key: string;
  label: string;
  format?: ValueFormat;
}

export interface SeriesDef {
  key: string;
  label: string;
}

export interface ChartDef {
  id: string;
  title: string;
  description: string;
  kind: 'bar' | 'line';
  format?: ValueFormat;
  series?: SeriesDef[];
  breakdown?: { field: string; order: SeriesDef[] };
}

export const USAGE_TILES: TileDef[] = [
  { key: 'users_new', label: 'New accounts' },
  { key: 'users_active_day', label: 'Active accounts' },
  { key: 'messages', label: 'Messages sent' },
  { key: 'files_sent', label: 'Files sent' },
  { key: 'calls', label: 'Calls' },
  { key: 'call_minutes', label: 'Call minutes', format: 'minutes' },
];

export const MODERATION_TILES: TileDef[] = [
  { key: 'reports_total', label: 'Account reports' },
  { key: 'pack_reports_total', label: 'Pack reports' },
  { key: 'decisions_total', label: 'Decisions' },
  { key: 'reversals', label: 'Reversed automatic levels' },
  { key: 'median_decision_seconds', label: 'Median time to decision', format: 'duration' },
];

const REPORT_REASONS: SeriesDef[] = [
  { key: 'spam', label: 'Spam' },
  { key: 'scam', label: 'Scam' },
  { key: 'harassment', label: 'Harassment' },
  { key: 'impersonation', label: 'Impersonation' },
  { key: 'inappropriate_profile', label: 'Inappropriate profile' },
  { key: 'illegal', label: 'Illegal content' },
  { key: 'other', label: 'Other' },
];

const PACK_REASONS: SeriesDef[] = [
  { key: 'spam', label: 'Spam' },
  { key: 'violence', label: 'Violence' },
  { key: 'sexual', label: 'Sexual' },
  { key: 'hate', label: 'Hate' },
  { key: 'copyright', label: 'Copyright' },
  { key: 'other', label: 'Other' },
];

const DECISIONS: SeriesDef[] = [
  { key: 'warned', label: 'Warned' },
  { key: 'limited', label: 'Limited' },
  { key: 'restricted', label: 'Restricted' },
  { key: 'suspended', label: 'Suspended' },
  { key: 'dismissed', label: 'Reports dismissed' },
  { key: 'content_removed', label: 'Content removed' },
  { key: 'other', label: 'Other' },
];

export const USAGE_CHARTS: ChartDef[] = [
  { id: 'active', title: 'Active accounts', description: 'Accounts that used the app that day; for weeks and months the daily average.', kind: 'line', series: [{ key: 'users_active_day', label: 'Active accounts' }] },
  { id: 'new-users', title: 'New accounts', description: 'Accounts created.', kind: 'bar', series: [{ key: 'users_new', label: 'New accounts' }] },
  {
    id: 'messages', title: 'Messages sent', description: 'By kind of message, forwards included.', kind: 'bar',
    series: [
      { key: 'messages_text', label: 'Text' },
      { key: 'messages_file', label: 'Files' },
      { key: 'messages_voice', label: 'Voice' },
      { key: 'messages_sticker', label: 'Stickers' },
      { key: 'messages_poll', label: 'Polls' },
      { key: 'messages_contact', label: 'Contacts' },
      { key: 'messages_location', label: 'Locations' },
    ],
  },
  {
    id: 'chats', title: 'New chats', description: 'Direct chats and groups created.', kind: 'bar',
    series: [{ key: 'direct_chats_new', label: 'Direct chats' }, { key: 'groups_new', label: 'Groups' }],
  },
  { id: 'call-minutes', title: 'Call minutes', description: 'Length of the calls that started in the period.', kind: 'bar', format: 'minutes', series: [{ key: 'call_minutes', label: 'Call minutes' }] },
  { id: 'storage', title: 'Stored files', description: 'Space used by uploaded files and their converted versions, at the end of the period.', kind: 'line', format: 'bytes', series: [{ key: 'storage_bytes', label: 'Stored files' }] },
];

export const MODERATION_CHARTS: ChartDef[] = [
  { id: 'reports', title: 'Account reports by reason', description: 'Reports received against accounts.', kind: 'bar', breakdown: { field: 'reports', order: REPORT_REASONS } },
  { id: 'pack-reports', title: 'Pack reports by reason', description: 'Reports received against sticker and emoji packs.', kind: 'bar', breakdown: { field: 'pack_reports', order: PACK_REASONS } },
  { id: 'decisions', title: 'Decisions by type', description: 'Decisions made by staff.', kind: 'bar', breakdown: { field: 'decisions', order: DECISIONS } },
  {
    id: 'median', title: 'Median time to decision', description: 'From case opened to decision; for weeks and months the average of the daily medians.',
    kind: 'line', format: 'duration', series: [{ key: 'median_decision_seconds', label: 'Median time to decision' }],
  },
];

export const SERIES_COLORS = ['var(--series-1)', 'var(--series-2)', 'var(--series-3)', 'var(--series-4)', 'var(--series-5)', 'var(--series-6)', 'var(--series-7)'];

export const numberOf = (row: StatsRow | null | undefined, key: string): number | null => {
  const value = row?.[key];
  return typeof value === 'number' ? value : null;
};

export interface ChartSeries extends SeriesDef {
  color: string;
}

export interface ChartData {
  series: ChartSeries[];
  points: Array<Record<string, string | number | null>>;
}

export const chartData = (chart: ChartDef, rows: StatsRow[]): ChartData => {
  if (chart.series) {
    return {
      series: chart.series.map((s, i) => ({ ...s, color: SERIES_COLORS[i] })),
      points: rows.map((row) => Object.fromEntries([['day', row.day], ...chart.series!.map((s) => [s.key, numberOf(row, s.key)])])),
    };
  }
  const { field, order } = chart.breakdown!;
  const known = new Set(order.map((s) => s.key));
  const points = rows.map((row) => {
    const counts = (row[field] ?? null) as Record<string, number> | null;
    const point: Record<string, string | number | null> = { day: row.day };
    for (const s of order) point[s.key] = counts ? 0 : null;
    if (counts) {
      for (const [key, n] of Object.entries(counts)) {
        const target = known.has(key) ? key : 'other';
        point[target] = (point[target] as number) + n;
      }
    }
    return point;
  });
  const used = order
    .map((s, i) => ({ ...s, color: SERIES_COLORS[i] }))
    .filter((s) => points.some((p) => typeof p[s.key] === 'number' && (p[s.key] as number) > 0));
  return { series: used, points };
};
