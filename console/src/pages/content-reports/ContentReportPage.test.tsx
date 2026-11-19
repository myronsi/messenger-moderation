import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ApiError } from '@/shared/api/client';
import type { ContentReportDetail, ContentReportSummary } from '@/shared/api/content';
import { ContentReportPage } from './ContentReportPage';
import { ContentReportsPage } from './ContentReportsPage';

const base: ContentReportSummary = {
  id: '10',
  kind: 'message',
  chat_id: '70',
  message_id: '900',
  reason: 'illegal',
  note: 'Sells prescription drugs.',
  reporter: { id: '1', username: 'reporter', display_name: 'Reporter' },
  target: { id: '2', username: 'spammer', display_name: 'Spammer' },
  chat_title: 'Shop',
  excerpt: 'cheap pills',
  created_at: '2026-11-19T10:05:00Z',
  withdrawn_at: null,
  reports_on_item: 2,
  decision: null,
};

const detail: ContentReportDetail = {
  ...base,
  copy: {
    chat: { id: '70', type: 'group', title: 'Shop' },
    messages: [
      { id: '899', sender_id: '1', sender_name: 'Reporter', type: 'text', text: 'how much?', files: [], sent_at: '2026-11-19T10:00:00Z', reported: false },
      { id: '900', sender_id: '2', sender_name: 'Spammer', type: 'file', text: 'cheap pills', files: ['offer.pdf'], sent_at: '2026-11-19T10:01:00Z', reported: true },
      { id: '901', sender_id: '1', sender_name: 'Reporter', type: 'text', text: 'no thanks', files: [], sent_at: '2026-11-19T10:02:00Z', reported: false },
    ],
  },
  other_reports: [{ ...base, id: '11', reason: 'spam', note: null, reporter: { id: '3', username: 'second', display_name: 'Second' } }],
};

const setup = (get: () => Promise<ContentReportDetail>) => {
  const api = { get: vi.fn(get), remove: vi.fn(async () => ({ already_removed: false })), dismiss: vi.fn(async () => undefined) };
  render(
    <MemoryRouter>
      <ContentReportPage id="10" api={api} accountHref={(id) => `/users/${id}`} reportHref={(id) => `/content-reports/${id}`} listHref="/content-reports" />
    </MemoryRouter>,
  );
  return { api, user: userEvent.setup() };
};

describe('ContentReportPage', () => {
  it('shows the copy with the reported message highlighted, the notice and all reports', async () => {
    setup(async () => detail);
    expect(await screen.findByRole('heading', { name: 'Illegal content' })).toBeInTheDocument();
    expect(screen.getByText(/Notice of illegal content/)).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Copied messages' })).toBeInTheDocument();
    const reported = screen.getByTestId('reported-message');
    expect(reported).toHaveAttribute('aria-current', 'true');
    expect(reported).toHaveTextContent('cheap pills');
    expect(reported).toHaveTextContent('offer.pdf');
    expect(reported).toHaveTextContent('Reported');
    expect(screen.getAllByTestId('context-message')).toHaveLength(2);
    expect(screen.getByText('Sells prescription drugs.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '@second' })).toHaveAttribute('href', '/users/3');
    expect(screen.getByRole('link', { name: '@spammer' })).toHaveAttribute('href', '/users/2');
    expect(screen.getByTestId('decision-state')).toHaveTextContent('Open');
  });

  it('removes the message with a rule and a text for the sender', async () => {
    const { api, user } = setup(async () => detail);
    await user.click(await screen.findByRole('button', { name: 'Remove message' }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('Removed by moderation');
    await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'illegal');
    await user.type(within(dialog).getByLabelText(/Text for the user/), 'Selling prescription drugs is not allowed.');
    await user.click(within(dialog).getByRole('button', { name: 'Remove message' }));
    expect(api.remove).toHaveBeenCalledWith('10', { reason_code: 'illegal', reason_text: 'Selling prescription drugs is not allowed.' });
    expect(api.get).toHaveBeenCalledTimes(2);
  });

  it('closes the reports without action and an optional note', async () => {
    const { api, user } = setup(async () => detail);
    await user.click(await screen.findByRole('button', { name: 'No violation' }));
    const dialog = screen.getByRole('dialog');
    await user.type(within(dialog).getByLabelText(/Internal note/), 'Price question, no drugs.');
    await user.click(within(dialog).getByRole('button', { name: 'No violation' }));
    expect(api.dismiss).toHaveBeenCalledWith('10', 'Price question, no drugs.');
  });

  it('offers no removal for a group report and shows the group copy', async () => {
    setup(async () => ({
      ...detail,
      kind: 'group',
      reason: 'scam',
      copy: { chat: { id: '71', type: 'group', title: 'Free money' }, group: { title: 'Free money', description: 'Send us your card', avatar_attachment_id: null, created_by: '2', owner_id: '2', member_count: 3 } },
      other_reports: [],
    }));
    expect(await screen.findByText('Send us your card')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Remove message' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'No violation' })).toBeInTheDocument();
  });

  it('shows a decided report without actions except removal after no violation', async () => {
    setup(async () => ({ ...detail, decision: { action: 'removed', staff_id: '4', staff_username: 'helper', reason_code: 'illegal', reason_text: 'Not allowed.', decided_at: '2026-11-19T11:00:00Z' } }));
    expect(await screen.findByTestId('decision-state')).toHaveTextContent('Removed by helper');
    expect(screen.queryByRole('button', { name: 'Remove message' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'No violation' })).not.toBeInTheDocument();
    expect(screen.getByText('Not allowed.')).toBeInTheDocument();
  });

  it('explains a report that is gone', async () => {
    setup(async () => Promise.reject(new ApiError(404, 'not_found', 'not found')));
    expect(await screen.findByRole('alert')).toHaveTextContent('deleted 6 months after');
  });
});

describe('ContentReportsPage', () => {
  it('lists open items and switches to decided ones', async () => {
    const api = { list: vi.fn(async (state: 'open' | 'decided') => (state === 'open' ? [base] : [])) };
    render(
      <MemoryRouter>
        <ContentReportsPage api={api} reportHref={(id) => `/content-reports/${id}`} />
      </MemoryRouter>,
    );
    const link = await screen.findByRole('link', { name: 'Message: Illegal content' });
    expect(link).toHaveAttribute('href', '/content-reports/10');
    expect(screen.getByText(/2 reports/)).toBeInTheDocument();
    expect(screen.getByText(/@spammer in Shop: cheap pills/)).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole('tab', { name: 'Decided' }));
    expect(await screen.findByText('No decided content reports.')).toBeInTheDocument();
    expect(api.list).toHaveBeenLastCalledWith('decided');
  });
});
