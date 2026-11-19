import { cn } from '@/shared/lib/utils';
import { statusLabel } from '../model/labels';

const TONES: Record<string, string> = {
  good: 'bg-success-soft text-success',
  warned: 'bg-warning-soft text-warning',
  limited: 'bg-warning-soft text-warning ring-1 ring-inset ring-warning/40',
  restricted: 'bg-destructive-soft text-destructive',
  suspended: 'bg-destructive text-destructive-foreground',
};

export function StatusBadge({ status, className }: { status: string; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium',
        TONES[status] ?? 'bg-destructive-soft text-destructive',
        className,
      )}
    >
      {statusLabel(status)}
    </span>
  );
}
