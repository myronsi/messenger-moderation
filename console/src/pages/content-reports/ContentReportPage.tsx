import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Check, Trash2 } from 'lucide-react';
import { ruleLabel } from '@/entities/moderation';
import { DecisionDialog } from '@/features/decision-dialog';
import { ApiError } from '@/shared/api/client';
import { contentApi, type ContentApi, type ContentReportDetail, type ContentReportSummary } from '@/shared/api/content';
import { formatDateTime } from '@/shared/lib/time';
import { Button } from '@/shared/ui/button';
import { CopyView } from './ui/CopyView';
import { DismissDialog } from './ui/DismissDialog';
import { decisionText, CONTENT_REASON_LABELS } from './model/labels';

type Load<T> = { state: 'loading' } | { state: 'missing' } | { state: 'error' } | { state: 'ready'; value: T };

export interface ContentReportPageProps {
  id: string;
  api?: Pick<ContentApi, 'get' | 'remove' | 'dismiss'>;
  accountHref: (id: string) => string;
  reportHref: (id: string) => string;
  listHref: string;
}

interface ReportLineProps {
  report: ContentReportSummary;
  accountHref: (id: string) => string;
  reportHref?: (id: string) => string;
}

function ReportLine({ report, accountHref, reportHref }: ReportLineProps) {
  return (
    <li className="flex flex-col gap-0.5">
      <span>
        {report.reporter ? (
          <Link to={accountHref(report.reporter.id)} className="hover:underline">
            @{report.reporter.username}
          </Link>
        ) : (
          'Deleted account'
        )}
        : {CONTENT_REASON_LABELS[report.reason] ?? report.reason}
        {reportHref && (
          <>
            {' · '}
            <Link to={reportHref(report.id)} className="text-xs hover:underline">
              open
            </Link>
          </>
        )}
      </span>
      {report.note && <span className="whitespace-pre-wrap break-words text-xs">{report.note}</span>}
      <span className="text-xs text-muted-foreground">{formatDateTime(report.created_at)}</span>
    </li>
  );
}

export function ContentReportPage({ id, api = contentApi, accountHref, reportHref, listHref }: ContentReportPageProps) {
  const [load, setLoad] = useState<Load<ContentReportDetail>>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const [acting, setActing] = useState<null | 'remove' | 'dismiss'>(null);

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.get(id).then(
      (value) => live && setLoad({ state: 'ready', value }),
      (error: unknown) => live && setLoad({ state: error instanceof ApiError && error.status === 404 ? 'missing' : 'error' }),
    );
    return () => {
      live = false;
    };
  }, [api, id, attempt]);

  const done = () => {
    setActing(null);
    setAttempt((n) => n + 1);
  };

  return (
    <div className="flex flex-col gap-4">
      <Link to={listHref} className="inline-flex items-center gap-1 self-start text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft aria-hidden className="size-4" />
        Back to content reports
      </Link>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading the report…
        </p>
      )}
      {load.state === 'missing' && (
        <p role="alert" className="rounded-md border border-dashed p-8 text-center text-sm">
          No report with ID {id}. Reports are deleted 6 months after they were sent.
        </p>
      )}
      {load.state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The report could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}
      {load.state === 'ready' && (
        <>
          <header className="flex flex-col gap-3 rounded-lg border bg-card p-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">
                {load.value.kind === 'group' ? 'Reported group' : 'Reported message'}
                {load.value.chat_title && ` · ${load.value.chat_title}`}
              </p>
              <h1 className="text-xl font-semibold">{CONTENT_REASON_LABELS[load.value.reason] ?? load.value.reason}</h1>
              {load.value.reason === 'illegal' && (
                <p className="mt-1 text-sm font-medium text-destructive">Notice of illegal content: decide quickly and record a reason.</p>
              )}
              <p className="text-sm text-muted-foreground">
                {load.value.target ? (
                  <>
                    {load.value.kind === 'group' ? 'Owner ' : 'Sent by '}
                    <Link to={accountHref(load.value.target.id)} className="hover:underline">
                      @{load.value.target.username}
                    </Link>
                    {' (account actions on the account page)'}
                  </>
                ) : (
                  'The account no longer exists'
                )}
              </p>
              <p className="mt-1 text-sm" data-testid="decision-state">
                {load.value.decision ? decisionText(load.value.decision) : 'Open'}
              </p>
            </div>
            <div className="flex flex-col gap-2 sm:items-end">
              {load.value.kind === 'message' && load.value.decision?.action !== 'removed' && (
                <Button variant="destructive" onClick={() => setActing('remove')}>
                  <Trash2 aria-hidden />
                  Remove message
                </Button>
              )}
              {!load.value.decision && (
                <Button variant="outline" onClick={() => setActing('dismiss')}>
                  <Check aria-hidden />
                  No violation
                </Button>
              )}
            </div>
          </header>
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
            <section aria-labelledby="copy-title" className="flex flex-col gap-2">
              <h2 id="copy-title" className="font-semibold">
                {load.value.kind === 'group' ? 'Group at the time of the report' : 'Messages as the reporter saw them'}
              </h2>
              <p className="text-xs text-muted-foreground">A copy taken when the report was sent. The chat itself is not available here.</p>
              <CopyView copy={load.value.copy} accountHref={accountHref} />
            </section>
            <aside className="flex flex-col gap-4">
              <section aria-labelledby="report-title" className="rounded-lg border bg-card p-4 text-sm">
                <h2 id="report-title" className="mb-2 font-semibold">
                  This report
                </h2>
                <ul>
                  <ReportLine report={load.value} accountHref={accountHref} />
                </ul>
              </section>
              {load.value.other_reports.length > 0 && (
                <section aria-labelledby="others-title" className="rounded-lg border bg-card p-4 text-sm">
                  <h2 id="others-title" className="mb-2 font-semibold">
                    Other reports on it ({load.value.other_reports.length})
                  </h2>
                  <ul className="flex flex-col gap-2">
                    {load.value.other_reports.map((r) => (
                      <ReportLine key={r.id} report={r} accountHref={accountHref} reportHref={reportHref} />
                    ))}
                  </ul>
                </section>
              )}
              {load.value.decision?.reason_code && (
                <section aria-labelledby="decision-title" className="rounded-lg border bg-card p-4 text-sm">
                  <h2 id="decision-title" className="mb-2 font-semibold">
                    Reason sent to the sender
                  </h2>
                  <p>{ruleLabel(load.value.decision.reason_code)}</p>
                  {load.value.decision.reason_text && <p className="mt-1 whitespace-pre-wrap text-muted-foreground">{load.value.decision.reason_text}</p>}
                </section>
              )}
            </aside>
          </div>
          {acting === 'remove' && (
            <DecisionDialog
              title="Remove the message for everyone"
              description="Chat members see Removed by moderation in its place. The sender also sees the rule and your text, signed as Messenger moderation."
              submitLabel="Remove message"
              destructive
              onSubmit={(reason) => api.remove(load.value.id, reason)}
              onDone={done}
              onCancel={() => setActing(null)}
            />
          )}
          {acting === 'dismiss' && <DismissDialog onSubmit={(note) => api.dismiss(load.value.id, note)} onDone={done} onCancel={() => setActing(null)} />}
        </>
      )}
    </div>
  );
}
