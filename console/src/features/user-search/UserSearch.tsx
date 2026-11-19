import { Link } from 'react-router-dom';
import { useEffect, useId, useState } from 'react';
import { Search } from 'lucide-react';
import { StatusBadge } from '@/entities/moderation';
import { reviewApi, type ReviewApi, type UserHit } from '@/shared/api/review';
import { Input } from '@/shared/ui/input';

const MIN_LENGTH = 2;

type Result = { state: 'idle' } | { state: 'loading' } | { state: 'error' } | { state: 'ready'; hits: UserHit[] };

interface UserSearchProps {
  api?: Pick<ReviewApi, 'searchUsers'>;
  accountHref: (id: string) => string;
  delayMs?: number;
}

export function UserSearch({ api = reviewApi, accountHref, delayMs = 250 }: UserSearchProps) {
  const [query, setQuery] = useState('');
  const [result, setResult] = useState<Result>({ state: 'idle' });
  const listId = useId();
  const term = query.trim().replace(/^@/, '');

  useEffect(() => {
    if (term.length < MIN_LENGTH && !/^\d+$/.test(term)) {
      setResult({ state: 'idle' });
      return;
    }
    let live = true;
    setResult({ state: 'loading' });
    const timer = window.setTimeout(() => {
      api.searchUsers(term).then(
        (hits) => {
          if (live) setResult({ state: 'ready', hits });
        },
        () => {
          if (live) setResult({ state: 'error' });
        },
      );
    }, delayMs);
    return () => {
      live = false;
      window.clearTimeout(timer);
    };
  }, [api, term, delayMs]);

  return (
    <div role="search" className="relative w-full max-w-sm">
      <Search aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        type="search"
        aria-label="Find an account"
        aria-controls={listId}
        placeholder="Username, name or ID"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Escape') setQuery('');
        }}
        className="h-9 pl-8"
      />
      {result.state !== 'idle' && (
        <div id={listId} className="absolute inset-x-0 top-full z-20 mt-1 overflow-hidden rounded-md border bg-popover text-sm shadow-md">
          {result.state === 'loading' && <p role="status" className="px-3 py-2 text-muted-foreground">Searching…</p>}
          {result.state === 'error' && <p role="alert" className="px-3 py-2 text-destructive">Search failed. Try again.</p>}
          {result.state === 'ready' && result.hits.length === 0 && (
            <p role="status" className="px-3 py-2 text-muted-foreground">No account matches "{term}".</p>
          )}
          {result.state === 'ready' && result.hits.length > 0 && (
            <ul aria-label="Accounts found" className="max-h-80 overflow-y-auto">
              {result.hits.map((hit) => (
                <li key={hit.id}>
                  <Link to={accountHref(hit.id)} onClick={() => setQuery('')} className="flex items-center justify-between gap-2 px-3 py-2 hover:bg-accent focus:bg-accent focus:outline-none">
                    <span className="min-w-0">
                      <span className="block truncate font-medium">{hit.display_name}</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        @{hit.username} · ID {hit.id}
                      </span>
                    </span>
                    <StatusBadge status={hit.status} />
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
