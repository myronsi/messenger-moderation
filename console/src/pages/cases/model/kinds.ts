import type { CaseKind, CaseState, NoticeMailState, NoticeOutcome } from '@/shared/api/decisions';

export const KIND_LABELS: Record<CaseKind, string> = {
  account_status: 'Automatic heavy level',
  pack_hidden: 'Pack hidden after reports',
  review_request: 'User asked for a review',
  notice: 'Notice by email',
};

export type KindFilter = CaseKind | 'all';

export const KIND_FILTER_OPTIONS: { value: KindFilter; label: string }[] = [
  { value: 'all', label: 'All sources' },
  ...(Object.keys(KIND_LABELS) as CaseKind[]).map((value) => ({ value, label: KIND_LABELS[value] })),
];

export const OUTCOME_LABELS: Record<NoticeOutcome, string> = {
  action_taken: 'Action taken',
  no_violation: 'No violation',
};

export const MAIL_LABELS: Record<NoticeMailState, string> = {
  none: 'No email address given',
  pending: 'Email to the notifier is being sent',
  sent: 'Email sent to the notifier',
  failed: 'Email to the notifier failed; reply by hand',
  skipped: 'Email is not set up; reply to the notifier by hand',
};

export function stateLabel(kind: CaseKind, state: CaseState): string {
  if (state === 'open') return 'Open';
  if (state === 'superseded') return kind === 'review_request' ? 'Closed: the status returned to good' : 'Replaced by a newer case';
  if (kind === 'notice') return state === 'confirmed' ? OUTCOME_LABELS.action_taken : OUTCOME_LABELS.no_violation;
  return state === 'confirmed' ? 'Confirmed' : 'Reversed';
}
