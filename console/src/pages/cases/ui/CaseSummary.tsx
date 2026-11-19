import { Link } from 'react-router-dom';
import { ArrowRight } from 'lucide-react';
import { ruleLabel, StatusBadge } from '@/entities/moderation';
import type { ModerationCase } from '@/shared/api/decisions';
import { formatDateTime } from '@/shared/lib/time';
import { KIND_LABELS, MAIL_LABELS, stateLabel } from '../model/kinds';

interface CaseSummaryProps {
  item: ModerationCase;
  accountHref: (id: string) => string;
  packHref: (kind: string, id: string) => string;
}

function Subject({ item, accountHref, packHref }: CaseSummaryProps) {
  if (item.user_id) {
    const name = item.subject?.display_name ?? `Account ${item.user_id}`;
    return (
      <Link to={accountHref(item.user_id)} className="font-medium hover:underline">
        {name}
        {item.subject?.username && <span className="ml-1 text-muted-foreground">@{item.subject.username}</span>}
      </Link>
    );
  }
  if (item.kind === 'notice') {
    return <span className="font-medium">{item.notice?.location ?? 'Notice'}</span>;
  }
  const kindLabel = item.pack_kind === 'emoji_pack' ? 'Emoji pack' : 'Sticker pack';
  return (
    <Link to={packHref(item.pack_kind ?? 'sticker_pack', item.pack_id ?? '')} className="font-medium hover:underline">
      {kindLabel}: {item.subject?.title ?? item.pack_id}
    </Link>
  );
}

function What({ item }: { item: ModerationCase }) {
  if (item.kind === 'account_status' && item.to_status) {
    return (
      <span className="inline-flex flex-wrap items-center gap-1">
        {item.from_status && <StatusBadge status={item.from_status} />}
        <ArrowRight aria-label="to" className="size-4 text-muted-foreground" />
        <StatusBadge status={item.to_status} />
        <span className="text-muted-foreground">automatically</span>
      </span>
    );
  }
  if (item.kind === 'review_request') {
    return (
      <div className="flex flex-col gap-1">
        <span className="inline-flex flex-wrap items-center gap-1">
          Asked for a review by a person while
          {item.to_status && <StatusBadge status={item.to_status} />}
          {item.subject?.status && item.subject.status !== item.to_status && (
            <>
              <span className="text-muted-foreground">now</span>
              <StatusBadge status={item.subject.status} />
            </>
          )}
        </span>
        {item.request_text && <blockquote className="whitespace-pre-wrap border-l-2 pl-2 text-muted-foreground">{item.request_text}</blockquote>}
      </div>
    );
  }
  if (item.kind === 'notice' && item.notice) {
    const n = item.notice;
    return (
      <dl className="grid gap-x-3 gap-y-1 text-xs sm:grid-cols-[auto_1fr]">
        <dt className="text-muted-foreground">From</dt>
        <dd>
          {n.notifier_name}
          {n.notifier_email && <span className="text-muted-foreground"> · {n.notifier_email}</span>}
        </dd>
        <dt className="text-muted-foreground">Received</dt>
        <dd>{formatDateTime(n.received_at)}</dd>
        <dt className="text-muted-foreground">What</dt>
        <dd className="whitespace-pre-wrap">{n.content}</dd>
        <dt className="text-muted-foreground">Where</dt>
        <dd className="whitespace-pre-wrap break-words">{n.location}</dd>
        <dt className="text-muted-foreground">Why illegal</dt>
        <dd className="whitespace-pre-wrap">{n.explanation}</dd>
      </dl>
    );
  }
  return <span>Hidden automatically after reports</span>;
}

export function CaseSummary(props: CaseSummaryProps) {
  const { item } = props;
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{KIND_LABELS[item.kind] ?? item.kind}</span>
      <Subject {...props} />
      <What item={item} />
      <span className="text-xs text-muted-foreground">
        Opened {formatDateTime(item.opened_at)}
        {item.notice && ` · entered by ${item.notice.entered_by}`}
      </span>
      {item.state !== 'open' && (
        <span className="text-xs">
          {stateLabel(item.kind, item.state)}
          {item.decided_by && ` by ${item.decided_by}`}
          {item.decided_at && ` on ${formatDateTime(item.decided_at)}`}
          {item.reason_code && ` · ${ruleLabel(item.reason_code)}`}
        </span>
      )}
      {item.notice && item.state !== 'open' && <span className="text-xs text-muted-foreground">{MAIL_LABELS[item.notice.mail_state]}</span>}
    </div>
  );
}
