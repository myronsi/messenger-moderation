import { Activity, BarChart3, Inbox, Users } from 'lucide-react';

export interface NavItem {
  to: string;
  label: string;
  icon: typeof Activity;
  adminOnly?: boolean;
}

export const NAV_ITEMS: NavItem[] = [
  { to: '/status', label: 'Status', icon: Activity, adminOnly: true },
  { to: '/queue', label: 'Queue', icon: Inbox },
  { to: '/statistics', label: 'Statistics', icon: BarChart3 },
  { to: '/staff', label: 'Staff', icon: Users, adminOnly: true },
];
