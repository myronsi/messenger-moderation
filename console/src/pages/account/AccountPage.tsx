import { Link, useNavigate } from 'react-router-dom';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { ArrowLeft } from 'lucide-react';
import { reasonLabel } from '@/entities/moderation';
import { reviewApi, type AccountDetail, type ReviewApi } from '@/shared/api/review';
import type { DecisionsApi } from '@/shared/api/decisions';
import { ApiError } from '@/shared/api/client';
import { Button } from '@/shared/ui/button';
import { buildTimeline } from './model/timeline';
import { AccountActions } from './ui/AccountActions';
import { AccountHeader } from './ui/AccountHeader';
import { AppealSummary } from './ui/AppealSummary';
import { Timeline } from './ui/Timeline';

type Load = { state: 'loading' } | { state: 'missing' } | { state: 'error' } | { state: 'ready'; detail: AccountDetail };

export interface AccountPageProps {
  api?: Pick<ReviewApi, 'account'>;
  userId: string;
  accountHref: (id: string) => string;
  queueHref: string;
  now?: () => Date;
  isAdmin?: boolean;
  decisions?: Pick<DecisionsApi, 'setStatus' | 'dismissReports' | 'deleteAccount'>;
}

export default function AccountPage({ api = reviewApi, userId, accountHref, queueHref, now = () => new Date(), isAdmin = false, decisions }: AccountPageProps) {
  const [load, setLoad] = useState<Load>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const navigate = useNavigate();

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.account(userId).then(
      (detail) => {
        if (live) setLoad({ state: 'ready', detail });
      },
      (error: unknown) => {
        if (live) setLoad({ state: error instanceof ApiError && error.status === 404 ? 'missing' : 'error' });
      },
    );
    return () => {
      live = false;
    };
  }, [api, userId, attempt]);

  return (
    <div className="flex flex-col gap-4">
      <Link to={queueHref} className="inline-flex items-center gap-1 self-start text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft aria-hidden className="size-4" />
        Back to the queue
      </Link>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading the account…
        </p>
      )}
      {load.state === 'missing' && (
        <p role="alert" className="rounded-md border border-dashed p-8 text-center text-sm">
          No account with ID {userId}. It may have been deleted.
        </p>
      )}
      {load.state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The account could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}
      {load.state === 'ready' && (
        <AccountView
          detail={load.detail}
          accountHref={accountHref}
          now={now()}
          actions={
            <AccountActions
              detail={load.detail}
              isAdmin={isAdmin}
              api={decisions}
              onChanged={() => setAttempt((n) => n + 1)}
              onDeleted={() => navigate(queueHref)}
            />
          }
        />
      )}
    </div>
  );
}

function AccountView({ detail, accountHref, now, actions }: { detail: AccountDetail; accountHref: (id: string) => string; now: Date; actions: ReactNode }) {
  const entries = useMemo(() => buildTimeline(detail), [detail]);
  const active = detail.reports.filter((report) => report.withdrawn_at === null && !report.dismissed_at);
  const dismissed = detail.reports.filter((report) => report.withdrawn_at === null && report.dismissed_at).length;
  const byReason = Object.entries(
    active.reduce<Record<string, number>>((counts, report) => ({ ...counts, [report.reason]: (counts[report.reason] ?? 0) + 1 }), {}),
  ).sort((a, b) => b[1] - a[1]);

  return (
    <>
      <AccountHeader detail={detail} now={now} />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <section aria-labelledby="timeline-title" className="flex min-w-0 flex-col gap-2">
          <h2 id="timeline-title" className="font-semibold">
            Timeline
          </h2>
          <Timeline entries={entries} current={detail.user} accountHref={accountHref} />
        </section>
        <aside className="flex flex-col gap-4">
          {actions}
          <section aria-labelledby="reports-title" className="rounded-lg border bg-card p-4 text-sm">
            <h2 id="reports-title" className="mb-2 font-semibold">
              Reports
            </h2>
            <p>
              {active.length} active, {detail.reports.length - active.length - dismissed} withdrawn
              {dismissed > 0 && `, ${dismissed} dismissed`}
            </p>
            {byReason.length > 0 && (
              <ul className="mt-2 flex flex-col gap-0.5 text-muted-foreground">
                {byReason.map(([reason, count]) => (
                  <li key={reason}>
                    {reasonLabel(reason)}: {count}
                  </li>
                ))}
              </ul>
            )}
          </section>
          <AppealSummary detail={detail} now={now} />
        </aside>
      </div>
    </>
  );
}
