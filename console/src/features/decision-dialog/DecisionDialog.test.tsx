import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '@/shared/api/client';
import { DecisionDialog } from './DecisionDialog';

const setup = (props: Partial<Parameters<typeof DecisionDialog>[0]> = {}) => {
  const onSubmit = vi.fn(async () => undefined);
  const onDone = vi.fn();
  const onCancel = vi.fn();
  render(<DecisionDialog title="Set status" submitLabel="Save" onSubmit={onSubmit} onDone={onDone} onCancel={onCancel} {...props} />);
  return { onSubmit, onDone, onCancel, user: userEvent.setup() };
};

describe('DecisionDialog', () => {
  it('needs a rule and a text before it sends the decision', async () => {
    const { onSubmit, onDone, user } = setup();
    const save = screen.getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();
    await user.selectOptions(screen.getByLabelText('Rule from the guidelines'), 'illegal');
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText(/Text for the user/), '  Shared illegal content.  ');
    expect(screen.getByText('23/1000')).toBeInTheDocument();
    await user.click(save);
    expect(onSubmit).toHaveBeenCalledWith({ reason_code: 'illegal', reason_text: 'Shared illegal content.' });
    expect(onDone).toHaveBeenCalled();
  });

  it('asks to type a word for heavy actions', async () => {
    const { onSubmit, user } = setup({ confirmWord: 'dave', destructive: true });
    await user.selectOptions(screen.getByLabelText('Rule from the guidelines'), 'spam');
    await user.type(screen.getByLabelText(/Text for the user/), 'Spam links.');
    const save = screen.getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText(/to confirm/), 'dav');
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText(/to confirm/), 'e');
    await user.click(save);
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it('shows the error and stays open when the decision fails', async () => {
    const { onDone, user } = setup({ onSubmit: vi.fn(async () => Promise.reject(new ApiError(400, 'invalid_request', 'invalid request: choose a status below restricted'))) });
    await user.selectOptions(screen.getByLabelText('Rule from the guidelines'), 'other');
    await user.type(screen.getByLabelText(/Text for the user/), 'No violation.');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('choose a status below restricted');
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
  });

  it('closes with Escape or Cancel', async () => {
    const { onCancel, user } = setup();
    await user.keyboard('{Escape}');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(2);
  });
});
