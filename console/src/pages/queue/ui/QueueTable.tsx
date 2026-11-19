import { Link } from 'react-router-dom';
import { reasonLabel, StatusBadge } from '@/entities/moderation';
import { type QueueItem } from '@/shared/api/review';
import { formatAge, formatDateTime } from '@/shared/lib/time';

interface QueueTableProps {
  items: QueueItem[];
  accountHref: (id: string) => string;
  now: Date;
}

const score = (value: number) => value.toLocaleString('en-GB', { maximumFractionDigits: 2 });

export function QueueTable({ items, accountHref, now }: QueueTableProps) {
  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full min-w-[720px] text-left text-sm">
        <thead className="bg-muted/60 text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th scope="col" className="px-3 py-2 font-medium">Account</th>
            <th scope="col" className="px-3 py-2 font-medium">Status</th>
            <th scope="col" className="px-3 py-2 text-right font-medium">Score</th>
            <th scope="col" className="px-3 py-2 text-right font-medium">New reports</th>
            <th scope="col" className="px-3 py-2 font-medium">Reasons</th>
            <th scope="col" className="px-3 py-2 font-medium">Waiting</th>
            <th scope="col" className="px-3 py-2 font-medium">Last report</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {items.map((item) => (
            <tr key={item.user.id} className="hover:bg-accent/50">
              <td className="px-3 py-2">
                <Link to={accountHref(item.user.id)} className="font-medium text-primary hover:underline">
                  {item.user.display_name}
                </Link>
                <div className="text-xs text-muted-foreground">@{item.user.username}</div>
              </td>
              <td className="px-3 py-2">
                <StatusBadge status={item.status} />
              </td>
              <td className="px-3 py-2 text-right tabular-nums">{score(item.score)}</td>
              <td className="px-3 py-2 text-right tabular-nums">
                {item.open_reports}
                {item.weighted_reports < item.open_reports && (
                  <span className="block text-xs text-muted-foreground">{item.weighted_reports} weighted</span>
                )}
              </td>
              <td className="px-3 py-2">{item.reasons.length ? item.reasons.map(reasonLabel).join(', ') : '—'}</td>
              <td className="px-3 py-2 whitespace-nowrap" title={formatDateTime(item.opened_at)}>
                {formatAge(item.opened_at, now)}
              </td>
              <td className="px-3 py-2 whitespace-nowrap text-muted-foreground">
                {item.newest_report_at ? `${formatAge(item.newest_report_at, now)} ago` : 'No new reports'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
