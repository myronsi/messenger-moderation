import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { AccountDetail } from '@/shared/api/review';
import { ApiError } from '@/shared/api/client';
import { account, NOW, report } from '@/test/moderationFixtures';
import AccountPage from './AccountPage';
import { buildTimeline } from './model/timeline';

const setup = (load: (id: string) => Promise<AccountDetail>) => {
  const api = { account: vi.fn(load) };
  render(
    <MemoryRouter>
      <AccountPage api={api} userId="101" accountHref={(id) => `/accounts/${id}`} queueHref="/queue" now={() => NOW} />
    </MemoryRouter>,
  );
  return api;
};

describe('AccountPage', () => {
  it('shows the header with the current status', async () => {
    const api = setup(async () => account());
    const header = await screen.findByRole('banner');
    expect(within(header).getByRole('heading', { name: 'Dave' })).toBeInTheDocument();
    expect(within(header).getByText('@dave · ID 101')).toBeInTheDocument();
    expect(within(header).getByText('Warned')).toBeInTheDocument();
    expect(within(header).getByText('3.5')).toBeInTheDocument();
    expect(within(header).getByText('49 d')).toBeInTheDocument();
    expect(api.account).toHaveBeenCalledWith('101');
  });

  it('lists reports and status changes newest first', async () => {
    setup(async () => account());
    const timeline = await screen.findByRole('list', { name: 'Timeline' });
    const entries = within(timeline).getAllByRole('article');
    expect(entries.map((entry) => entry.getAttribute('aria-label'))).toEqual(['Report: Spam', 'Status change', 'Report: Scam']);

    const spam = within(entries[0]);
    expect(spam.getByRole('link', { name: '@alice' })).toHaveAttribute('href', '/accounts/201');
    expect(spam.getByText('Sends links to a shop')).toBeInTheDocument();
    expect(spam.getByText('They contacted the reporter first')).toBeInTheDocument();
    expect(spam.getByText('Dave Deals')).toBeInTheDocument();
    expect(spam.getByText('Cheap watches, DM me')).toBeInTheDocument();
    expect(spam.getAllByText('changed since')).toHaveLength(2);

    const scam = within(entries[2]);
    expect(scam.getByText('0 (does not count)')).toBeInTheDocument();
    expect(scam.getByText('Limited')).toBeInTheDocument();
    expect(scam.getByText('12 h old when reporting')).toBeInTheDocument();
    expect(scam.queryByText('changed since')).not.toBeInTheDocument();

    expect(within(entries[1]).getByText('New reports')).toBeInTheDocument();
  });

  it('marks withdrawn reports and counts only active ones', async () => {
    setup(async () => account({ reports: [report(), report({ reporter: { ...report().reporter, id: '9', username: 'zed' }, withdrawn_at: '2026-11-19T11:00:00Z' })] }));
    expect(await screen.findByText('Withdrawn')).toBeInTheDocument();
    const summary = screen.getByRole('region', { name: 'Reports' });
    expect(summary).toHaveTextContent('1 active, 1 withdrawn');
    expect(summary).toHaveTextContent('Spam: 1');
  });

  it('summarises appeals and probation', async () => {
    setup(async () =>
      account({
        status: {
          status: 'limited',
          score: 7,
          since: '2026-11-17T10:00:00Z',
          probation_until: '2026-12-01T10:00:00Z',
          appeal_blocked_until: null,
          last_appeal_at: '2026-11-17T10:00:00Z',
        },
        history: [
          { id: '8', from_status: 'restricted', to_status: 'limited', score: 7, cause: 'appeal', created_at: '2026-11-17T10:00:00Z' },
          { id: '7', from_status: 'limited', to_status: 'restricted', score: 11, cause: 'reports', created_at: '2026-11-10T10:00:00Z' },
        ],
      }),
    );
    const appeals = await screen.findByRole('region', { name: 'Appeals' });
    expect(appeals).toHaveTextContent('On probation until 1 Dec 2026');
    expect(within(appeals).getAllByRole('listitem')).toHaveLength(2);
    expect(appeals).toHaveTextContent('Appeal by the user');
  });

  it('says when the account does not exist', async () => {
    setup(async () => {
      throw new ApiError(404, 'not_found', 'no account with this ID');
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('No account with ID 101');
  });

  it('retries after an error', async () => {
    const user = userEvent.setup();
    let fail = true;
    setup(async () => {
      if (fail) throw new ApiError(500, 'internal_error', 'something went wrong');
      return account();
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('The account could not be loaded.');
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('heading', { name: 'Dave' })).toBeInTheDocument();
  });
});

describe('buildTimeline', () => {
  it('puts a status change before the report made at the same moment', () => {
    const at = '2026-11-19T10:00:00Z';
    const entries = buildTimeline({
      reports: [report({ created_at: at })],
      history: [{ id: '1', from_status: 'good', to_status: 'warned', score: 3, cause: 'reports', created_at: at }],
    });
    expect(entries.map((entry) => entry.kind)).toEqual(['status', 'report']);
  });
});
