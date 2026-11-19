import { Activity, BarChart3, Clock, Inbox, MessageSquareWarning, Sticker, Users } from 'lucide-react';

export interface NavItem {
  to: string;
  label: string;
  icon: typeof Activity;
  adminOnly?: boolean;
}

export const NAV_ITEMS: NavItem[] = [
  { to: '/status', label: 'Status', icon: Activity, adminOnly: true },
  { to: '/queue', label: 'Queue', icon: Inbox },
  { to: '/cases', label: 'Cases', icon: Clock },
  { to: '/content-reports', label: 'Content', icon: MessageSquareWarning },
  { to: '/packs', label: 'Packs', icon: Sticker },
  { to: '/statistics', label: 'Statistics', icon: BarChart3 },
  { to: '/staff', label: 'Staff', icon: Users, adminOnly: true },
];
