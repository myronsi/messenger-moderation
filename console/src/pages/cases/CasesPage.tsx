import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight } from 'lucide-react';
import { ruleLabel, STATUS_LABELS, StatusBadge } from '@/entities/moderation';
import { DecisionDialog } from '@/features/decision-dialog';
import { decisionsApi, type DecisionsApi, type ModerationCase } from '@/shared/api/decisions';
import { ACCOUNT_STATUSES, type AccountStatus } from '@/shared/api/review';
import { formatDateTime } from '@/shared/lib/time';
import { cn } from '@/shared/lib/utils';
import { Button } from '@/shared/ui/button';
import { Select } from '@/shared/ui/select';
import { deadlineText } from './model/deadline';

type Tab = 'open' | 'decided';
type Load = { state: 'loading' } | { state: 'error' } | { state: 'ready'; cases: ModerationCase[] };
type Acting = null | { kind: 'confirm' | 'reverse'; item: ModerationCase };

const LEVEL: Record<AccountStatus, number> = { good: 0, warned: 1, limited: 2, restricted: 3, suspended: 4 };

const STATE_LABELS: Record<ModerationCase['state'], string> = {
  open: 'Open',
  confirmed: 'Confirmed',
  reversed: 'Reversed',
  superseded: 'Replaced by a newer case',
};

export interface CasesPageProps {
  api?: Pick<DecisionsApi, 'cases' | 'confirmCase' | 'reverseCase'>;
  accountHref: (id: string) => string;
  packHref: (kind: string, id: string) => string;
  now?: () => Date;
}

export default function CasesPage({ api = decisionsApi, accountHref, packHref, now = () => new Date() }: CasesPageProps) {
  const [tab, setTab] = useState<Tab>('open');
  const [load, setLoad] = useState<Load>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const [acting, setActing] = useState<Acting>(null);
  const [lower, setLower] = useState<AccountStatus>('good');

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.cases(tab).then(
      (cases) => live && setLoad({ state: 'ready', cases }),
      () => live && setLoad({ state: 'error' }),
    );
    return () => {
      live = false;
    };
  }, [api, tab, attempt]);

  const at = now();

  const subject = (item: ModerationCase) => {
    if (item.kind === 'account_status' && item.user_id) {
      const name = item.subject?.display_name ?? `Account ${item.user_id}`;
      return (
        <Link to={accountHref(item.user_id)} className="font-medium hover:underline">
          {name}
          {item.subject?.username && <span className="ml-1 text-muted-foreground">@{item.subject.username}</span>}
        </Link>
      );
    }
    const kindLabel = item.pack_kind === 'emoji_pack' ? 'Emoji pack' : 'Sticker pack';
    return (
      <Link to={packHref(item.pack_kind ?? 'sticker_pack', item.pack_id ?? '')} className="font-medium hover:underline">
        {kindLabel}: {item.subject?.title ?? item.pack_id}
      </Link>
    );
  };

  const what = (item: ModerationCase) =>
    item.kind === 'account_status' && item.to_status ? (
      <span className="inline-flex flex-wrap items-center gap-1">
        {item.from_status && <StatusBadge status={item.from_status} />}
        <ArrowRight aria-label="to" className="size-4 text-muted-foreground" />
        <StatusBadge status={item.to_status} />
        <span className="text-muted-foreground">automatically</span>
      </span>
    ) : (
      <span>Hidden automatically after reports</span>
    );

  const lowerOptions = (item: ModerationCase) =>
    ACCOUNT_STATUSES.filter((s) => LEVEL[s] < LEVEL[item.to_status ?? 'suspended']).map((value) => ({ value, label: STATUS_LABELS[value] }));

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-semibold">Cases</h1>
        <div role="tablist" aria-label="Case state" className="flex gap-1 rounded-md border p-1">
          {(['open', 'decided'] as const).map((value) => (
            <button
              key={value}
              role="tab"
              type="button"
              aria-selected={tab === value}
              onClick={() => setTab(value)}
              className={cn('rounded px-3 py-1 text-sm', tab === value ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent')}
            >
              {value === 'open' ? 'Open' : 'Decided'}
            </button>
          ))}
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        Automatic restricted and suspended levels and packs hidden after reports apply at once. A person confirms or reverses each within 72 hours.
      </p>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading cases…
        </p>
      )}
      {load.state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The cases could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}
      {load.state === 'ready' && load.cases.length === 0 && (
        <p className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">{tab === 'open' ? 'No open cases.' : 'No decided cases yet.'}</p>
      )}
      {load.state === 'ready' && load.cases.length > 0 && (
        <ul aria-label="Cases" className="flex flex-col gap-2">
          {load.cases.map((item) => {
            const due = deadlineText(item.deadline, at);
            return (
              <li key={item.id} className="flex flex-col gap-2 rounded-lg border bg-card p-3 text-sm md:flex-row md:items-center md:justify-between">
                <div className="flex min-w-0 flex-col gap-1">
                  {subject(item)}
                  {what(item)}
                  <span className="text-xs text-muted-foreground">Opened {formatDateTime(item.opened_at)}</span>
                  {item.state !== 'open' && (
                    <span className="text-xs">
                      {STATE_LABELS[item.state]}
                      {item.decided_by && ` by ${item.decided_by}`}
                      {item.decided_at && ` on ${formatDateTime(item.decided_at)}`}
                      {item.reason_code && ` · ${ruleLabel(item.reason_code)}`}
                    </span>
                  )}
                </div>
                {item.state === 'open' && (
                  <div className="flex shrink-0 flex-wrap items-center gap-2">
                    <span
                      data-testid="deadline"
                      className={cn('rounded-full px-2 py-0.5 text-xs font-medium', due.overdue ? 'bg-destructive text-destructive-foreground' : due.soon ? 'bg-warning text-warning-foreground' : 'bg-muted')}
                    >
                      {due.text}
                    </span>
                    <Button size="sm" onClick={() => setActing({ kind: 'confirm', item })}>
                      Confirm
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setLower(lowerOptions(item)[0]?.value ?? 'good');
                        setActing({ kind: 'reverse', item });
                      }}
                    >
                      Reverse
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {acting && (
        <DecisionDialog
          title={acting.kind === 'confirm' ? 'Confirm the automatic decision' : 'Reverse the automatic decision'}
          description={
            acting.item.kind === 'account_status'
              ? acting.kind === 'confirm'
                ? `The account stays ${STATUS_LABELS[acting.item.to_status ?? 'restricted'].toLowerCase()}, now as a decision by a person.`
                : 'The account gets the lower status you choose.'
              : acting.kind === 'confirm'
                ? 'The pack stays hidden.'
                : 'The pack becomes visible again and its reports are cleared.'
          }
          submitLabel={acting.kind === 'confirm' ? 'Confirm' : 'Reverse'}
          onSubmit={(reason) =>
            acting.kind === 'confirm'
              ? api.confirmCase(acting.item.id, reason)
              : api.reverseCase(acting.item.id, reason, acting.item.kind === 'account_status' ? lower : undefined)
          }
          onDone={() => {
            setActing(null);
            setAttempt((n) => n + 1);
          }}
          onCancel={() => setActing(null)}
        >
          {acting.kind === 'reverse' && acting.item.kind === 'account_status' && (
            <Select label="New status" value={lower} options={lowerOptions(acting.item)} onValueChange={setLower} />
          )}
        </DecisionDialog>
      )}
    </div>
  );
}
