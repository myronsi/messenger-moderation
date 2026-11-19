import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ApiError } from '@/shared/api/client';
import type { PackDetail } from '@/shared/api/decisions';
import { PackPage, PacksPage } from './PackPage';

const stickers: PackDetail = {
  kind: 'sticker_pack',
  id: '7',
  title: 'Cats',
  slug: 'CATSPACK1',
  owner: { id: '3', username: 'owner', display_name: 'Owner', created_at: '2026-01-01T00:00:00Z' },
  group_pack: false,
  builtin: false,
  hidden_at: '2026-11-19T08:00:00Z',
  hidden_by: 'reports',
  deleted_at: null,
  created_at: '2026-10-01T00:00:00Z',
  items: [
    { id: '1', attachment_id: '0b7f9a3e-1c2d-4e5f-8a9b-0c1d2e3f4a5b', label: '😺', animated: false },
    { id: '2', attachment_id: '1b7f9a3e-1c2d-4e5f-8a9b-0c1d2e3f4a5b', label: '😾', animated: true },
  ],
  reports: [{ reporter_id: '1', reporter_username: 'alice', reason: 'violence', created_at: '2026-11-19T07:00:00Z' }],
  cases: [],
};

const setup = (pack: () => Promise<PackDetail>) => {
  const api = { pack: vi.fn(pack), setPackHidden: vi.fn(async () => ({ changed: true })) };
  render(
    <MemoryRouter>
      <PackPage kind="sticker_pack" id="7" api={api} accountHref={(id) => `/users/${id}`} packsHref="/packs" />
    </MemoryRouter>,
  );
  return { api, user: userEvent.setup() };
};

describe('PackPage', () => {
  it('shows every sticker through the image proxy, the owner and the reports', async () => {
    setup(async () => stickers);
    expect(await screen.findByRole('heading', { name: 'Cats' })).toBeInTheDocument();
    const images = within(screen.getByRole('list', { name: 'Pack contents' })).getAllByRole('img');
    expect(images).toHaveLength(2);
    expect(images[0]).toHaveAttribute('src', '/api/packs/sticker_pack/7/images/0b7f9a3e-1c2d-4e5f-8a9b-0c1d2e3f4a5b');
    expect(images[0]).toHaveAccessibleName('Sticker 😺');
    expect(screen.getByRole('link', { name: '@owner' })).toHaveAttribute('href', '/users/3');
    expect(screen.getByTestId('pack-state')).toHaveTextContent('Hidden after reports');
    expect(screen.getByText(/: Violence/)).toBeInTheDocument();
  });

  it('makes a hidden pack visible again with a reason', async () => {
    const { api, user } = setup(async () => stickers);
    await user.click(await screen.findByRole('button', { name: 'Make visible' }));
    const dialog = screen.getByRole('dialog');
    await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'other');
    await user.type(within(dialog).getByLabelText(/Text for the user/), 'Cartoon cats, nothing wrong.');
    await user.click(within(dialog).getByRole('button', { name: 'Make visible' }));
    expect(api.setPackHidden).toHaveBeenCalledWith('sticker_pack', '7', false, { reason_code: 'other', reason_text: 'Cartoon cats, nothing wrong.' });
    expect(api.pack).toHaveBeenCalledTimes(2);
  });

  it('hides a visible pack', async () => {
    const { api, user } = setup(async () => ({ ...stickers, hidden_at: null, hidden_by: null }));
    await user.click(await screen.findByRole('button', { name: 'Hide pack' }));
    const dialog = screen.getByRole('dialog');
    await user.selectOptions(within(dialog).getByLabelText('Rule from the guidelines'), 'illegal');
    await user.type(within(dialog).getByLabelText(/Text for the user/), 'Copies protected artwork.');
    await user.click(within(dialog).getByRole('button', { name: 'Hide pack' }));
    expect(api.setPackHidden).toHaveBeenCalledWith('sticker_pack', '7', true, { reason_code: 'illegal', reason_text: 'Copies protected artwork.' });
  });

  it('offers no action on built-in or deleted packs', async () => {
    setup(async () => ({ ...stickers, owner: null, builtin: true, hidden_at: null, hidden_by: null }));
    expect(await screen.findByText(/Built-in pack/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Hide pack' })).not.toBeInTheDocument();
  });

  it('says when the pack does not exist', async () => {
    setup(async () => Promise.reject(new ApiError(404, 'not_found', 'not found')));
    expect(await screen.findByRole('alert')).toHaveTextContent('No sticker pack with ID 7.');
  });
});

describe('PacksPage', () => {
  it('lists reported and hidden packs', async () => {
    const api = {
      packs: vi.fn(async () => [
        { kind: 'emoji_pack' as const, id: '4', title: 'Faces', reports: 5, hidden_at: '2026-11-19T08:00:00Z', hidden_by: 'reports' as const, last_report_at: null },
        { kind: 'sticker_pack' as const, id: '7', title: 'Cats', reports: 1, hidden_at: null, hidden_by: null, last_report_at: null },
      ]),
    };
    render(
      <MemoryRouter>
        <PacksPage api={api} packHref={(kind, id) => `/packs/${kind}/${id}`} />
      </MemoryRouter>,
    );
    const items = within(await screen.findByRole('list', { name: 'Packs' })).getAllByRole('listitem');
    expect(within(items[0]).getByRole('link', { name: 'Emoji pack: Faces' })).toHaveAttribute('href', '/packs/emoji_pack/4');
    expect(items[0]).toHaveTextContent('5 reports · Hidden after reports');
    expect(items[1]).toHaveTextContent('1 report');
  });
});
