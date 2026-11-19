import type { AccountDetail, StatusChange, UserReport } from '@/shared/api/review';

export type TimelineEntry =
  | { kind: 'status'; key: string; at: string; change: StatusChange }
  | { kind: 'report'; key: string; at: string; report: UserReport };

const order = { status: 0, report: 1 } as const;

export function buildTimeline(detail: Pick<AccountDetail, 'history' | 'reports'>): TimelineEntry[] {
  const entries: TimelineEntry[] = [
    ...detail.history.map((change) => ({ kind: 'status' as const, key: `status-${change.id}`, at: change.created_at, change })),
    ...detail.reports.map((report) => ({ kind: 'report' as const, key: `report-${report.reporter.id}`, at: report.created_at, report })),
  ];
  return entries.sort((a, b) => {
    const diff = new Date(b.at).getTime() - new Date(a.at).getTime();
    return diff !== 0 ? diff : order[a.kind] - order[b.kind];
  });
}

export interface SnapshotDiff {
  displayName: boolean;
  username: boolean;
  bio: boolean;
}

export function snapshotDiff(report: UserReport, current: { username: string; display_name: string; bio: string | null }): SnapshotDiff {
  const { snapshot } = report;
  return {
    displayName: snapshot.display_name !== undefined && snapshot.display_name !== current.display_name,
    username: snapshot.username !== undefined && snapshot.username !== current.username,
    bio: snapshot.bio !== undefined && (snapshot.bio ?? null) !== current.bio,
  };
}
