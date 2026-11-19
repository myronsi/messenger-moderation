import { describe, expect, it } from 'vitest';
import { ageWindow, formatAge } from './time';

const now = new Date('2026-11-19T12:00:00Z');

describe('formatAge', () => {
  it('uses minutes, hours and days', () => {
    expect(formatAge('2026-11-19T11:59:50Z', now)).toBe('1 min');
    expect(formatAge('2026-11-19T11:15:00Z', now)).toBe('45 min');
    expect(formatAge('2026-11-19T09:00:00Z', now)).toBe('3 h');
    expect(formatAge('2026-11-16T12:00:00Z', now)).toBe('3 d');
  });
});

describe('ageWindow', () => {
  it('turns an age choice into a time range', () => {
    expect(ageWindow('any', now)).toEqual({});
    expect(ageWindow('day', now)).toEqual({ opened_after: '2026-11-18T12:00:00.000Z' });
    expect(ageWindow('week', now)).toEqual({ opened_after: '2026-11-12T12:00:00.000Z', opened_before: '2026-11-18T12:00:00.000Z' });
    expect(ageWindow('older', now)).toEqual({ opened_before: '2026-11-12T12:00:00.000Z' });
  });
});
