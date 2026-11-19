import { useEffect, useId, useState } from 'react';
import { ApiError } from '@/shared/api/client';
import { NOTICE_LIMITS, type NoticeInput, type NoticesApi } from '@/shared/api/notices';
import { Button } from '@/shared/ui/button';
import { Input } from '@/shared/ui/input';

interface AddNoticeDialogProps {
  api: Pick<NoticesApi, 'create'>;
  mailEnabled: boolean | null;
  now: () => Date;
  onDone: () => void;
  onCancel: () => void;
}

const pad = (n: number) => String(n).padStart(2, '0');
const localInput = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
const area = 'rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

type Field = 'notifier_name' | 'notifier_email' | 'content' | 'location' | 'explanation' | 'user_id';

const FIELDS: { key: Field; label: string; hint?: string; max: number; rows?: number; required: boolean }[] = [
  { key: 'notifier_name', label: 'Who sent it', hint: 'Name or organisation as given in the email', max: NOTICE_LIMITS.name, required: true },
  { key: 'notifier_email', label: 'Email for the reply (optional)', hint: 'The decision is emailed here', max: NOTICE_LIMITS.email, required: false },
  { key: 'content', label: 'What was reported', max: NOTICE_LIMITS.content, rows: 3, required: true },
  { key: 'location', label: 'Where it is', hint: 'Link, @username, group name, date of the message', max: NOTICE_LIMITS.location, rows: 2, required: true },
  { key: 'explanation', label: 'Why it is illegal, in the sender’s words', max: NOTICE_LIMITS.explanation, rows: 4, required: true },
  { key: 'user_id', label: 'Account ID (optional)', hint: 'Fill in when you found the account, so the case links to it', max: 20, required: false },
];

export function AddNoticeDialog({ api, mailEnabled, now, onDone, onCancel }: AddNoticeDialogProps) {
  const titleId = useId();
  const baseId = useId();
  const [values, setValues] = useState<Record<Field, string>>({ notifier_name: '', notifier_email: '', content: '', location: '', explanation: '', user_id: '' });
  const [received, setReceived] = useState(() => localInput(now()));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [busy, onCancel]);

  const ready = FIELDS.every((f) => !f.required || values[f.key].trim() !== '') && received !== '' && !busy;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!ready) return;
    setBusy(true);
    setError(null);
    const input: NoticeInput = { ...values, received_at: new Date(received).toISOString() };
    try {
      await api.create(input);
      onDone();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : 'The notice could not be saved. Try again.');
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 p-0 sm:items-center sm:p-4">
      <div role="dialog" aria-modal="true" aria-labelledby={titleId} className="max-h-[100dvh] w-full max-w-lg overflow-y-auto rounded-t-lg border bg-background p-5 shadow-lg sm:rounded-lg">
        <form onSubmit={submit} className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <h2 id={titleId} className="text-lg font-semibold">
              Add a notice that arrived by email
            </h2>
            <p className="text-sm text-muted-foreground">
              Copy the facts from the email. The case gets a 72-hour deadline from the time it was received.
              {mailEnabled === false && ' Email is not set up here, so reply to the sender by hand after the decision.'}
            </p>
          </div>
          {FIELDS.map((f) => {
            const id = `${baseId}-${f.key}`;
            return (
              <div key={f.key} className="flex flex-col gap-1">
                <label htmlFor={id} className="text-xs font-medium text-muted-foreground">
                  {f.label}
                </label>
                {f.rows ? (
                  <textarea
                    id={id}
                    rows={f.rows}
                    maxLength={f.max}
                    required={f.required}
                    value={values[f.key]}
                    onChange={(event) => setValues((v) => ({ ...v, [f.key]: event.target.value }))}
                    className={area}
                  />
                ) : (
                  <Input
                    id={id}
                    type={f.key === 'notifier_email' ? 'email' : 'text'}
                    inputMode={f.key === 'user_id' ? 'numeric' : undefined}
                    maxLength={f.max}
                    required={f.required}
                    value={values[f.key]}
                    onChange={(event) => setValues((v) => ({ ...v, [f.key]: event.target.value }))}
                  />
                )}
                {f.hint && <span className="text-xs text-muted-foreground">{f.hint}</span>}
              </div>
            );
          })}
          <div className="flex flex-col gap-1">
            <label htmlFor={`${baseId}-received`} className="text-xs font-medium text-muted-foreground">
              Received at (your local time)
            </label>
            <Input id={`${baseId}-received`} type="datetime-local" required value={received} onChange={(event) => setReceived(event.target.value)} />
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
            <Button type="submit" disabled={!ready}>
              {busy ? 'Saving…' : 'Add notice'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
