import type { StatsGroup } from '@/shared/api/client';

export type ValueFormat = 'number' | 'bytes' | 'minutes' | 'duration';

const NUMBER = new Intl.NumberFormat('en-US');

export const formatValue = (value: number | null | undefined, format: ValueFormat = 'number'): string => {
  if (value === null || value === undefined) return '-';
  switch (format) {
    case 'bytes': {
      const units = ['B', 'KB', 'MB', 'GB', 'TB'];
      let v = value;
      let i = 0;
      while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
      }
      return `${i === 0 ? v : v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`;
    }
    case 'minutes':
      return `${NUMBER.format(value)} min`;
    case 'duration': {
      if (value < 60) return `${value} s`;
      if (value < 3600) return `${Math.round(value / 60)} min`;
      const hours = Math.floor(value / 3600);
      const minutes = Math.round((value % 3600) / 60);
      return minutes ? `${hours} h ${minutes} min` : `${hours} h`;
    }
    default:
      return NUMBER.format(value);
  }
};

const DAY = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
const MONTH = new Intl.DateTimeFormat('en-US', { month: 'short', year: 'numeric', timeZone: 'UTC' });

export const formatPeriod = (day: string, group: StatsGroup): string => {
  const date = new Date(`${day}T00:00:00Z`);
  if (group === 'month') return MONTH.format(date);
  if (group === 'week') return `Week of ${DAY.format(date)}`;
  return DAY.format(date);
};

export const formatTick = (day: string, group: StatsGroup): string => {
  const date = new Date(`${day}T00:00:00Z`);
  return group === 'month' ? MONTH.format(date) : DAY.format(date);
};

export const isoDay = (date: Date) => date.toISOString().slice(0, 10);

export const addDays = (day: string, days: number) => {
  const date = new Date(`${day}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return isoDay(date);
};
