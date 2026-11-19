import type { ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { LogOut, ShieldCheck } from 'lucide-react';
import type { Meta, Staff } from '@/shared/api/client';
import { cn } from '@/shared/lib/utils';
import { Button } from '@/shared/ui/button';
import { NAV_ITEMS } from './navItems';

const ENVIRONMENT_CLASS: Record<Meta['environment'], string> = {
  production: 'bg-destructive text-destructive-foreground',
  staging: 'bg-warning text-warning-foreground',
  development: 'bg-muted text-muted-foreground',
};

interface ShellProps {
  staff: Staff;
  meta: Meta | null;
  onSignOut: () => void;
  children: ReactNode;
}

const Shell = ({ staff, meta, onSignOut, children }: ShellProps) => (
  <div className="flex min-h-[100dvh] flex-col">
    <header className="sticky top-0 z-20 border-b border-border bg-background/90 backdrop-blur">
      <div className="flex h-14 items-center gap-3 px-4">
        <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <ShieldCheck aria-hidden="true" className="h-4 w-4" />
        </span>
        <span className="font-semibold text-foreground">Moderation</span>
        {meta && (
          <span data-testid="environment" className={cn('rounded-full px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wide', ENVIRONMENT_CLASS[meta.environment])}>
            {meta.environment}
          </span>
        )}
        <div className="flex-1" />
        <span className="hidden text-sm text-muted-foreground sm:inline">{staff.username} · {staff.role}</span>
        <Button variant="ghost" size="sm" onClick={onSignOut}>
          <LogOut aria-hidden="true" />
          Sign out
        </Button>
      </div>
      <nav aria-label="Sections" className="flex gap-1 overflow-x-auto px-3 pb-2 md:hidden">
        {NAV_ITEMS.filter((item) => !item.adminOnly || staff.role === 'admin').map((item) => (
          <NavLink key={item.to} to={item.to}
            className={({ isActive }) => cn('whitespace-nowrap rounded-md px-3 py-1.5 text-sm',
              isActive ? 'bg-primary/10 font-medium text-primary' : 'text-subtle-foreground hover:bg-accent')}>
            {item.label}
          </NavLink>
        ))}
      </nav>
    </header>
    <div className="flex flex-1">
      <nav aria-label="Sections" className="hidden w-56 shrink-0 border-r border-border p-3 md:block">
        <ul className="space-y-0.5">
          {NAV_ITEMS.filter((item) => !item.adminOnly || staff.role === 'admin').map(({ to, label, icon: Icon }) => (
            <li key={to}>
              <NavLink to={to}
                className={({ isActive }) => cn('flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors',
                  isActive ? 'bg-primary/10 font-medium text-primary' : 'text-subtle-foreground hover:bg-accent hover:text-foreground')}>
                <Icon aria-hidden="true" className="h-4 w-4" />
                {label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
      <main className="min-w-0 flex-1 p-4 md:p-6">{children}</main>
    </div>
  </div>
);

export default Shell;
