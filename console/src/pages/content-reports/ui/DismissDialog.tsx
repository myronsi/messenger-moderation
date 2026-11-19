import { useEffect, useId, useState } from 'react';
import { ApiError } from '@/shared/api/client';
import { Button } from '@/shared/ui/button';

const MAX_NOTE = 1000;

interface DismissDialogProps {
  onSubmit: (note: string) => Promise<unknown>;
  onDone: () => void;
  onCancel: () => void;
}

export function DismissDialog({ onSubmit, onDone, onCancel }: DismissDialogProps) {
  const titleId = useId();
  const noteId = useId();
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [busy, onCancel]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit(note.trim());
      onDone();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : 'The decision could not be sent. Try again.');
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 p-0 sm:items-center sm:p-4">
      <div role="dialog" aria-modal="true" aria-labelledby={titleId} className="w-full max-w-lg rounded-t-lg border bg-background p-5 shadow-lg sm:rounded-lg">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <h2 id={titleId} className="text-lg font-semibold">
              No violation
            </h2>
            <p className="text-sm text-muted-foreground">The content stays. The reports on it leave the open list. Nobody in Messenger is told.</p>
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor={noteId} className="text-xs font-medium text-muted-foreground">
              Internal note (optional, only staff see it)
            </label>
            <textarea
              id={noteId}
              value={note}
              onChange={(event) => setNote(event.target.value)}
              rows={3}
              maxLength={MAX_NOTE}
              className="rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button type="button" variant="outline" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? 'Sending…' : 'No violation'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
