import type { AccountDetail, QueueItem, UserHit, UserReport } from '@/shared/api/review';

export const NOW = new Date('2026-11-19T12:00:00Z');

export const queueItem = (overrides: Partial<QueueItem> & { id?: string; name?: string } = {}): QueueItem => {
  const { id = '101', name = 'dave', ...rest } = overrides;
  return {
    user: { id, username: name, display_name: name[0].toUpperCase() + name.slice(1), created_at: '2026-10-01T08:00:00Z' },
    status: 'warned',
    score: 3.5,
    status_since: '2026-11-18T10:00:00Z',
    open_reports: 2,
    weighted_reports: 2,
    reasons: ['scam', 'spam'],
    newest_report_at: '2026-11-19T10:00:00Z',
    opened_at: '2026-11-16T12:00:00Z',
    ...rest,
  };
};

export const report = (overrides: Partial<UserReport> = {}): UserReport => ({
  reporter: { id: '201', username: 'alice', display_name: 'Alice', created_at: '2025-01-05T09:00:00Z', status: 'good' },
  reason: 'spam',
  note: 'Sends links to a shop',
  context: 'target_initiated',
  weight: 1,
  snapshot: { username: 'dave', display_name: 'Dave Deals', bio: 'Cheap watches, DM me', avatar_attachment_id: null },
  created_at: '2026-11-19T10:00:00Z',
  updated_at: '2026-11-19T10:00:00Z',
  withdrawn_at: null,
  ...overrides,
});

export const account = (overrides: Partial<AccountDetail> = {}): AccountDetail => ({
  user: {
    id: '101',
    username: 'dave',
    display_name: 'Dave',
    bio: null,
    created_at: '2026-10-01T08:00:00Z',
    last_seen_at: '2026-11-19T11:30:00Z',
  },
  status: {
    status: 'warned',
    score: 3.5,
    since: '2026-11-18T10:00:00Z',
    probation_until: null,
    appeal_blocked_until: null,
    last_appeal_at: null,
  },
  history: [
    { id: '7', from_status: 'good', to_status: 'warned', score: 3.5, cause: 'reports', created_at: '2026-11-18T10:00:00Z' },
  ],
  reports: [
    report(),
    report({
      reporter: { id: '202', username: 'bob', display_name: 'Bob', created_at: '2026-11-16T00:00:00Z', status: 'limited' },
      reason: 'scam',
      note: null,
      context: 'none',
      weight: 0,
      snapshot: { username: 'dave', display_name: 'Dave', bio: null },
      created_at: '2026-11-16T12:00:00Z',
      updated_at: '2026-11-16T12:00:00Z',
    }),
  ],
  ...overrides,
});

export const hits: UserHit[] = [
  { id: '101', username: 'dave', display_name: 'Dave', created_at: '2026-10-01T08:00:00Z', status: 'warned' },
  { id: '102', username: 'davina', display_name: 'Davina K', created_at: '2026-09-01T08:00:00Z', status: 'good' },
];
