import { useEffect, useId, useState, type ReactNode } from 'react';
import { RULE_LABELS } from '@/entities/moderation';
import { ApiError } from '@/shared/api/client';
import { DECISION_RULES, MAX_REASON_TEXT, type DecisionRule, type Reason } from '@/shared/api/decisions';
import { Button } from '@/shared/ui/button';
import { Select } from '@/shared/ui/select';

export interface DecisionDialogProps {
  title: string;
  description?: ReactNode;
  submitLabel: string;
  destructive?: boolean;
  confirmWord?: string;
  textLabel?: string;
  children?: ReactNode;
  onSubmit: (reason: Reason) => Promise<unknown>;
  onDone: () => void;
  onCancel: () => void;
}

const RULE_OPTIONS = [
  { value: '' as const, label: 'Choose a rule' },
  ...DECISION_RULES.map((rule) => ({ value: rule, label: RULE_LABELS[rule] })),
];

export function DecisionDialog({
  title,
  description,
  submitLabel,
  destructive,
  confirmWord,
  textLabel = 'Text for the user (shown as written by Messenger moderation)',
  children,
  onSubmit,
  onDone,
  onCancel,
}: DecisionDialogProps) {
  const titleId = useId();
  const textId = useId();
  const confirmId = useId();
  const [rule, setRule] = useState<DecisionRule | ''>('');
  const [text, setText] = useState('');
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [busy, onCancel]);

  const trimmed = text.trim();
  const confirmed = !confirmWord || typed.trim() === confirmWord;
  const ready = rule !== '' && trimmed.length > 0 && trimmed.length <= MAX_REASON_TEXT && confirmed && !busy;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!ready) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({ reason_code: rule as DecisionRule, reason_text: trimmed });
      onDone();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : 'The decision could not be sent. Try again.');
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 p-0 sm:items-center sm:p-4">
      <div role="dialog" aria-modal="true" aria-labelledby={titleId} className="max-h-[100dvh] w-full max-w-lg overflow-y-auto rounded-t-lg border bg-background p-5 shadow-lg sm:rounded-lg">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <h2 id={titleId} className="text-lg font-semibold">
              {title}
            </h2>
            {description && <div className="text-sm text-muted-foreground">{description}</div>}
          </div>
          {children}
          <Select label="Rule from the guidelines" value={rule} options={RULE_OPTIONS} onValueChange={setRule} required />
          <div className="flex flex-col gap-1">
            <label htmlFor={textId} className="text-xs font-medium text-muted-foreground">
              {textLabel}
            </label>
            <textarea
              id={textId}
              value={text}
              onChange={(event) => setText(event.target.value)}
              rows={4}
              maxLength={MAX_REASON_TEXT}
              required
              className="rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <span className="self-end text-xs text-muted-foreground">
              {trimmed.length}/{MAX_REASON_TEXT}
            </span>
          </div>
          {confirmWord && (
            <div className="flex flex-col gap-1 rounded-md border border-destructive/40 bg-destructive-soft p-3">
              <label htmlFor={confirmId} className="text-sm">
                Type <strong>{confirmWord}</strong> to confirm.
              </label>
              <input
                id={confirmId}
                value={typed}
                onChange={(event) => setTyped(event.target.value)}
                autoComplete="off"
                className="h-9 rounded-md border border-input bg-background px-3 text-sm"
              />
            </div>
          )}
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button type="button" variant="outline" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" variant={destructive ? 'destructive' : 'default'} disabled={!ready}>
              {busy ? 'Sending…' : submitLabel}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
