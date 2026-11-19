import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Clock3, Loader2, RefreshCw } from 'lucide-react';
import { fetchStatus, type CheckState, type StatusOverview } from '@/shared/api/client';
import { cn } from '@/shared/lib/utils';
import { Button } from '@/shared/ui/button';
import { formatDuration, formatTime, formatUptime } from './format';

const STATE_STYLE: Record<CheckState, { label: string; className: string }> = {
  ok: { label: 'Working', className: 'bg-success-soft text-success' },
  slow: { label: 'Slow', className: 'bg-warning-soft text-warning' },
  failed: { label: 'Down', className: 'bg-destructive-soft text-destructive' },
};

const REFRESH_MS = 30_000;

const StatusPage = () => {
  const [data, setData] = useState<StatusOverview | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setData(await fetchStatus());
      setError('');
    } catch {
      setError('Status could not be loaded.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  const failing = data?.components.filter((c) => c.state === 'failed').length ?? 0;

  return (
    <section aria-labelledby="status-title" className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <h1 id="status-title" className="text-2xl font-semibold tracking-tight text-foreground">Status</h1>
        <div className="flex-1" />
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
          {loading ? <Loader2 aria-hidden="true" className="animate-spin" /> : <RefreshCw aria-hidden="true" />}
          Refresh
        </Button>
      </div>

      {error && <p role="alert" className="rounded-md bg-destructive-soft px-3 py-2 text-sm text-destructive">{error}</p>}

      {data && (
        <div role="status" className={cn('flex items-center gap-3 rounded-lg border px-4 py-3',
          failing ? 'border-destructive/30 bg-destructive-soft' : 'border-success/30 bg-success-soft')}>
          {failing
            ? <AlertTriangle aria-hidden="true" className="h-5 w-5 text-destructive" />
            : <CheckCircle2 aria-hidden="true" className="h-5 w-5 text-success" />}
          <p className="text-sm font-medium text-foreground">
            {failing ? `${failing} ${failing === 1 ? 'component is' : 'components are'} down` : 'All components are working'}
          </p>
        </div>
      )}

      {data && (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/60 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">Component</th>
                <th scope="col" className="px-4 py-2 font-medium">State</th>
                <th scope="col" className="px-4 py-2 font-medium">Response</th>
                <th scope="col" className="px-4 py-2 font-medium">Uptime 30 days</th>
                <th scope="col" className="px-4 py-2 font-medium">Last check</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {data.components.map((c) => (
                <tr key={c.component}>
                  <th scope="row" className="px-4 py-2 font-medium text-foreground">{c.component}</th>
                  <td className="px-4 py-2">
                    <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', STATE_STYLE[c.state].className)}>{STATE_STYLE[c.state].label}</span>
                    {c.detail && <span className="ml-2 text-xs text-muted-foreground">{c.detail}</span>}
                  </td>
                  <td className="px-4 py-2 tabular-nums text-subtle-foreground">{c.latency_ms} ms</td>
                  <td className="px-4 py-2 tabular-nums text-subtle-foreground">{formatUptime(c.uptime_30d)}</td>
                  <td className="px-4 py-2 text-subtle-foreground">{formatTime(c.checked_at)}</td>
                </tr>
              ))}
              {data.components.length === 0 && (
                <tr><td colSpan={5} className="px-4 py-6 text-center text-muted-foreground">No checks yet. The first results appear within a minute.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {data && (
        <section aria-labelledby="incidents-title" className="space-y-3">
          <h2 id="incidents-title" className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Incidents</h2>
          {data.recent_incidents.length === 0 ? (
            <p className="text-sm text-muted-foreground">No incidents recorded.</p>
          ) : (
            <ul className="divide-y divide-border rounded-lg border border-border bg-card">
              {data.recent_incidents.map((incident) => (
                <li key={incident.id} className="flex flex-wrap items-start gap-x-4 gap-y-1 px-4 py-3 text-sm">
                  <Clock3 aria-hidden="true" className="mt-0.5 h-4 w-4 text-muted-foreground" />
                  <span className="font-medium text-foreground">
                    {incident.kind === 'monitoring_gap' ? 'Monitoring interrupted' : `${incident.component} down`}
                  </span>
                  <span className="text-subtle-foreground">
                    {formatTime(incident.started_at)} · {incident.ended_at
                      ? formatDuration(incident.started_at, incident.ended_at)
                      : `ongoing for ${formatDuration(incident.started_at)}`}
                  </span>
                  {!incident.ended_at && <span className="rounded-full bg-destructive-soft px-2 py-0.5 text-xs font-medium text-destructive">Open</span>}
                  {incident.detail && <span className="basis-full pl-8 text-xs text-muted-foreground">{incident.detail}</span>}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </section>
  );
};

export default StatusPage;
