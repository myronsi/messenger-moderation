import { useState } from 'react';
import { ShieldAlert, Trash2, UserX } from 'lucide-react';
import { ruleLabel, STATUS_LABELS } from '@/entities/moderation';
import { DecisionDialog } from '@/features/decision-dialog';
import { decisionsApi, type DecisionsApi } from '@/shared/api/decisions';
import { ACCOUNT_STATUSES, type AccountDetail, type AccountStatus } from '@/shared/api/review';
import { Button } from '@/shared/ui/button';
import { Select } from '@/shared/ui/select';

const HEAVY: AccountStatus[] = ['restricted', 'suspended'];

type Open = null | 'status' | 'dismiss' | 'delete';

export interface AccountActionsProps {
  detail: AccountDetail;
  isAdmin: boolean;
  api?: Pick<DecisionsApi, 'setStatus' | 'dismissReports' | 'deleteAccount'>;
  onChanged: () => void;
  onDeleted: () => void;
}

export function AccountActions({ detail, isAdmin, api = decisionsApi, onChanged, onDeleted }: AccountActionsProps) {
  const [open, setOpen] = useState<Open>(null);
  const [status, setStatus] = useState<AccountStatus>(detail.status.status);
  const active = detail.reports.filter((report) => report.withdrawn_at === null && !report.dismissed_at);
  const [selected, setSelected] = useState<string[]>([]);
  const user = detail.user;
  const close = () => setOpen(null);

  return (
    <section aria-labelledby="actions-title" className="rounded-lg border bg-card p-4 text-sm">
      <h2 id="actions-title" className="mb-2 font-semibold">
        Decisions
      </h2>
      {detail.status.decided_by === 'staff' && (
        <div className="mb-3 rounded-md bg-muted/50 p-2" data-testid="current-decision">
          <p className="font-medium">Decided by staff: {ruleLabel(detail.status.reason_code)}</p>
          {detail.status.reason_text && <p className="whitespace-pre-line text-muted-foreground">{detail.status.reason_text}</p>}
        </div>
      )}
      <div className="flex flex-col gap-2">
        <Button variant="outline" size="sm" onClick={() => { setStatus(detail.status.status); setOpen('status'); }}>
          <ShieldAlert aria-hidden />
          Set status
        </Button>
        <Button variant="outline" size="sm" disabled={active.length === 0} onClick={() => { setSelected([]); setOpen('dismiss'); }}>
          <UserX aria-hidden />
          Dismiss reports
        </Button>
        {isAdmin && (
          <Button variant="destructive" size="sm" onClick={() => setOpen('delete')}>
            <Trash2 aria-hidden />
            Delete account
          </Button>
        )}
      </div>

      {open === 'status' && (
        <DecisionDialog
          title={`Set the status of @${user.username}`}
          description="Applies at once and replaces the automatic level. Only a person can lower it again; new reports can still raise it."
          submitLabel={HEAVY.includes(status) ? `${STATUS_LABELS[status]} the account` : 'Save status'}
          destructive={HEAVY.includes(status)}
          confirmWord={HEAVY.includes(status) ? user.username : undefined}
          onSubmit={(reason) => api.setStatus(user.id, status, reason)}
          onDone={() => { close(); onChanged(); }}
          onCancel={close}
        >
          <Select
            label="New status"
            value={status}
            options={ACCOUNT_STATUSES.map((value) => ({ value, label: STATUS_LABELS[value] }))}
            onValueChange={setStatus}
          />
        </DecisionDialog>
      )}

      {open === 'dismiss' && (
        <DecisionDialog
          title={`Dismiss reports on @${user.username}`}
          description="Dismissed reports stop counting and the status is worked out again. Reporters are told no violation was found."
          submitLabel={selected.length === 0 ? `Dismiss all ${active.length}` : `Dismiss ${selected.length}`}
          onSubmit={(reason) => api.dismissReports(user.id, reason, selected.length === 0 ? undefined : selected)}
          onDone={() => { close(); onChanged(); }}
          onCancel={close}
        >
          <fieldset className="flex max-h-48 flex-col gap-1 overflow-y-auto rounded-md border p-2">
            <legend className="px-1 text-xs font-medium text-muted-foreground">Only these reports (none ticked: all)</legend>
            {active.map((report) => (
              <label key={report.reporter.id} className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={selected.includes(report.reporter.id)}
                  onChange={(event) =>
                    setSelected((current) => (event.target.checked ? [...current, report.reporter.id] : current.filter((id) => id !== report.reporter.id)))
                  }
                />
                @{report.reporter.username}
              </label>
            ))}
          </fieldset>
        </DecisionDialog>
      )}

      {open === 'delete' && (
        <DecisionDialog
          title={`Delete @${user.username}`}
          description="Deletes the account, its direct chats and the groups nobody else is in. This cannot be undone."
          submitLabel="Delete account"
          destructive
          confirmWord={user.username}
          onSubmit={(reason) => api.deleteAccount(user.id, reason)}
          onDone={() => { close(); onDeleted(); }}
          onCancel={close}
        />
      )}
    </section>
  );
}
