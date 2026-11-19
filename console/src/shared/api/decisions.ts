import { api } from './client';
import type { AccountStatus, UserSummary } from './review';

export const DECISION_RULES = ['spam', 'scam', 'harassment', 'impersonation', 'inappropriate_profile', 'illegal', 'other'] as const;
export type DecisionRule = (typeof DECISION_RULES)[number];

export const MAX_REASON_TEXT = 1000;

export interface Reason {
  reason_code: DecisionRule;
  reason_text: string;
}

export interface StatusView {
  status: AccountStatus;
  decided_by: 'automatic' | 'staff';
  reason_code: DecisionRule | null;
  reason_text: string | null;
}

export interface StatusChangeResult {
  before: StatusView;
  after: StatusView;
}

export interface DismissResult extends StatusChangeResult {
  dismissed: number;
}

export type PackKind = 'sticker_pack' | 'emoji_pack';
export type CaseState = 'open' | 'confirmed' | 'reversed' | 'superseded';

export interface CaseSubject {
  id: string;
  username?: string;
  display_name?: string;
  title?: string;
  status?: AccountStatus;
  hidden?: boolean;
}

export interface ModerationCase {
  id: string;
  kind: 'account_status' | 'pack_hidden';
  user_id?: string;
  pack_kind?: PackKind;
  pack_id?: string;
  from_status?: AccountStatus;
  to_status?: AccountStatus;
  opened_at: string;
  deadline: string;
  state: CaseState;
  decided_at: string | null;
  decided_by: string | null;
  reason_code: DecisionRule | null;
  reason_text: string | null;
  subject?: CaseSubject | null;
}

export interface PackItem {
  id: string;
  attachment_id: string;
  label: string;
  animated: boolean;
}

export interface PackReport {
  reporter_id: string;
  reporter_username: string;
  reason: string;
  created_at: string;
}

export interface PackDetail {
  kind: PackKind;
  id: string;
  title: string;
  slug: string | null;
  owner: UserSummary | null;
  group_pack: boolean;
  builtin: boolean;
  hidden_at: string | null;
  hidden_by: 'reports' | 'staff' | null;
  deleted_at: string | null;
  created_at: string;
  items: PackItem[];
  reports: PackReport[];
  cases: ModerationCase[];
}

export interface ReportedPack {
  kind: PackKind;
  id: string;
  title: string;
  reports: number;
  hidden_at: string | null;
  hidden_by: 'reports' | 'staff' | null;
  last_report_at: string | null;
}

const user = (id: string) => `/api/users/${encodeURIComponent(id)}`;
const pack = (kind: PackKind, id: string) => `/api/packs/${kind}/${encodeURIComponent(id)}`;

export const packImageUrl = (kind: PackKind, id: string, attachmentId: string) => `${pack(kind, id)}/images/${encodeURIComponent(attachmentId)}`;

export interface DecisionsApi {
  setStatus(userId: string, status: AccountStatus, reason: Reason): Promise<StatusChangeResult>;
  dismissReports(userId: string, reason: Reason, reporterIds?: string[]): Promise<DismissResult>;
  deleteAccount(userId: string, reason: Reason): Promise<void>;
  cases(state: 'open' | 'decided'): Promise<ModerationCase[]>;
  confirmCase(caseId: string, reason: Reason): Promise<void>;
  reverseCase(caseId: string, reason: Reason, status?: AccountStatus): Promise<void>;
  packs(): Promise<ReportedPack[]>;
  pack(kind: PackKind, id: string): Promise<PackDetail>;
  setPackHidden(kind: PackKind, id: string, hidden: boolean, reason: Reason): Promise<{ changed: boolean }>;
}

export const decisionsApi: DecisionsApi = {
  setStatus: (userId, status, reason) => api.post(`${user(userId)}/status`, { status, ...reason }),
  dismissReports: (userId, reason, reporterIds) =>
    api.post(`${user(userId)}/reports/dismiss`, reporterIds ? { ...reason, reporter_ids: reporterIds } : reason),
  deleteAccount: (userId, reason) => api.del(user(userId), reason),
  cases: (state) => api.get<{ cases: ModerationCase[] }>(`/api/cases?state=${state}`).then((result) => result.cases),
  confirmCase: (caseId, reason) => api.post(`/api/cases/${encodeURIComponent(caseId)}/confirm`, reason),
  reverseCase: (caseId, reason, status) =>
    api.post(`/api/cases/${encodeURIComponent(caseId)}/reverse`, status ? { ...reason, status } : reason),
  packs: () => api.get<{ packs: ReportedPack[] }>('/api/packs').then((result) => result.packs),
  pack: (kind, id) => api.get<PackDetail>(pack(kind, id)),
  setPackHidden: (kind, id, hidden, reason) => api.post(`${pack(kind, id)}/${hidden ? 'hide' : 'unhide'}`, reason),
};
