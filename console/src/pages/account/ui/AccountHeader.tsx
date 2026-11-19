import { StatusBadge } from '@/entities/moderation';
import { type AccountDetail } from '@/shared/api/review';
import { formatAge, formatDate, formatDateTime } from '@/shared/lib/time';

interface AccountHeaderProps {
  detail: AccountDetail;
  now: Date;
}

export function AccountHeader({ detail, now }: AccountHeaderProps) {
  const { user, status } = detail;
  return (
    <header className="flex flex-col gap-3 rounded-lg border bg-card p-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="truncate text-xl font-semibold">{user.display_name}</h1>
          <StatusBadge status={status.status} />
        </div>
        <p className="text-sm text-muted-foreground">
          @{user.username} · ID {user.id}
        </p>
        {user.bio && <p className="mt-2 whitespace-pre-line text-sm">{user.bio}</p>}
      </div>
      <dl className="grid shrink-0 grid-cols-2 gap-x-6 gap-y-1 text-sm sm:text-right">
        <dt className="text-muted-foreground">Score</dt>
        <dd className="tabular-nums">{status.score.toLocaleString('en-GB', { maximumFractionDigits: 2 })}</dd>
        <dt className="text-muted-foreground">Status since</dt>
        <dd>{status.since ? formatDateTime(status.since) : 'Never changed'}</dd>
        <dt className="text-muted-foreground">Account age</dt>
        <dd title={formatDate(user.created_at)}>{formatAge(user.created_at, now)}</dd>
        <dt className="text-muted-foreground">Last seen</dt>
        <dd>{formatDateTime(user.last_seen_at)}</dd>
      </dl>
    </header>
  );
}
