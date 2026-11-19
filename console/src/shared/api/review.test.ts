import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchAccount, queuePath, searchUsers } from './review';

const now = new Date('2026-11-19T12:00:00Z');

describe('queuePath', () => {
  it('sends only the chosen filters', () => {
    expect(queuePath({ status: 'any', reason: 'any', age: 'any' }, null, now)).toBe('/api/queue');
    expect(queuePath({ status: 'suspended', reason: 'inappropriate_profile', age: 'older' }, 'abc', now)).toBe(
      '/api/queue?status=suspended&reason=inappropriate_profile&opened_before=2026-11-12T12%3A00%3A00.000Z&cursor=abc',
    );
  });
});

describe('review requests', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('escapes the account ID and the search text', async () => {
    const fetch = vi.fn((input: RequestInfo | URL) =>
      Promise.resolve(new Response(String(input).includes('?q=') ? '{"users":[]}' : '{}', { status: 200 })),
    );
    vi.stubGlobal('fetch', fetch);
    await fetchAccount('4/../x');
    expect(await searchUsers('a&b c')).toEqual([]);
    expect(fetch.mock.calls.map(([input]) => String(input))).toEqual(['/api/users/4%2F..%2Fx', '/api/users?q=a%26b%20c']);
  });
});
