import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import StatisticsPage from './StatisticsPage';
import { formatPeriod, formatValue } from './format';
import { chartData, MODERATION_CHARTS, USAGE_CHARTS } from './metrics';

const now = () => new Date('2026-11-19T10:30:00Z');

const adminRange = {
  from: '2026-10-20', to: '2026-11-18', group: 'day', scope: 'all',
  rows: [
    { day: '2026-11-17', late: false, users_new: 3, users_active_day: 40, messages: 120, messages_text: 100, messages_file: 10, messages_voice: 6, messages_sticker: 4,
      reports_total: 2, reports: { spam: 2 }, pack_reports: {}, decisions: null },
    { day: '2026-11-18', late: true, users_new: 5, users_active_day: null, messages: 150, messages_text: 130, messages_file: 10, messages_voice: 5, messages_sticker: 5,
      reports_total: 3, reports: { spam: 1, scam: 1, unknown_reason: 1 }, pack_reports: { hate: 1 }, decisions: null },
  ],
};

const adminToday = {
  scope: 'all', as_of: '2026-11-19T10:30:00Z',
  today: { day: '2026-11-19', late: false, users_new: 2, users_active_day: 31, messages: 48, files_sent: 3, calls: 1, call_minutes: 7, reports_total: 1, pack_reports_total: 0 },
  yesterday: { day: '2026-11-18', late: false, users_new: 5, users_active_day: 44, messages: 150, files_sent: 9, calls: 4, call_minutes: 61, reports_total: 3, pack_reports_total: 1 },
};

const moderatorRange = { ...adminRange, scope: 'moderation', rows: adminRange.rows.map(({ day, late, reports_total, reports, pack_reports, decisions }) => ({ day, late, reports_total, reports, pack_reports, decisions })) };

const moderatorToday = {
  scope: 'moderation', as_of: '2026-11-19T10:30:00Z',
  today: { day: '2026-11-19', late: false, reports_total: 1, pack_reports_total: 0 },
  yesterday: { day: '2026-11-18', late: false, reports_total: 3, pack_reports_total: 1 },
};

const respond = (stats: unknown, today: unknown, todayStatus = 200) => {
  const fetchMock = vi.fn((url: string) => {
    if (url.startsWith('/api/stats/today')) return Promise.resolve(new Response(JSON.stringify(today), { status: todayStatus }));
    return Promise.resolve(new Response(JSON.stringify(stats), { status: 200 }));
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
};

describe('StatisticsPage', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('shows usage and moderation numbers for admins, today next to yesterday', async () => {
    const fetchMock = respond(adminRange, adminToday);
    render(<StatisticsPage now={now} />);
    const usage = await screen.findByRole('region', { name: 'Usage today' });
    const messages = within(usage).getByText('Messages sent').parentElement!;
    expect(messages).toHaveTextContent('48');
    expect(messages).toHaveTextContent('Yesterday 150');
    expect(within(usage).getByText('Call minutes').parentElement).toHaveTextContent('7 min');
    expect(screen.getByRole('heading', { name: 'Messages sent' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Account reports by reason' })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith('/api/stats?from=2026-10-20&to=2026-11-18&group=day', expect.anything());
    expect(screen.getByRole('link', { name: /Download CSV/ })).toHaveAttribute('href', '/api/stats?from=2026-10-20&to=2026-11-18&group=day&format=csv');
  });

  it('asks again when the period or grouping changes', async () => {
    const fetchMock = respond(adminRange, adminToday);
    render(<StatisticsPage now={now} />);
    await screen.findByRole('region', { name: 'Usage today' });
    await userEvent.click(screen.getByRole('radio', { name: '1 year' }));
    await userEvent.click(screen.getByRole('radio', { name: 'Month' }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/stats?from=2025-11-19&to=2026-11-18&group=month', expect.anything()));
    expect(screen.getByRole('link', { name: /Download CSV/ })).toHaveAttribute('href', expect.stringContaining('group=month&format=csv'));
  });

  it('shows moderators the moderation block only', async () => {
    respond(moderatorRange, moderatorToday);
    render(<StatisticsPage now={now} />);
    const moderation = await screen.findByRole('region', { name: 'Moderation today' });
    expect(within(moderation).getByText('Account reports').parentElement).toHaveTextContent('Yesterday 3');
    expect(screen.queryByRole('region', { name: 'Usage today' })).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Usage' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Messages sent' })).not.toBeInTheDocument();
    expect(screen.getByText(/Moderators see the moderation numbers only/)).toBeInTheDocument();
  });

  it('explains missing numbers', async () => {
    respond(adminRange, { error: 'unavailable' }, 503);
    render(<StatisticsPage now={now} />);
    expect(await screen.findByText(/the Messenger database is not connected/)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Decisions by type' }).closest('figure')).toHaveTextContent('No data for this period yet.');
    const table = screen.getAllByRole('table')[0];
    expect(within(table).getByRole('rowheader', { name: 'Nov 18 *' })).toBeInTheDocument();
    expect(screen.getAllByText(/Computed late/)).toHaveLength(2);
  });

  it('reports a failed load', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('{}', { status: 500 }))));
    render(<StatisticsPage now={now} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Statistics could not be loaded.');
  });
});

describe('chart data', () => {
  it('keeps colours with their reason and folds unknown reasons into Other', () => {
    const reports = MODERATION_CHARTS.find((c) => c.id === 'reports')!;
    const { series, points } = chartData(reports, adminRange.rows as never);
    expect(series.map((s) => [s.key, s.color])).toEqual([
      ['spam', 'var(--series-1)'], ['scam', 'var(--series-2)'], ['other', 'var(--series-7)'],
    ]);
    expect(points[1]).toMatchObject({ day: '2026-11-18', spam: 1, scam: 1, other: 1, harassment: 0 });
    const decisions = MODERATION_CHARTS.find((c) => c.id === 'decisions')!;
    expect(chartData(decisions, adminRange.rows as never).series).toEqual([]);
    const messages = USAGE_CHARTS.find((c) => c.id === 'messages')!;
    expect(chartData(messages, adminRange.rows as never).points[0]).toMatchObject({ messages_text: 100, messages_sticker: 4 });
  });
});

describe('format', () => {
  it('formats values and periods', () => {
    expect(formatValue(1536, 'bytes')).toBe('1.5 KB');
    expect(formatValue(5 * 1024 ** 3, 'bytes')).toBe('5.0 GB');
    expect(formatValue(5400, 'duration')).toBe('1 h 30 min');
    expect(formatValue(90, 'duration')).toBe('2 min');
    expect(formatValue(12345)).toBe('12,345');
    expect(formatValue(null)).toBe('-');
    expect(formatPeriod('2026-11-16', 'week')).toBe('Week of Nov 16');
    expect(formatPeriod('2026-11-01', 'month')).toBe('Nov 2026');
  });
});
