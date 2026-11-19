import { Link } from 'react-router-dom';
import { ArrowRight, Flag, ShieldAlert } from 'lucide-react';
import { causeLabel, CONTEXT_LABELS, reasonLabel, ruleLabel, StatusBadge } from '@/entities/moderation';
import { type AccountProfile, type StatusChange, type UserReport } from '@/shared/api/review';
import { cn } from '@/shared/lib/utils';
import { formatAge, formatDateTime } from '@/shared/lib/time';
import { snapshotDiff, type TimelineEntry } from '../model/timeline';

interface TimelineProps {
  entries: TimelineEntry[];
  current: AccountProfile;
  accountHref: (id: string) => string;
}

export function Timeline({ entries, current, accountHref }: TimelineProps) {
  if (entries.length === 0) {
    return <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">No reports or status changes.</p>;
  }
  return (
    <ol aria-label="Timeline" className="flex flex-col gap-3">
      {entries.map((entry) => (
        <li key={entry.key}>
          {entry.kind === 'status' ? (
            <StatusEntry change={entry.change} />
          ) : (
            <ReportEntry report={entry.report} current={current} accountHref={accountHref} />
          )}
        </li>
      ))}
    </ol>
  );
}

function StatusEntry({ change }: { change: StatusChange }) {
  return (
    <article aria-label="Status change" className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-sm">
      <ShieldAlert aria-hidden className="size-4 text-muted-foreground" />
      <StatusBadge status={change.from_status} />
      <ArrowRight aria-label="to" className="size-4 text-muted-foreground" />
      <StatusBadge status={change.to_status} />
      <span>{causeLabel(change.cause)}</span>
      {change.reason_code && <span className="text-muted-foreground">· {ruleLabel(change.reason_code)}</span>}
      <span className="text-muted-foreground">score {change.score.toLocaleString('en-GB', { maximumFractionDigits: 2 })}</span>
      <time dateTime={change.created_at} className="ml-auto text-xs text-muted-foreground">
        {formatDateTime(change.created_at)}
      </time>
      {change.reason_text && <p className="w-full whitespace-pre-line text-muted-foreground">{change.reason_text}</p>}
    </article>
  );
}

interface ReportEntryProps {
  report: UserReport;
  current: AccountProfile;
  accountHref: (id: string) => string;
}

function ReportEntry({ report, current, accountHref }: ReportEntryProps) {
  const diff = snapshotDiff(report, current);
  const withdrawn = report.withdrawn_at !== null;
  const dismissed = !!report.dismissed_at;
  return (
    <article aria-label={`Report: ${reasonLabel(report.reason)}`} className={cn('rounded-md border bg-card p-3 text-sm', (withdrawn || dismissed) && 'opacity-70')}>
      <div className="flex flex-wrap items-center gap-2">
        <Flag aria-hidden className="size-4 text-destructive" />
        <span className="font-medium">{reasonLabel(report.reason)}</span>
        <span className="text-muted-foreground">by</span>
        <Link to={accountHref(report.reporter.id)} className="text-primary hover:underline">
          @{report.reporter.username}
        </Link>
        {report.reporter.status !== 'good' && <StatusBadge status={report.reporter.status} />}
        {withdrawn && <span className="rounded bg-muted px-1.5 text-xs">Withdrawn</span>}
        {dismissed && <span className="rounded bg-muted px-1.5 text-xs">Dismissed</span>}
        <time dateTime={report.created_at} className="ml-auto text-xs text-muted-foreground">
          {formatDateTime(report.created_at)}
        </time>
      </div>

      <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
        <dt className="text-muted-foreground">Weight</dt>
        <dd className="tabular-nums">{report.weight === 0 ? '0 (does not count)' : report.weight}</dd>
        <dt className="text-muted-foreground">Context</dt>
        <dd>{CONTEXT_LABELS[report.context] ?? report.context}</dd>
        <dt className="text-muted-foreground">Reporter account</dt>
        <dd>{formatAge(report.reporter.created_at, new Date(report.created_at))} old when reporting</dd>
      </dl>

      {report.note && <blockquote className="mt-2 border-l-2 pl-3 italic">{report.note}</blockquote>}

      <div className="mt-3 rounded bg-muted/60 p-2">
        <p className="mb-1 text-xs font-medium text-muted-foreground">Profile when reported</p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
          <dt className="text-muted-foreground">Name</dt>
          <dd>
            {report.snapshot.display_name ?? '—'}
            {diff.displayName && <Changed />}
          </dd>
          <dt className="text-muted-foreground">Username</dt>
          <dd>
            {report.snapshot.username ? `@${report.snapshot.username}` : '—'}
            {diff.username && <Changed />}
          </dd>
          <dt className="text-muted-foreground">Bio</dt>
          <dd className="whitespace-pre-line">
            {report.snapshot.bio || '—'}
            {diff.bio && <Changed />}
          </dd>
        </dl>
      </div>
    </article>
  );
}

function Changed() {
  return <span className="ml-2 rounded bg-warning-soft px-1 text-[0.7rem] font-medium text-warning">changed since</span>;
}
