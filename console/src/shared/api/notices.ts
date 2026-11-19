import { api } from './client';
import type { NoticeMailState, NoticeOutcome, Reason } from './decisions';

export const NOTICE_LIMITS = { name: 200, email: 254, content: 2000, location: 1000, explanation: 4000 } as const;

export interface NoticeInput {
  notifier_name: string;
  notifier_email: string;
  content: string;
  location: string;
  explanation: string;
  received_at: string;
  user_id: string;
}

export interface NoticesApi {
  settings(): Promise<{ mail_enabled: boolean }>;
  create(input: NoticeInput): Promise<{ id: string }>;
  decide(caseId: string, outcome: NoticeOutcome, reason: Reason): Promise<{ mail_state: NoticeMailState }>;
}

export const noticesApi: NoticesApi = {
  settings: () => api.get('/api/notices/settings'),
  create: (input) => api.post('/api/notices', input),
  decide: (caseId, outcome, reason) => api.post(`/api/cases/${encodeURIComponent(caseId)}/notice-decision`, { outcome, ...reason }),
};
