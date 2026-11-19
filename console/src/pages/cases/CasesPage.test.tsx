import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { CaseKind, ModerationCase } from '@/shared/api/decisions';
import { NOW } from '@/test/moderationFixtures';
import CasesPage from './CasesPage';
import { deadlineText } from './model/deadline';

const accountCase: ModerationCase = {
  id: '11',
  kind: 'account_status',
  user_id: '101',
  from_status: 'limited',
  to_status: 'suspended',
  opened_at: '2026-11-17T10:00:00Z',
  deadline: '2026-11-20T10:00:00Z',
  state: 'open',
  decided_at: null,
  decided_by: null,
  reason_code: null,
  reason_text: null,
  subject: { id: '101', username: 'dave', display_name: 'Dave', status: 'suspended' },
};

const packCase: ModerationCase = {
  id: '12',
  kind: 'pack_hidden',
  pack_kind: 'emoji_pack',
  pack_id: '4',
  opened_at: '2026-11-16T08:00:00Z',
  deadline: '2026-11-19T08:00:00Z',
  state: 'open',
  decided_at: null,
  decided_by: null,
  reason_code: null,
  reason_text: null,
  subject: { id: '4', title: 'Faces', hidden: true },
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

describe('CasesPage', () => {
  it('lists open cases with their deadlines', async () => {
    setup([packCase, accountCase]);
    const items = within(await screen.findByRole('list', { name: 'Cases' })).getAllByRole('listitem');
    expect(within(items[0]).getByRole('link', { name: 'Emoji pack: Faces' })).toHaveAttribute('href', '/packs/emoji_pack/4');
    expect(within(items[0]).getByTestId('deadline')).toHaveTextContent('Overdue by 4 h 0 min');
    expect(within(items[1]).getByRole('link', { name: /Dave/ })).toHaveAttribute('href', '/users/101');
    expect(within(items[1]).getByText('Limited')).toBeInTheDocument();
    expect(within(items[1]).getByTestId('deadline')).toHaveTextContent('22 h 0 min left');
  });

  it('confirms an account case with a reason', async () => {
    const { api, user } = setup([accountCase]);
    await user.click(await screen.findByRole('button', { name: 'Confirm' }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('The account stays suspended');
    await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'spam');
    await user.type(within(dialog).getByLabelText(/Text for the user/), 'Spam links.');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));
    expect(api.confirmCase).toHaveBeenCalledWith('11', { reason_code: 'spam', reason_text: 'Spam links.' });
    expect(api.cases).toHaveBeenCalledTimes(2);
  });

  it('reverses an account case to a lower status only', async () => {
    const { api, user } = setup([accountCase]);
    await user.click(await screen.findByRole('button', { name: 'Reverse' }));
    const dialog = screen.getByRole('dialog');
    const options = within(within(dialog).getByLabelText('New status')).getAllByRole('option').map((o) => o.textContent);
    expect(options).toEqual(['Good', 'Warned', 'Limited', 'Restricted']);
    await user.selectOptions(within(dialog).getByLabelText('New status'), 'warned');
    await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'other');
    await user.type(within(dialog).getByLabelText(/Text for the user/), 'The reports were a misunderstanding.');
    await user.click(within(dialog).getByRole('button', { name: 'Reverse' }));
    expect(api.reverseCase).toHaveBeenCalledWith('11', { reason_code: 'other', reason_text: 'The reports were a misunderstanding.' }, 'warned');
  });

  it('reverses a pack case without a status', async () => {
    const { api, user } = setup([packCase]);
    await user.click(await screen.findByRole('button', { name: 'Reverse' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByLabelText('New status')).not.toBeInTheDocument();
    await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'other');
    await user.type(within(dialog).getByLabelText(/Text for the user/), 'Fine.');
    await user.click(within(dialog).getByRole('button', { name: 'Reverse' }));
    expect(api.reverseCase).toHaveBeenCalledWith('12', { reason_code: 'other', reason_text: 'Fine.' }, undefined);
  });

  it('shows decided cases with who decided', async () => {
    const { user } = setup([], [{ ...accountCase, state: 'reversed', decided_by: 'helper', decided_at: '2026-11-18T09:00:00Z', reason_code: 'other' }]);
    expect(await screen.findByText('No open cases.')).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Decided' }));
    expect(await screen.findByText(/Reversed by helper on .* · Other/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument();
  });

  it('words the time left', () => {
    expect(deadlineText('2026-11-19T12:30:00Z', NOW)).toEqual({ text: '30 min left', overdue: false, soon: true });
    expect(deadlineText('2026-11-21T12:00:00Z', NOW)).toEqual({ text: '48 h 0 min left', overdue: false, soon: false });
  });
});
