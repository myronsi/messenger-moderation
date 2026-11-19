import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { UserHit } from '@/shared/api/review';
import { hits } from '@/test/moderationFixtures';
import { UserSearch } from './UserSearch';

const setup = (search: (q: string) => Promise<UserHit[]>) => {
  const api = { searchUsers: vi.fn(search) };
  render(
    <MemoryRouter>
      <UserSearch api={api} accountHref={(id) => `/accounts/${id}`} delayMs={0} />
    </MemoryRouter>,
  );
  return { api, user: userEvent.setup(), box: screen.getByRole('searchbox', { name: 'Find an account' }) };
};

describe('UserSearch', () => {
  it('finds accounts and links to them', async () => {
    const { api, user, box } = setup(async () => hits);
    await user.type(box, '@dav');

    const list = await screen.findByRole('list', { name: 'Accounts found' });
    const links = within(list).getAllByRole('link');
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute('href', '/accounts/101');
    expect(links[0]).toHaveTextContent('@dave · ID 101');
    expect(links[0]).toHaveTextContent('Warned');
    expect(api.searchUsers).toHaveBeenLastCalledWith('dav');
  });

  it('waits for two characters unless the query is an ID', async () => {
    const { api, user, box } = setup(async () => []);
    await user.type(box, 'd');
    expect(api.searchUsers).not.toHaveBeenCalled();

    await user.clear(box);
    await user.type(box, '7');
    expect(await screen.findByText('No account matches "7".')).toBeInTheDocument();
    expect(api.searchUsers).toHaveBeenCalledWith('7');
  });

  it('reports a failed search and closes on Escape', async () => {
    const { user, box } = setup(async () => {
      throw new Error('down');
    });
    await user.type(box, 'grace');
    expect(await screen.findByRole('alert')).toHaveTextContent('Search failed.');
    await user.keyboard('{Escape}');
    expect(box).toHaveValue('');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
