import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { AccountDetail } from '@/shared/api/review';
import { account, NOW } from '@/test/moderationFixtures';
import AccountPage from './AccountPage';

const setup = (detail: AccountDetail, isAdmin = false) => {
  const api = { account: vi.fn(async () => detail) };
  const view = { status: detail.status.status, decided_by: 'automatic' as const, reason_code: null, reason_text: null };
  const decisions = {
    setStatus: vi.fn(async () => ({ before: view, after: view })),
    dismissReports: vi.fn(async () => ({ dismissed: 1, before: view, after: view })),
    deleteAccount: vi.fn(async () => undefined),
  };
  render(
    <MemoryRouter initialEntries={['/users/101']}>
      <Routes>
        <Route
          path="/users/:id"
          element={<AccountPage api={api} decisions={decisions} isAdmin={isAdmin} userId="101" accountHref={(id) => `/users/${id}`} queueHref="/queue" now={() => NOW} />}
        />
        <Route path="/queue" element={<p>Queue page</p>} />
      </Routes>
    </MemoryRouter>,
  );
  return { api, decisions, user: userEvent.setup() };
};

const reasonFor = async (user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement, rule = 'spam', text = 'Sent the same link to many people.') => {
  await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), rule);
  await user.type(within(dialog).getByLabelText(/Text for the user/), text);
};

describe('Account decisions', () => {
  it('sets a light status and reloads the account', async () => {
    const { api, decisions, user } = setup(account());
    await user.click(await screen.findByRole('button', { name: 'Set status' }));
    const dialog = screen.getByRole('dialog', { name: 'Set the status of @dave' });
    await user.selectOptions(within(dialog).getByLabelText('New status'), 'limited');
    await reasonFor(user, dialog);
    expect(within(dialog).queryByLabelText(/to confirm/)).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Save status' }));
    expect(decisions.setStatus).toHaveBeenCalledWith('101', 'limited', { reason_code: 'spam', reason_text: 'Sent the same link to many people.' });
    expect(await screen.findByRole('button', { name: 'Set status' })).toBeInTheDocument();
    expect(api.account).toHaveBeenCalledTimes(2);
  });

  it('asks to type the username before suspending', async () => {
    const { decisions, user } = setup(account());
    await user.click(await screen.findByRole('button', { name: 'Set status' }));
    const dialog = screen.getByRole('dialog');
    await user.selectOptions(within(dialog).getByLabelText('New status'), 'suspended');
    await reasonFor(user, dialog, 'scam', 'Asked people for money.');
    const submit = within(dialog).getByRole('button', { name: 'Suspended the account' });
    expect(submit).toBeDisabled();
    await user.type(within(dialog).getByLabelText(/to confirm/), 'dave');
    await user.click(submit);
    expect(decisions.setStatus).toHaveBeenCalledWith('101', 'suspended', { reason_code: 'scam', reason_text: 'Asked people for money.' });
  });

  it('dismisses all reports or only the ticked ones', async () => {
    const { decisions, user } = setup(account());
    await user.click(await screen.findByRole('button', { name: 'Dismiss reports' }));
    let dialog = screen.getByRole('dialog');
    await reasonFor(user, dialog, 'other', 'No violation found.');
    await user.click(within(dialog).getByRole('button', { name: 'Dismiss all 2' }));
    expect(decisions.dismissReports).toHaveBeenLastCalledWith('101', { reason_code: 'other', reason_text: 'No violation found.' }, undefined);

    await user.click(await screen.findByRole('button', { name: 'Dismiss reports' }));
    dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByLabelText('@bob'));
    await reasonFor(user, dialog, 'other', 'No violation found.');
    await user.click(within(dialog).getByRole('button', { name: 'Dismiss 1' }));
    expect(decisions.dismissReports).toHaveBeenLastCalledWith('101', { reason_code: 'other', reason_text: 'No violation found.' }, ['202']);
  });

  it('shows the current staff decision and counts dismissed reports apart', async () => {
    const detail = account();
    detail.status = { ...detail.status, decided_by: 'staff', reason_code: 'impersonation', reason_text: 'Pretends to be a bank.' };
    detail.reports[1] = { ...detail.reports[1], dismissed_at: '2026-11-19T09:00:00Z' };
    setup(detail);
    expect(await screen.findByTestId('current-decision')).toHaveTextContent('Decided by staff: ImpersonationPretends to be a bank.');
    expect(screen.getByRole('region', { name: 'Reports' })).toHaveTextContent('1 active, 0 withdrawn, 1 dismissed');
  });

  it('lets only admins delete, after typing the username', async () => {
    setup(account());
    expect(await screen.findByRole('button', { name: 'Set status' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete account' })).not.toBeInTheDocument();
  });

  it('deletes the account as an admin and goes back to the queue', async () => {
    const { decisions, user } = setup(account(), true);
    await user.click(await screen.findByRole('button', { name: 'Delete account' }));
    const dialog = screen.getByRole('dialog', { name: 'Delete @dave' });
    await reasonFor(user, dialog, 'illegal', 'Shared illegal content.');
    await user.type(within(dialog).getByLabelText(/to confirm/), 'dave');
    await user.click(within(dialog).getByRole('button', { name: 'Delete account' }));
    expect(decisions.deleteAccount).toHaveBeenCalledWith('101', { reason_code: 'illegal', reason_text: 'Shared illegal content.' });
    expect(await screen.findByText('Queue page')).toBeInTheDocument();
  });
});
