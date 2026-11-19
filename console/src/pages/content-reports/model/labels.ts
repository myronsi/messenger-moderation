import { ruleLabel } from '@/entities/moderation';
import type { ContentDecision } from '@/shared/api/content';
import { formatDateTime } from '@/shared/lib/time';

export const CONTENT_REASON_LABELS: Record<string, string> = {
  spam: 'Spam',
  scam: 'Scam or fraud',
  harassment: 'Harassment or bullying',
  impersonation: 'Pretends to be someone else',
  inappropriate_profile: 'Offensive or inappropriate content',
  illegal: 'Illegal content',
  other: 'Something else',
};

export const decisionText = (d: ContentDecision) => {
  const when = formatDateTime(d.decided_at);
  if (d.action === 'removed') return `Removed by ${d.staff_username} on ${when} · ${ruleLabel(d.reason_code)}`;
  return `No violation, decided by ${d.staff_username} on ${when}`;
};
