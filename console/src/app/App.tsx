import { useCallback, useEffect, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { ThemeProvider } from 'next-themes';
import { Loader2 } from 'lucide-react';
import { AccountPage } from '@/pages/account';
import { CasesRoute } from '@/pages/cases';
import { ContentReportRoute, ContentReportsRoute } from '@/pages/content-reports';
import { LoginPage } from '@/pages/login';
import { PackRoute, PacksRoute } from '@/pages/packs';
import { PlaceholderPage } from '@/pages/placeholder';
import { QueuePage } from '@/pages/queue';
import { StatusPage } from '@/pages/status';
import { ApiError, fetchMe, fetchMeta, signOut, type Meta, type Staff } from '@/shared/api/client';
import { Shell } from '@/widgets/shell';

type SessionState = { status: 'loading' } | { status: 'signed-out' } | { status: 'signed-in'; staff: Staff };

const App = () => {
  const [session, setSession] = useState<SessionState>({ status: 'loading' });
  const [meta, setMeta] = useState<Meta | null>(null);

  useEffect(() => {
    let active = true;
    fetchMe()
      .then(({ staff }) => active && setSession({ status: 'signed-in', staff }))
      .catch((error) => {
        if (!active) return;
        if (!(error instanceof ApiError) || error.status !== 401) console.error(error);
        setSession({ status: 'signed-out' });
      });
    fetchMeta().then((value) => active && setMeta(value)).catch(() => undefined);
    return () => {
      active = false;
    };
  }, []);

  const handleSignOut = useCallback(async () => {
    await signOut().catch(() => undefined);
    setSession({ status: 'signed-out' });
  }, []);

  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      {session.status === 'loading' && (
        <div className="flex min-h-[100dvh] items-center justify-center text-muted-foreground" role="status">
          <Loader2 aria-hidden="true" className="h-5 w-5 animate-spin" />
          <span className="sr-only">Loading</span>
        </div>
      )}
      {session.status === 'signed-out' && <LoginPage onSignedIn={(staff) => setSession({ status: 'signed-in', staff })} />}
      {session.status === 'signed-in' && (
        <Shell staff={session.staff} meta={meta} onSignOut={handleSignOut}>
          <Routes>
            <Route path="/" element={<Navigate to={session.staff.role === 'admin' ? '/status' : '/queue'} replace />} />
            <Route path="/status" element={<StatusPage />} />
            <Route path="/queue" element={<QueuePage />} />
            <Route path="/users/:id" element={<AccountPage isAdmin={session.staff.role === 'admin'} />} />
            <Route path="/cases" element={<CasesRoute />} />
            <Route path="/content-reports" element={<ContentReportsRoute />} />
            <Route path="/content-reports/:id" element={<ContentReportRoute />} />
            <Route path="/packs" element={<PacksRoute />} />
            <Route path="/packs/:kind/:id" element={<PackRoute />} />
            <Route path="/statistics" element={<PlaceholderPage title="Statistics" description="Daily numbers and charts." />} />
            <Route path="/staff" element={<PlaceholderPage title="Staff" description="Staff accounts and roles." />} />
            <Route path="*" element={<PlaceholderPage title="Not found" description="This page does not exist." />} />
          </Routes>
        </Shell>
      )}
    </ThemeProvider>
  );
};

export default App;
