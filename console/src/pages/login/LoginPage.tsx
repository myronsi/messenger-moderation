import { useState, type FormEvent } from 'react';
import { Loader2, ShieldCheck } from 'lucide-react';
import { ApiError, signIn, type Staff } from '@/shared/api/client';
import { Button } from '@/shared/ui/button';
import { Input } from '@/shared/ui/input';

const LoginPage = ({ onSignedIn }: { onSignedIn: (staff: Staff) => void }) => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const { staff } = await signIn(username, password, code);
      onSignedIn(staff);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : 'Unable to reach the server. Try again.');
      setCode('');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-[100dvh] items-center justify-center bg-gradient-to-br from-primary/10 to-background px-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-5 rounded-lg border border-border bg-card p-6 shadow-sm" aria-labelledby="login-title">
        <div className="flex items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <ShieldCheck aria-hidden="true" className="h-5 w-5" />
          </span>
          <div>
            <h1 id="login-title" className="text-lg font-semibold text-foreground">Messenger moderation</h1>
            <p className="text-sm text-muted-foreground">Staff sign-in</p>
          </div>
        </div>
        <label className="block space-y-1.5">
          <span className="text-sm font-medium text-foreground">Username</span>
          <Input autoComplete="username" autoCapitalize="none" spellCheck={false} required value={username}
            onChange={(event) => setUsername(event.target.value)} />
        </label>
        <label className="block space-y-1.5">
          <span className="text-sm font-medium text-foreground">Password</span>
          <Input type="password" autoComplete="current-password" required value={password}
            onChange={(event) => setPassword(event.target.value)} />
        </label>
        <label className="block space-y-1.5">
          <span className="text-sm font-medium text-foreground">Authenticator code</span>
          <Input inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} required value={code}
            onChange={(event) => setCode(event.target.value.replace(/\D/g, ''))} />
        </label>
        {error && <p role="alert" className="rounded-md bg-destructive-soft px-3 py-2 text-sm text-destructive">{error}</p>}
        <Button type="submit" className="w-full" disabled={busy}>
          {busy && <Loader2 aria-hidden="true" className="animate-spin" />}
          Sign in
        </Button>
      </form>
    </div>
  );
};

export default LoginPage;
