import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { account } from '@/test/moderationFixtures';
import App from './App';

const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }));

const mockServer = (routes: Record<string, () => Promise<Response>>) =>
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`;
    const handler = routes[key];
    return handler ? handler() : json(404, { error: 'not_found' });
  }));

const renderApp = (path = '/') => render(<MemoryRouter initialEntries={[path]}><App /></MemoryRouter>);

describe('App', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('asks to sign in when there is no session', async () => {
    mockServer({ 'GET /api/me': () => json(401, { error: 'unauthenticated' }), 'GET /api/meta': () => json(200, { environment: 'staging', version: '1' }) });
    renderApp();
    expect(await screen.findByRole('heading', { name: 'Messenger moderation' })).toBeInTheDocument();
  });

  it('signs in with username, password and code, then shows the shell', async () => {
    let signedIn = false;
    mockServer({
      'GET /api/me': () => json(401, { error: 'unauthenticated' }),
      'GET /api/meta': () => json(200, { environment: 'production', version: '1' }),
      'POST /api/auth/login': () => {
        signedIn = true;
        return json(200, { staff: { id: 1, username: 'operator', role: 'admin' }, expires_at: '2026-11-20T00:00:00Z' });
      },
    });
    const user = userEvent.setup();
    renderApp();
    await user.type(await screen.findByLabelText('Username'), 'operator');
    await user.type(screen.getByLabelText('Password'), 'a long enough password');
    await user.type(screen.getByLabelText('Authenticator code'), '12a3456');
    expect(screen.getByLabelText('Authenticator code')).toHaveValue('123456');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    await waitFor(() => expect(signedIn).toBe(true));
    expect(await screen.findByRole('heading', { name: 'Status' })).toBeInTheDocument();
    expect(screen.getByTestId('environment')).toHaveTextContent('production');
  });

  it('shows the server message when sign-in fails', async () => {
    mockServer({
      'GET /api/me': () => json(401, { error: 'unauthenticated' }),
      'GET /api/meta': () => json(200, { environment: 'development', version: '1' }),
      'POST /api/auth/login': () => json(401, { error: 'invalid_login', message: 'invalid username, password or code' }),
    });
    const user = userEvent.setup();
    renderApp();
    await user.type(await screen.findByLabelText('Username'), 'operator');
    await user.type(screen.getByLabelText('Password'), 'wrong');
    await user.type(screen.getByLabelText('Authenticator code'), '000000');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('invalid username, password or code');
    expect(screen.getByLabelText('Authenticator code')).toHaveValue('');
  });

  it('hides admin sections from moderators and sends them to the queue', async () => {
    mockServer({
      'GET /api/me': () => json(200, { staff: { id: 2, username: 'helper', role: 'moderator' } }),
      'GET /api/meta': () => json(200, { environment: 'staging', version: '1' }),
      'GET /api/queue': () => json(200, { items: [], next_cursor: null }),
    });
    renderApp();
    expect(await screen.findByRole('heading', { name: 'Review queue' })).toBeInTheDocument();
    expect(screen.queryAllByRole('link', { name: 'Status' })).toHaveLength(0);
    expect(screen.queryAllByRole('link', { name: 'Staff' })).toHaveLength(0);
  });

  it('opens an account from the search box', async () => {
    mockServer({
      'GET /api/me': () => json(200, { staff: { id: 2, username: 'helper', role: 'moderator' } }),
      'GET /api/meta': () => json(200, { environment: 'staging', version: '1' }),
      'GET /api/queue': () => json(200, { items: [], next_cursor: null }),
      'GET /api/users?q=dave': () => json(200, { users: [{ id: '4', username: 'dave', display_name: 'Dave', created_at: '2026-10-01T08:00:00Z', status: 'warned' }] }),
      'GET /api/users/4': () => json(200, account()),
    });
    const user = userEvent.setup();
    renderApp('/queue');
    const [search] = await screen.findAllByRole('searchbox', { name: 'Find an account' });
    await user.type(search, 'dave');
    await user.click(await screen.findByRole('link', { name: /@dave · ID 4/ }));
    expect(await screen.findByRole('heading', { name: 'Dave' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to the queue' })).toHaveAttribute('href', '/queue');
    expect(search).toHaveValue('');
  });

  it('signs out', async () => {
    mockServer({
      'GET /api/me': () => json(200, { staff: { id: 1, username: 'operator', role: 'admin' } }),
      'GET /api/meta': () => json(200, { environment: 'staging', version: '1' }),
      'POST /api/auth/logout': () => json(204, null),
    });
    const user = userEvent.setup();
    renderApp('/status');
    await user.click(await screen.findByRole('button', { name: 'Sign out' }));
    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeInTheDocument();
  });
});
