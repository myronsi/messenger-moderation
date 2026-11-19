import { useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { reviewApi, type QueueFilter, type QueueItem, type ReviewApi } from '@/shared/api/review';
import { Button } from '@/shared/ui/button';
import { DEFAULT_FILTER } from './model/filter';
import { QueueFilters } from './ui/QueueFilters';
import { QueueTable } from './ui/QueueTable';

type LoadState = 'loading' | 'ready' | 'error';

export interface QueuePageProps {
  api?: Pick<ReviewApi, 'queue'>;
  accountHref: (id: string) => string;
  initialFilter?: QueueFilter;
  onFilterChange?: (filter: QueueFilter) => void;
  now?: () => Date;
}

export default function QueuePage({ api = reviewApi, accountHref, initialFilter = DEFAULT_FILTER, onFilterChange, now = () => new Date() }: QueuePageProps) {
  const [filter, setFilter] = useState(initialFilter);
  const [items, setItems] = useState<QueueItem[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [state, setState] = useState<LoadState>('loading');
  const [moreState, setMoreState] = useState<'idle' | 'loading' | 'error'>('idle');
  const [reload, setReload] = useState(0);

  useEffect(() => {
    let live = true;
    setState('loading');
    setMoreState('idle');
    api.queue(filter, null).then(
      (page) => {
        if (!live) return;
        setItems(page.items);
        setCursor(page.next_cursor);
        setState('ready');
      },
      () => {
        if (live) setState('error');
      },
    );
    return () => {
      live = false;
    };
  }, [api, filter, reload]);

  const changeFilter = (next: QueueFilter) => {
    setFilter(next);
    onFilterChange?.(next);
  };

  const loadMore = () => {
    if (!cursor) return;
    setMoreState('loading');
    api.queue(filter, cursor).then(
      (page) => {
        setItems((current) => [...current, ...page.items]);
        setCursor(page.next_cursor);
        setMoreState('idle');
      },
      () => setMoreState('error'),
    );
  };

  return (
    <section aria-labelledby="queue-title" className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 id="queue-title" className="text-xl font-semibold">
            Review queue
          </h1>
          <p className="text-sm text-muted-foreground">Accounts with new reports or a restricted or suspended status, most urgent first.</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => setReload((n) => n + 1)} disabled={state === 'loading'}>
          <RefreshCw aria-hidden />
          Refresh
        </Button>
      </div>

      <QueueFilters value={filter} onChange={changeFilter} />

      {state === 'loading' && (
        <p role="status" className="py-8 text-center text-sm text-muted-foreground">
          Loading the queue…
        </p>
      )}

      {state === 'error' && (
        <div role="alert" className="flex flex-col items-center gap-3 rounded-md border border-destructive/40 bg-destructive-soft p-6 text-sm">
          <p>The queue could not be loaded.</p>
          <Button variant="outline" size="sm" onClick={() => setReload((n) => n + 1)}>
            Try again
          </Button>
        </div>
      )}

      {state === 'ready' && items.length === 0 && (
        <p role="status" className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
          Nothing to review with these filters.
        </p>
      )}

      {state === 'ready' && items.length > 0 && (
        <>
          <QueueTable items={items} accountHref={accountHref} now={now()} />
          {cursor && (
            <div className="flex flex-col items-center gap-2">
              {moreState === 'error' && (
                <p role="alert" className="text-sm text-destructive">
                  More accounts could not be loaded.
                </p>
              )}
              <Button variant="outline" onClick={loadMore} disabled={moreState === 'loading'}>
                {moreState === 'loading' ? 'Loading…' : 'Load more'}
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  );
}
