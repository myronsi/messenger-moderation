import { ageWindow } from '@/shared/lib/time';
import { api } from './client';

export const ACCOUNT_STATUSES = ['good', 'warned', 'limited', 'restricted', 'suspended'] as const;
export type AccountStatus = (typeof ACCOUNT_STATUSES)[number];

export const REPORT_REASONS = ['spam', 'scam', 'harassment', 'impersonation', 'inappropriate_profile', 'other'] as const;
export type ReportReason = (typeof REPORT_REASONS)[number];

export type ReportContext = 'target_initiated' | 'shared_chat' | 'none';

export const QUEUE_AGES = ['any', 'day', 'week', 'older'] as const;
export type QueueAge = (typeof QUEUE_AGES)[number];

export interface QueueFilter {
  status: AccountStatus | 'any';
  reason: ReportReason | 'any';
  age: QueueAge;
}

export interface UserSummary {
  id: string;
  username: string;
  display_name: string;
  created_at: string;
}

export interface QueueItem {
  user: UserSummary;
  status: AccountStatus;
  score: number;
  status_since: string | null;
  open_reports: number;
  weighted_reports: number;
  reasons: ReportReason[];
  newest_report_at: string | null;
  opened_at: string;
}

export interface QueuePage {
  items: QueueItem[];
  next_cursor: string | null;
}

export interface ProfileSnapshot {
  username?: string;
  display_name?: string;
  bio?: string | null;
  avatar_attachment_id?: string | null;
}

export interface Reporter extends UserSummary {
  status: AccountStatus;
}

export interface UserReport {
  reporter: Reporter;
  reason: ReportReason;
  note: string | null;
  context: ReportContext;
  weight: number;
  snapshot: ProfileSnapshot;
  created_at: string;
  updated_at: string;
  withdrawn_at: string | null;
}

export interface StatusChange {
  id: string;
  from_status: AccountStatus;
  to_status: AccountStatus;
  score: number;
  cause: string;
  created_at: string;
}

export interface CurrentStatus {
  status: AccountStatus;
  score: number;
  since: string | null;
  probation_until: string | null;
  appeal_blocked_until: string | null;
  last_appeal_at: string | null;
}

export interface AccountProfile extends UserSummary {
  bio: string | null;
  last_seen_at: string;
}

export interface AccountDetail {
  user: AccountProfile;
  status: CurrentStatus;
  history: StatusChange[];
  reports: UserReport[];
}

export interface UserHit extends UserSummary {
  status: AccountStatus;
}

export interface ReviewApi {
  queue(filter: QueueFilter, cursor: string | null): Promise<QueuePage>;
  account(id: string): Promise<AccountDetail>;
  searchUsers(query: string): Promise<UserHit[]>;
}

export function queuePath(filter: QueueFilter, cursor: string | null, now: Date): string {
  const params = new URLSearchParams();
  if (filter.status !== 'any') params.set('status', filter.status);
  if (filter.reason !== 'any') params.set('reason', filter.reason);
  const window = ageWindow(filter.age, now);
  if (window.opened_after) params.set('opened_after', window.opened_after);
  if (window.opened_before) params.set('opened_before', window.opened_before);
  if (cursor) params.set('cursor', cursor);
  const query = params.toString();
  return query ? `/api/queue?${query}` : '/api/queue';
}

export const fetchQueue = (filter: QueueFilter, cursor: string | null) => api.get<QueuePage>(queuePath(filter, cursor, new Date()));

export const fetchAccount = (id: string) => api.get<AccountDetail>(`/api/users/${encodeURIComponent(id)}`);

export const searchUsers = (query: string) =>
  api.get<{ users: UserHit[] }>(`/api/users?q=${encodeURIComponent(query)}`).then((result) => result.users);

export const reviewApi: ReviewApi = { queue: fetchQueue, account: fetchAccount, searchUsers };
