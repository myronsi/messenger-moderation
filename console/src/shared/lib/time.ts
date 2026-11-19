const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export function formatAge(from: string, now: Date): string {
  const ms = Math.max(0, now.getTime() - new Date(from).getTime());
  if (ms < HOUR) return `${Math.max(1, Math.floor(ms / MINUTE))} min`;
  if (ms < DAY) return `${Math.floor(ms / HOUR)} h`;
  return `${Math.floor(ms / DAY)} d`;
}

const dateTime = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'short' });
const dateOnly = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium' });

export const formatDateTime = (value: string) => dateTime.format(new Date(value));

export const formatDate = (value: string) => dateOnly.format(new Date(value));

export interface AgeWindow {
  opened_after?: string;
  opened_before?: string;
}

export function ageWindow(age: 'any' | 'day' | 'week' | 'older', now: Date): AgeWindow {
  const at = (ms: number) => new Date(now.getTime() - ms).toISOString();
  switch (age) {
    case 'day':
      return { opened_after: at(DAY) };
    case 'week':
      return { opened_after: at(7 * DAY), opened_before: at(DAY) };
    case 'older':
      return { opened_before: at(7 * DAY) };
    default:
      return {};
  }
}
