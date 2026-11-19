import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { contentApi, type ContentApi, type ContentReportSummary } from '@/shared/api/content';
import { formatDateTime } from '@/shared/lib/time';
import { Button } from '@/shared/ui/button';
import { CONTENT_REASON_LABELS, decisionText } from './model/labels';

type Load<T> = { state: 'loading' } | { state: 'error' } | { state: 'ready'; value: T };
type Tab = 'open' | 'decided';

export interface ContentReportsPageProps {
  api?: Pick<ContentApi, 'list'>;
  reportHref: (id: string) => string;
}

export function ContentReportsPage({ api = contentApi, reportHref }: ContentReportsPageProps) {
  const [tab, setTab] = useState<Tab>('open');
  const [load, setLoad] = useState<Load<ContentReportSummary[]>>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.list(tab).then(
      (value) => live && setLoad({ state: 'ready', value }),
      () => live && setLoad({ state: 'error' }),
    );
    return () => {
      live = false;
    };
  }, [api, tab, attempt]);

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">Content reports</h1>
      <p className="text-sm text-muted-foreground">Reported messages and groups, one row per reported item. Copies are deleted 6 months after the report.</p>
      <div role="tablist" aria-label="State" className="flex gap-2">
        {(['open', 'decided'] as const).map((value) => (
          <Button key={value} role="tab" aria-selected={tab === value} variant={tab === value ? 'default' : 'outline'} size="sm" onClick={() => setTab(value)}>
            {value === 'open' ? 'Open' : 'Decided'}
          </Button>
        ))}
      </div>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading reports…
        </p>
      )}
      {load.state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The reports could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}
      {load.state === 'ready' && load.value.length === 0 && (
        <p className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
          {tab === 'open' ? 'No open content reports.' : 'No decided content reports.'}
        </p>
      )}
      {load.state === 'ready' && load.value.length > 0 && (
        <ul aria-label="Content reports" className="flex flex-col gap-2">
          {load.value.map((r) => (
            <li key={r.id} className="flex flex-col gap-1 rounded-lg border bg-card p-3 text-sm">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <Link to={reportHref(r.id)} className="font-medium hover:underline">
                  {r.kind === 'group' ? 'Group' : 'Message'}: {CONTENT_REASON_LABELS[r.reason] ?? r.reason}
                </Link>
                <span className="text-xs text-muted-foreground">
                  {r.reports_on_item} {r.reports_on_item === 1 ? 'report' : 'reports'} · {formatDateTime(r.created_at)}
                </span>
              </div>
              <p className="truncate text-muted-foreground">
                {r.target ? `@${r.target.username}` : 'Deleted account'}
                {r.chat_title && ` in ${r.chat_title}`}
                {r.excerpt && `: ${r.excerpt}`}
              </p>
              {r.reason === 'illegal' && <p className="text-xs font-medium text-destructive">Notice of illegal content</p>}
              {r.decision && <p className="text-xs text-muted-foreground">{decisionText(r.decision)}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
