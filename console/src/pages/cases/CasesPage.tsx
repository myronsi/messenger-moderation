import { useEffect, useState } from 'react';
import { Plus } from 'lucide-react';
import { STATUS_LABELS } from '@/entities/moderation';
import { DecisionDialog } from '@/features/decision-dialog';
import { decisionsApi, type DecisionsApi, type ModerationCase, type NoticeOutcome } from '@/shared/api/decisions';
import { noticesApi, type NoticesApi } from '@/shared/api/notices';
import { ACCOUNT_STATUSES, type AccountStatus } from '@/shared/api/review';
import { cn } from '@/shared/lib/utils';
import { Button } from '@/shared/ui/button';
import { Select } from '@/shared/ui/select';
import { deadlineText } from './model/deadline';
import { KIND_FILTER_OPTIONS, OUTCOME_LABELS, type KindFilter } from './model/kinds';
import { AddNoticeDialog } from './ui/AddNoticeDialog';
import { CaseSummary } from './ui/CaseSummary';

type Tab = 'open' | 'decided';
type Load = { state: 'loading' } | { state: 'error' } | { state: 'ready'; cases: ModerationCase[] };
type Acting = null | { kind: 'confirm' | 'reverse' | 'notice'; item: ModerationCase };

const LEVEL: Record<AccountStatus, number> = { good: 0, warned: 1, limited: 2, restricted: 3, suspended: 4 };
const OUTCOME_OPTIONS = (Object.keys(OUTCOME_LABELS) as NoticeOutcome[]).map((value) => ({ value, label: OUTCOME_LABELS[value] }));

export interface CasesPageProps {
  api?: Pick<DecisionsApi, 'cases' | 'confirmCase' | 'reverseCase'>;
  notices?: Pick<NoticesApi, 'settings' | 'create' | 'decide'>;
  accountHref: (id: string) => string;
  packHref: (kind: string, id: string) => string;
  now?: () => Date;
}

const currentLevel = (item: ModerationCase): AccountStatus =>
  (item.kind === 'review_request' ? (item.subject?.status ?? item.to_status) : item.to_status) ?? 'suspended';

const lowerOptions = (item: ModerationCase) =>
  ACCOUNT_STATUSES.filter((s) => LEVEL[s] < LEVEL[currentLevel(item)]).map((value) => ({ value, label: STATUS_LABELS[value] }));

function dialogText(acting: NonNullable<Acting>): { title: string; description: string; submit: string } {
  const { kind, item } = acting;
  if (kind === 'notice') {
    const reply = item.notice?.notifier_email
      ? 'The notifier gets the outcome, the rule and your text by email.'
      : 'The notifier gave no email address, so no reply is sent.';
    const first = item.user_id ? ' Act on the account from its page first, then record the outcome here.' : '';
    return { title: 'Decide on the notice', description: reply + first, submit: 'Record decision' };
  }
  if (item.kind === 'pack_hidden') {
    return kind === 'confirm'
      ? { title: 'Confirm the automatic decision', description: 'The pack stays hidden.', submit: 'Confirm' }
      : { title: 'Reverse the automatic decision', description: 'The pack becomes visible again and its reports are cleared.', submit: 'Reverse' };
  }
  const level = STATUS_LABELS[currentLevel(item)].toLowerCase();
  if (item.kind === 'review_request') {
    return kind === 'confirm'
      ? { title: 'Keep the status after review', description: `The account stays ${level}, now as a decision by a person. The user sees your text.`, submit: 'Keep status' }
      : { title: 'Lower the status after review', description: 'The account gets the lower status you choose. The user sees your text.', submit: 'Lower status' };
  }
  return kind === 'confirm'
    ? { title: 'Confirm the automatic decision', description: `The account stays ${level}, now as a decision by a person.`, submit: 'Confirm' }
    : { title: 'Reverse the automatic decision', description: 'The account gets the lower status you choose.', submit: 'Reverse' };
}

export default function CasesPage({ api = decisionsApi, notices = noticesApi, accountHref, packHref, now = () => new Date() }: CasesPageProps) {
  const [tab, setTab] = useState<Tab>('open');
  const [kind, setKind] = useState<KindFilter>('all');
  const [load, setLoad] = useState<Load>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const [acting, setActing] = useState<Acting>(null);
  const [lower, setLower] = useState<AccountStatus>('good');
  const [outcome, setOutcome] = useState<NoticeOutcome>('action_taken');
  const [adding, setAdding] = useState(false);
  const [mailEnabled, setMailEnabled] = useState<boolean | null>(null);

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.cases(tab, kind === 'all' ? undefined : kind).then(
      (cases) => live && setLoad({ state: 'ready', cases }),
      () => live && setLoad({ state: 'error' }),
    );
    return () => {
      live = false;
    };
  }, [api, tab, kind, attempt]);

  useEffect(() => {
    let live = true;
    notices.settings().then(
      (s) => live && setMailEnabled(s.mail_enabled),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [notices]);

  const at = now();
  const reload = () => setAttempt((n) => n + 1);
  const text = acting && dialogText(acting);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-semibold">Cases</h1>
        <div className="flex flex-wrap items-end gap-2">
          <Select label="Source" value={kind} options={KIND_FILTER_OPTIONS} onValueChange={setKind} className="min-w-48" />
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
          <Button size="sm" onClick={() => setAdding(true)}>
            <Plus aria-hidden className="size-4" />
            Add notice
          </Button>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        Automatic restricted and suspended levels, packs hidden after reports, review requests from users and notices by email. Each has a 72-hour deadline;
        the most urgent come first.
      </p>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading cases…
        </p>
      )}
      {load.state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The cases could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={reload}>
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
              <li key={item.id} className="flex flex-col gap-2 rounded-lg border bg-card p-3 text-sm md:flex-row md:items-start md:justify-between">
                <CaseSummary item={item} accountHref={accountHref} packHref={packHref} />
                {item.state === 'open' && (
                  <div className="flex shrink-0 flex-wrap items-center gap-2">
                    <span
                      data-testid="deadline"
                      className={cn('rounded-full px-2 py-0.5 text-xs font-medium', due.overdue ? 'bg-destructive text-destructive-foreground' : due.soon ? 'bg-warning text-warning-foreground' : 'bg-muted')}
                    >
                      {due.text}
                    </span>
                    {item.kind === 'notice' ? (
                      <Button size="sm" onClick={() => setActing({ kind: 'notice', item })}>
                        Decide
                      </Button>
                    ) : (
                      <>
                        <Button size="sm" onClick={() => setActing({ kind: 'confirm', item })}>
                          {item.kind === 'review_request' ? 'Keep status' : 'Confirm'}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={item.kind !== 'pack_hidden' && lowerOptions(item).length === 0}
                          onClick={() => {
                            setLower(lowerOptions(item)[0]?.value ?? 'good');
                            setActing({ kind: 'reverse', item });
                          }}
                        >
                          {item.kind === 'review_request' ? 'Lower status' : 'Reverse'}
                        </Button>
                      </>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {acting && text && (
        <DecisionDialog
          title={text.title}
          description={text.description}
          submitLabel={text.submit}
          textLabel={acting.kind === 'notice' ? 'Text for the notifier (sent by email when they gave an address)' : undefined}
          onSubmit={(reason) => {
            if (acting.kind === 'notice') return notices.decide(acting.item.id, outcome, reason);
            if (acting.kind === 'confirm') return api.confirmCase(acting.item.id, reason);
            return api.reverseCase(acting.item.id, reason, acting.item.kind === 'pack_hidden' ? undefined : lower);
          }}
          onDone={() => {
            setActing(null);
            reload();
          }}
          onCancel={() => setActing(null)}
        >
          {acting.kind === 'reverse' && acting.item.kind !== 'pack_hidden' && (
            <Select label="New status" value={lower} options={lowerOptions(acting.item)} onValueChange={setLower} />
          )}
          {acting.kind === 'notice' && <Select label="Outcome" value={outcome} options={OUTCOME_OPTIONS} onValueChange={setOutcome} />}
        </DecisionDialog>
      )}
      {adding && (
        <AddNoticeDialog
          api={notices}
          mailEnabled={mailEnabled}
          now={now}
          onDone={() => {
            setAdding(false);
            setKind('notice');
            setTab('open');
            reload();
          }}
          onCancel={() => setAdding(false)}
        />
      )}
    </div>
  );
}
