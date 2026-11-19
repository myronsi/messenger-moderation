import { api } from './client';
import type { Reason } from './decisions';

export type ContentReportKind = 'message' | 'group';
export type ContentReportReason = 'spam' | 'scam' | 'harassment' | 'impersonation' | 'inappropriate_profile' | 'illegal' | 'other';

export interface ContentPerson {
  id: string;
  username: string;
  display_name: string;
}

export interface ContentDecision {
  action: 'removed' | 'no_violation';
  staff_id: string;
  staff_username: string;
  reason_code: string | null;
  reason_text: string | null;
  decided_at: string;
}

export interface ContentReportSummary {
  id: string;
  kind: ContentReportKind;
  chat_id: string;
  message_id: string;
  reason: ContentReportReason;
  note: string | null;
  reporter: ContentPerson | null;
  target: ContentPerson | null;
  chat_title: string | null;
  excerpt: string;
  created_at: string;
  withdrawn_at: string | null;
  reports_on_item: number;
  decision: ContentDecision | null;
}

export interface CopiedMessage {
  id: string;
  sender_id: string | null;
  sender_name: string;
  sender_username?: string;
  type: string;
  text: string | null;
  files: string[];
  forwarded_from_name?: string | null;
  reply_to?: string | null;
  sent_at: string;
  edited_at?: string | null;
  reported: boolean;
}

export interface CopiedGroup {
  title: string;
  description: string;
  avatar_attachment_id: string | null;
  created_by: string | null;
  owner_id: string | null;
  member_count: number;
}

export interface ContentCopy {
  chat: { id: string; type: string; title?: string | null };
  messages?: CopiedMessage[];
  group?: CopiedGroup;
}

export interface ContentReportDetail extends ContentReportSummary {
  copy: ContentCopy;
  other_reports: ContentReportSummary[];
}

export interface RemovalResult {
  already_removed: boolean;
}

const report = (id: string) => `/api/content-reports/${encodeURIComponent(id)}`;

export interface ContentApi {
  list(state: 'open' | 'decided'): Promise<ContentReportSummary[]>;
  get(id: string): Promise<ContentReportDetail>;
  remove(id: string, reason: Reason): Promise<RemovalResult>;
  dismiss(id: string, note: string): Promise<void>;
}

export const contentApi: ContentApi = {
  list: (state) => api.get<{ reports: ContentReportSummary[] }>(`/api/content-reports?state=${state}`).then((result) => result.reports),
  get: (id) => api.get<ContentReportDetail>(report(id)),
  remove: (id, reason) => api.post(`${report(id)}/remove`, reason),
  dismiss: (id, note) => api.post(`${report(id)}/dismiss`, { note }),
};
