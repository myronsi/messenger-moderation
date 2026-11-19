import { ACCOUNT_STATUSES, QUEUE_AGES, REPORT_REASONS, type QueueFilter } from '@/shared/api/review';

export const DEFAULT_FILTER: QueueFilter = { status: 'any', reason: 'any', age: 'any' };

export const isDefaultFilter = (filter: QueueFilter) =>
  filter.status === DEFAULT_FILTER.status && filter.reason === DEFAULT_FILTER.reason && filter.age === DEFAULT_FILTER.age;

const pick = <T extends string>(allowed: readonly T[], value: string | null, fallback: T): T =>
  value !== null && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;

export function filterFromSearch(search: string): QueueFilter {
  const params = new URLSearchParams(search);
  return {
    status: pick(['any', ...ACCOUNT_STATUSES], params.get('status'), 'any'),
    reason: pick(['any', ...REPORT_REASONS], params.get('reason'), 'any'),
    age: pick(QUEUE_AGES, params.get('age'), 'any'),
  };
}

export function filterToSearch(filter: QueueFilter): string {
  const params = new URLSearchParams();
  if (filter.status !== 'any') params.set('status', filter.status);
  if (filter.reason !== 'any') params.set('reason', filter.reason);
  if (filter.age !== 'any') params.set('age', filter.age);
  const query = params.toString();
  return query ? `?${query}` : '';
}
