import { APPEAL_CAUSES, causeLabel } from '@/entities/moderation';
import { type AccountDetail } from '@/shared/api/review';
import { formatDateTime } from '@/shared/lib/time';

export function AppealSummary({ detail, now }: { detail: AccountDetail; now: Date }) {
  const { status } = detail;
  const appeals = detail.history.filter((change) => APPEAL_CAUSES.has(change.cause));
  const onProbation = status.probation_until !== null && new Date(status.probation_until) > now;
  const blocked = status.appeal_blocked_until !== null && new Date(status.appeal_blocked_until) > now;

  return (
    <section aria-labelledby="appeals-title" className="rounded-lg border bg-card p-4 text-sm">
      <h2 id="appeals-title" className="mb-2 font-semibold">
        Appeals
      </h2>
      {appeals.length === 0 && !onProbation && !blocked ? (
        <p className="text-muted-foreground">No appeals.</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {onProbation && <li>On probation until {formatDateTime(status.probation_until!)}</li>}
          {blocked && <li>Appeals blocked until {formatDateTime(status.appeal_blocked_until!)}</li>}
          {appeals.map((change) => (
            <li key={change.id} className="text-muted-foreground">
              {causeLabel(change.cause)} · {formatDateTime(change.created_at)}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
