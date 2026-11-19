import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { CaseKind, ModerationCase } from '@/shared/api/decisions';
import { NOW } from '@/test/moderationFixtures';
import CasesPage from './CasesPage';

const base = { state: 'open' as const, decided_at: null, decided_by: null, reason_code: null, reason_text: null };

const requestCase: ModerationCase = {
  ...base,
  id: '21',
  kind: 'review_request',
  user_id: '102',
  to_status: 'restricted',
  request_text: 'I only sent one link to a friend.',
  opened_at: '2026-11-19T08:00:00Z',
  deadline: '2026-11-22T08:00:00Z',
  subject: { id: '102', username: 'erin', display_name: 'Erin', status: 'limited' },
};

const noticeCase: ModerationCase = {
  ...base,
  id: '31',
  kind: 'notice',
  user_id: '103',
  opened_at: '2026-11-19T06:00:00Z',
  deadline: '2026-11-22T06:00:00Z',
  subject: { id: '103', username: 'frank', display_name: 'Frank', status: 'good' },
  notice: {
    notifier_name: 'Jane Doe',
    notifier_email: 'jane@example.com',
    content: 'A message calling for violence',
    location: "Group 'Town news'",
    explanation: 'It incites violence against a named person.',
    received_at: '2026-11-19T06:00:00Z',
    entered_by: 'helper',
    outcome: null,
    mail_state: 'none',
    mail_sent_at: null,
  },
};

const setup = (open: ModerationCase[], decided: ModerationCase[] = [], mailEnabled = true) => {
  const api = {
    cases: vi.fn(async (state: 'open' | 'decided', _kind?: CaseKind) => (state === 'open' ? open : decided)),
    confirmCase: vi.fn(async () => undefined),
    reverseCase: vi.fn(async () => undefined),
  };
  const notices = {
    settings: vi.fn(async () => ({ mail_enabled: mailEnabled })),
    create: vi.fn(async () => ({ id: '99' })),
    decide: vi.fn(async () => ({ mail_state: 'pending' as const })),
  };
  render(
    <MemoryRouter>
      <CasesPage api={api} notices={notices} accountHref={(id) => `/users/${id}`} packHref={(kind, id) => `/packs/${kind}/${id}`} now={() => NOW} />
    </MemoryRouter>,
  );
  return { api, notices, user: userEvent.setup() };
};

const fillReason = async (user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement, label: RegExp, text: string) => {
  await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'other');
  await user.type(within(dialog).getByLabelText(label), text);
};

describe('CasesPage sources', () => {
  it('shows a review request with the user text and keeps the current status', async () => {
    const { api, user } = setup([requestCase]);
    const item = within(await screen.findByRole('list', { name: 'Cases' })).getByRole('listitem');
    expect(item).toHaveTextContent('User asked for a review');
    expect(item).toHaveTextContent('I only sent one link to a friend.');
    expect(within(item).getByText('Restricted')).toBeInTheDocument();
    expect(within(item).getByText('Limited')).toBeInTheDocument();
    await user.click(within(item).getByRole('button', { name: 'Keep status' }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('The account stays limited');
    await fillReason(user, dialog, /Text for the user/, 'The reports were justified.');
    await user.click(within(dialog).getByRole('button', { name: 'Keep status' }));
    expect(api.confirmCase).toHaveBeenCalledWith('21', { reason_code: 'other', reason_text: 'The reports were justified.' });
  });

  it('offers only statuses below the current one when lowering after a review', async () => {
    const { api, user } = setup([requestCase]);
    await user.click(await screen.findByRole('button', { name: 'Lower status' }));
    const dialog = screen.getByRole('dialog');
    const options = within(within(dialog).getByLabelText('New status')).getAllByRole('option').map((o) => o.textContent);
    expect(options).toEqual(['Good', 'Warned']);
    await fillReason(user, dialog, /Text for the user/, 'We checked the chats.');
    await user.click(within(dialog).getByRole('button', { name: 'Lower status' }));
    expect(api.reverseCase).toHaveBeenCalledWith('21', { reason_code: 'other', reason_text: 'We checked the chats.' }, 'good');
  });

  it('decides a notice with an outcome and a text for the notifier', async () => {
    const { notices, user } = setup([noticeCase]);
    const item = within(await screen.findByRole('list', { name: 'Cases' })).getByRole('listitem');
    expect(item).toHaveTextContent('Notice by email');
    expect(item).toHaveTextContent('Jane Doe · jane@example.com');
    expect(item).toHaveTextContent('It incites violence against a named person.');
    expect(within(item).getByRole('link', { name: /Frank/ })).toHaveAttribute('href', '/users/103');
    expect(within(item).queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument();
    await user.click(within(item).getByRole('button', { name: 'Decide' }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('The notifier gets the outcome, the rule and your text by email.');
    expect(dialog).toHaveTextContent('Act on the account from its page first');
    await user.selectOptions(within(dialog).getByLabelText('Outcome'), 'no_violation');
    await fillReason(user, dialog, /Text for the notifier/, 'The message quotes a news report.');
    await user.click(within(dialog).getByRole('button', { name: 'Record decision' }));
    expect(notices.decide).toHaveBeenCalledWith('31', 'no_violation', { reason_code: 'other', reason_text: 'The message quotes a news report.' });
  });

  it('shows the outcome and the email state of a decided notice', async () => {
    const decided = { ...noticeCase, state: 'reversed' as const, decided_by: 'helper', decided_at: '2026-11-19T10:00:00Z', reason_code: 'other' as const };
    const { user } = setup([], [{ ...decided, notice: { ...noticeCase.notice!, outcome: 'no_violation', mail_state: 'skipped' } }]);
    await user.click(await screen.findByRole('tab', { name: 'Decided' }));
    expect(await screen.findByText(/No violation by helper/)).toBeInTheDocument();
    expect(screen.getByText('Email is not set up; reply to the notifier by hand')).toBeInTheDocument();
  });

  it('filters by source', async () => {
    const { api, user } = setup([requestCase, noticeCase]);
    await screen.findByRole('list', { name: 'Cases' });
    expect(api.cases).toHaveBeenLastCalledWith('open', undefined);
    await user.selectOptions(screen.getByLabelText('Source'), 'notice');
    expect(api.cases).toHaveBeenLastCalledWith('open', 'notice');
  });

  it('adds a notice that arrived by email', async () => {
    const { api, notices, user } = setup([], [], false);
    await screen.findByText('No open cases.');
    await user.click(screen.getByRole('button', { name: 'Add notice' }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('Email is not set up here');
    const save = within(dialog).getByRole('button', { name: 'Add notice' });
    expect(save).toBeDisabled();
    await user.type(within(dialog).getByLabelText('Who sent it'), 'Jane Doe');
    await user.type(within(dialog).getByLabelText('Email for the reply (optional)'), 'jane@example.com');
    await user.type(within(dialog).getByLabelText('What was reported'), 'A threat');
    await user.type(within(dialog).getByLabelText('Where it is'), '@frank, message of 18 November');
    await user.type(within(dialog).getByLabelText(/Why it is illegal/), 'It threatens a named person.');
    await user.type(within(dialog).getByLabelText('Account ID (optional)'), '103');
    await user.click(save);
    expect(notices.create).toHaveBeenCalledWith(
      expect.objectContaining({
        notifier_name: 'Jane Doe',
        notifier_email: 'jane@example.com',
        content: 'A threat',
        location: '@frank, message of 18 November',
        explanation: 'It threatens a named person.',
        user_id: '103',
        received_at: expect.stringMatching(/^2026-11-19T/),
      }),
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(api.cases).toHaveBeenLastCalledWith('open', 'notice');
  });
});
