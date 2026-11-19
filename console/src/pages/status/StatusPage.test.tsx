import { render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import StatusPage from './StatusPage';
import { formatDuration, formatUptime } from './format';

const overview = {
  components: [
    { component: 'api', state: 'ok', latency_ms: 12, checked_at: '2026-11-19T12:00:00Z', uptime_30d: 0.9999 },
    { component: 'scylla', state: 'failed', latency_ms: 5000, detail: 'unavailable', checked_at: '2026-11-19T12:00:00Z', uptime_30d: 0.98 },
  ],
  open_incidents: [{ id: 2, component: 'scylla', kind: 'failure', started_at: '2026-11-19T11:30:00Z', detail: 'unavailable' }],
  recent_incidents: [
    { id: 2, component: 'scylla', kind: 'failure', started_at: '2026-11-19T11:30:00Z', detail: 'unavailable' },
    { id: 1, component: 'monitoring', kind: 'monitoring_gap', started_at: '2026-11-18T08:00:00Z', ended_at: '2026-11-18T09:30:00Z' },
  ],
};

describe('StatusPage', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('shows components, the failing count and incidents', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify(overview), { status: 200 }))));
    render(<StatusPage />);
    expect(await screen.findByText('1 component is down')).toBeInTheDocument();
    const scylla = screen.getByRole('row', { name: /scylla/ });
    expect(within(scylla).getByText('Down')).toBeInTheDocument();
    expect(within(scylla).getByText('98.00 %')).toBeInTheDocument();
    expect(screen.getByText('Monitoring interrupted')).toBeInTheDocument();
    expect(screen.getByText('Open')).toBeInTheDocument();
  });

  it('explains when the status cannot be loaded', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('{}', { status: 500 }))));
    render(<StatusPage />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Status could not be loaded.');
  });
});

describe('format', () => {
  it('formats durations and uptime', () => {
    expect(formatDuration('2026-11-19T10:00:00Z', '2026-11-19T10:45:00Z')).toBe('45 min');
    expect(formatDuration('2026-11-19T10:00:00Z', '2026-11-19T13:05:00Z')).toBe('3 h 5 min');
    expect(formatDuration('2026-11-17T10:00:00Z', '2026-11-19T13:00:00Z')).toBe('2 d 3 h');
    expect(formatUptime(1)).toBe('100.0 %');
    expect(formatUptime(0.98765)).toBe('98.77 %');
    expect(formatUptime(undefined)).toBe('-');
  });
});
