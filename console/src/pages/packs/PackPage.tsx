import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Eye, EyeOff } from 'lucide-react';
import { ruleLabel } from '@/entities/moderation';
import { DecisionDialog } from '@/features/decision-dialog';
import { ApiError } from '@/shared/api/client';
import { decisionsApi, packImageUrl, type DecisionsApi, type PackDetail, type PackKind, type ReportedPack } from '@/shared/api/decisions';
import { formatDateTime } from '@/shared/lib/time';
import { Button } from '@/shared/ui/button';

type Load<T> = { state: 'loading' } | { state: 'missing' } | { state: 'error' } | { state: 'ready'; value: T };

const KIND_LABELS: Record<PackKind, string> = { sticker_pack: 'Sticker pack', emoji_pack: 'Emoji pack' };

const PACK_REASON_LABELS: Record<string, string> = {
  spam: 'Spam',
  violence: 'Violence',
  sexual: 'Sexual content',
  hate: 'Hate',
  copyright: 'Copyright',
  other: 'Other',
};

const hiddenText = (hiddenBy: string | null) => (hiddenBy === 'staff' ? 'Hidden by staff' : 'Hidden after reports');

export interface PackPageProps {
  kind: PackKind;
  id: string;
  api?: Pick<DecisionsApi, 'pack' | 'setPackHidden'>;
  accountHref: (id: string) => string;
  packsHref: string;
}

export function PackPage({ kind, id, api = decisionsApi, accountHref, packsHref }: PackPageProps) {
  const [load, setLoad] = useState<Load<PackDetail>>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const [acting, setActing] = useState<null | 'hide' | 'unhide'>(null);

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.pack(kind, id).then(
      (value) => live && setLoad({ state: 'ready', value }),
      (error: unknown) => live && setLoad({ state: error instanceof ApiError && error.status === 404 ? 'missing' : 'error' }),
    );
    return () => {
      live = false;
    };
  }, [api, kind, id, attempt]);

  return (
    <div className="flex flex-col gap-4">
      <Link to={packsHref} className="inline-flex items-center gap-1 self-start text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft aria-hidden className="size-4" />
        Back to packs
      </Link>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading the pack…
        </p>
      )}
      {load.state === 'missing' && (
        <p role="alert" className="rounded-md border border-dashed p-8 text-center text-sm">
          No {KIND_LABELS[kind].toLowerCase()} with ID {id}.
        </p>
      )}
      {load.state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The pack could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}
      {load.state === 'ready' && (
        <>
          <header className="flex flex-col gap-3 rounded-lg border bg-card p-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">{KIND_LABELS[kind]}</p>
              <h1 className="truncate text-xl font-semibold">{load.value.title}</h1>
              <p className="text-sm text-muted-foreground">
                {load.value.owner ? (
                  <>
                    by{' '}
                    <Link to={accountHref(load.value.owner.id)} className="hover:underline">
                      @{load.value.owner.username}
                    </Link>
                  </>
                ) : load.value.builtin ? (
                  'Built-in pack'
                ) : (
                  'Owner deleted'
                )}
                {load.value.group_pack && ' · Group pack'} · {load.value.items.length} {kind === 'emoji_pack' ? 'emoji' : 'stickers'}
              </p>
              <p className="mt-1 text-sm" data-testid="pack-state">
                {load.value.deleted_at ? 'Deleted by its owner' : load.value.hidden_at ? `${hiddenText(load.value.hidden_by)} on ${formatDateTime(load.value.hidden_at)}` : 'Visible'}
              </p>
            </div>
            {!load.value.deleted_at && !load.value.builtin && (
              load.value.hidden_at ? (
                <Button variant="outline" onClick={() => setActing('unhide')}>
                  <Eye aria-hidden />
                  Make visible
                </Button>
              ) : (
                <Button variant="destructive" onClick={() => setActing('hide')}>
                  <EyeOff aria-hidden />
                  Hide pack
                </Button>
              )
            )}
          </header>
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <section aria-labelledby="items-title" className="flex flex-col gap-2">
              <h2 id="items-title" className="font-semibold">
                {kind === 'emoji_pack' ? 'Emoji' : 'Stickers'}
              </h2>
              {load.value.items.length === 0 ? (
                <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">The pack is empty.</p>
              ) : (
                <ul aria-label="Pack contents" className="grid grid-cols-3 gap-2 sm:grid-cols-5 xl:grid-cols-8">
                  {load.value.items.map((item) => (
                    <li key={item.id} className="flex flex-col items-center gap-1 rounded-md border bg-card p-2">
                      <img
                        src={packImageUrl(kind, id, item.attachment_id)}
                        alt={kind === 'emoji_pack' ? `:${item.label}:` : `Sticker ${item.label}`}
                        loading="lazy"
                        className="aspect-square w-full object-contain"
                      />
                      <span className="max-w-full truncate text-xs text-muted-foreground">{kind === 'emoji_pack' ? `:${item.label}:` : item.label}</span>
                    </li>
                  ))}
                </ul>
              )}
            </section>
            <aside className="flex flex-col gap-4">
              <section aria-labelledby="pack-reports-title" className="rounded-lg border bg-card p-4 text-sm">
                <h2 id="pack-reports-title" className="mb-2 font-semibold">
                  Reports ({load.value.reports.length})
                </h2>
                {load.value.reports.length === 0 ? (
                  <p className="text-muted-foreground">No open reports.</p>
                ) : (
                  <ul className="flex flex-col gap-1">
                    {load.value.reports.map((report) => (
                      <li key={report.reporter_id}>
                        <Link to={accountHref(report.reporter_id)} className="hover:underline">
                          @{report.reporter_username}
                        </Link>
                        : {PACK_REASON_LABELS[report.reason] ?? report.reason}
                        <span className="block text-xs text-muted-foreground">{formatDateTime(report.created_at)}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
              {load.value.cases.length > 0 && (
                <section aria-labelledby="pack-cases-title" className="rounded-lg border bg-card p-4 text-sm">
                  <h2 id="pack-cases-title" className="mb-2 font-semibold">
                    Cases
                  </h2>
                  <ul className="flex flex-col gap-1">
                    {load.value.cases.map((c) => (
                      <li key={c.id}>
                        {c.state === 'open' ? `Open, due ${formatDateTime(c.deadline)}` : `${c.state[0].toUpperCase()}${c.state.slice(1)}`}
                        {c.decided_by && ` by ${c.decided_by}`}
                        {c.reason_code && ` · ${ruleLabel(c.reason_code)}`}
                      </li>
                    ))}
                  </ul>
                </section>
              )}
            </aside>
          </div>
          {acting && (
            <DecisionDialog
              title={acting === 'hide' ? `Hide "${load.value.title}"` : `Make "${load.value.title}" visible`}
              description={
                acting === 'hide'
                  ? 'Nobody except its owner can find or add the pack. Reporters are told action was taken.'
                  : 'The pack can be found again and its reports are cleared. Reporters are told no violation was found.'
              }
              submitLabel={acting === 'hide' ? 'Hide pack' : 'Make visible'}
              destructive={acting === 'hide'}
              onSubmit={(reason) => api.setPackHidden(kind, id, acting === 'hide', reason)}
              onDone={() => {
                setActing(null);
                setAttempt((n) => n + 1);
              }}
              onCancel={() => setActing(null)}
            />
          )}
        </>
      )}
    </div>
  );
}

export interface PacksPageProps {
  api?: Pick<DecisionsApi, 'packs'>;
  packHref: (kind: string, id: string) => string;
}

export function PacksPage({ api = decisionsApi, packHref }: PacksPageProps) {
  const [load, setLoad] = useState<Load<ReportedPack[]>>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let live = true;
    setLoad({ state: 'loading' });
    api.packs().then(
      (value) => live && setLoad({ state: 'ready', value }),
      () => live && setLoad({ state: 'error' }),
    );
    return () => {
      live = false;
    };
  }, [api, attempt]);

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">Packs</h1>
      <p className="text-sm text-muted-foreground">Sticker and emoji packs with reports or hidden.</p>
      {load.state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading packs…
        </p>
      )}
      {(load.state === 'error' || load.state === 'missing') && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The packs could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}
      {load.state === 'ready' && load.value.length === 0 && (
        <p className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">No reported or hidden packs.</p>
      )}
      {load.state === 'ready' && load.value.length > 0 && (
        <ul aria-label="Packs" className="flex flex-col gap-2">
          {load.value.map((p) => (
            <li key={`${p.kind}:${p.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-card p-3 text-sm">
              <Link to={packHref(p.kind, p.id)} className="font-medium hover:underline">
                {KIND_LABELS[p.kind]}: {p.title}
              </Link>
              <span className="text-muted-foreground">
                {p.reports} {p.reports === 1 ? 'report' : 'reports'}
                {p.hidden_at && ` · ${hiddenText(p.hidden_by)}`}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
