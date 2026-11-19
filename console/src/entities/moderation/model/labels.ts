import type { AccountStatus, QueueAge, ReportContext, ReportReason } from '@/shared/api/review';

export const STATUS_LABELS: Record<AccountStatus, string> = {
  good: 'Good',
  warned: 'Warned',
  limited: 'Limited',
  restricted: 'Restricted',
  suspended: 'Suspended',
};

export const REASON_LABELS: Record<ReportReason, string> = {
  spam: 'Spam',
  scam: 'Scam',
  harassment: 'Harassment',
  impersonation: 'Impersonation',
  inappropriate_profile: 'Inappropriate profile',
  other: 'Other',
};

export const CONTEXT_LABELS: Record<ReportContext, string> = {
  target_initiated: 'They contacted the reporter first',
  shared_chat: 'They share a chat',
  none: 'No shared chat',
};

export const AGE_LABELS: Record<QueueAge, string> = {
  any: 'Any age',
  day: 'Under 24 hours',
  week: '1 to 7 days',
  older: 'Over 7 days',
};

const CAUSE_LABELS: Record<string, string> = {
  reports: 'New reports',
  decay: 'Reports aged out',
  quiet: '30 quiet days',
  appeal: 'Appeal by the user',
  probation_passed: 'Probation passed',
  probation_failed: 'Probation failed',
};

export const causeLabel = (cause: string) => CAUSE_LABELS[cause] ?? cause;

export const APPEAL_CAUSES = new Set(['appeal', 'probation_passed', 'probation_failed']);

export const statusLabel = (status: string) => STATUS_LABELS[status as AccountStatus] ?? status;

export const reasonLabel = (reason: string) => REASON_LABELS[reason as ReportReason] ?? reason;
