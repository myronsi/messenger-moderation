import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { QueueFilter, QueuePage as Page } from '@/shared/api/review';
import { NOW, queueItem } from '@/test/moderationFixtures';
import QueuePage from './QueuePage';
import { filterFromSearch, filterToSearch } from './model/filter';

const accountHref = (id: string) => `/accounts/${id}`;

const setup = (queue: (filter: QueueFilter, cursor: string | null) => Promise<Page>, onFilterChange?: (f: QueueFilter) => void) => {
  const api = { queue: vi.fn(queue) };
  render(
    <MemoryRouter>
      <QueuePage api={api} accountHref={accountHref} now={() => NOW} onFilterChange={onFilterChange} />
    </MemoryRouter>,
  );
  return api;
};

describe('QueuePage', () => {
  it('lists accounts with status, reports and how long they wait', async () => {
    setup(async () => ({
      items: [
        queueItem({ id: '7', name: 'grace', status: 'suspended', score: 16, open_reports: 0, weighted_reports: 0, reasons: [], newest_report_at: null, opened_at: '2026-11-14T12:00:00Z' }),
        queueItem({ weighted_reports: 1 }),
      ],
      next_cursor: null,
    }));

    const rows = await screen.findAllByRole('row');
    expect(rows).toHaveLength(3);
    const grace = within(rows[1]);
    expect(grace.getByRole('link', { name: 'Grace' })).toHaveAttribute('href', '/accounts/7');
    expect(grace.getByText('Suspended')).toBeInTheDocument();
    expect(grace.getByText('5 d')).toBeInTheDocument();
    expect(grace.getByText('No new reports')).toBeInTheDocument();

    const dave = within(rows[2]);
    expect(dave.getByText('Scam, Spam')).toBeInTheDocument();
    expect(dave.getByText('1 weighted')).toBeInTheDocument();
    expect(dave.getByText('2 h ago')).toBeInTheDocument();
    expect(dave.getByText('3.5')).toBeInTheDocument();
  });

  it('reloads with the chosen filters and can clear them', async () => {
    const user = userEvent.setup();
    const changes = vi.fn();
    const api = setup(async () => ({ items: [queueItem()], next_cursor: null }), changes);
    await screen.findByRole('link', { name: 'Dave' });

    await user.selectOptions(screen.getByLabelText('Status'), 'restricted');
    await user.selectOptions(screen.getByLabelText('Reason'), 'harassment');
    await user.selectOptions(screen.getByLabelText('Age'), 'older');

    expect(api.queue).toHaveBeenLastCalledWith({ status: 'restricted', reason: 'harassment', age: 'older' }, null);
    expect(changes).toHaveBeenLastCalledWith({ status: 'restricted', reason: 'harassment', age: 'older' });

    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    expect(api.queue).toHaveBeenLastCalledWith({ status: 'any', reason: 'any', age: 'any' }, null);
    expect(screen.queryByRole('button', { name: 'Clear filters' })).not.toBeInTheDocument();
  });

  it('loads the next page with the cursor', async () => {
    const user = userEvent.setup();
    const api = setup(async (_filter, cursor) =>
      cursor ? { items: [queueItem({ id: '2', name: 'erin' })], next_cursor: null } : { items: [queueItem()], next_cursor: 'c1' },
    );
    await user.click(await screen.findByRole('button', { name: 'Load more' }));

    expect(await screen.findByRole('link', { name: 'Erin' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Dave' })).toBeInTheDocument();
    expect(api.queue).toHaveBeenLastCalledWith({ status: 'any', reason: 'any', age: 'any' }, 'c1');
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();
  });

  it('says when nothing waits', async () => {
    setup(async () => ({ items: [], next_cursor: null }));
    expect(await screen.findByText('Nothing to review with these filters.')).toBeInTheDocument();
  });

  it('shows an error and retries', async () => {
    const user = userEvent.setup();
    let fail = true;
    setup(async () => {
      if (fail) throw new Error('down');
      return { items: [queueItem()], next_cursor: null };
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('The queue could not be loaded.');
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('link', { name: 'Dave' })).toBeInTheDocument();
  });
});

describe('queue filter in the address', () => {
  it('reads known values and ignores the rest', () => {
    expect(filterFromSearch('?status=suspended&reason=spam&age=day')).toEqual({ status: 'suspended', reason: 'spam', age: 'day' });
    expect(filterFromSearch('?status=bogus&age=year')).toEqual({ status: 'any', reason: 'any', age: 'any' });
  });

  it('writes only chosen values', () => {
    expect(filterToSearch({ status: 'any', reason: 'scam', age: 'any' })).toBe('?reason=scam');
    expect(filterToSearch({ status: 'any', reason: 'any', age: 'any' })).toBe('');
  });
});
