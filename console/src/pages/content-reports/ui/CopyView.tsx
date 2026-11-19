import { Link } from 'react-router-dom';
import { Paperclip } from 'lucide-react';
import type { ContentCopy, CopiedMessage } from '@/shared/api/content';
import { formatDateTime } from '@/shared/lib/time';

interface CopyViewProps {
  copy: ContentCopy;
  accountHref: (id: string) => string;
}

function MessageRow({ message, accountHref }: { message: CopiedMessage; accountHref: (id: string) => string }) {
  const sender = message.sender_id ? (
    <Link to={accountHref(message.sender_id)} className="font-medium hover:underline">
      {message.sender_name}
    </Link>
  ) : (
    <span className="font-medium">{message.sender_name || 'System'}</span>
  );
  return (
    <li
      data-testid={message.reported ? 'reported-message' : 'context-message'}
      aria-current={message.reported ? 'true' : undefined}
      className={
        message.reported
          ? 'rounded-md border-2 border-destructive bg-destructive-soft p-3'
          : 'rounded-md border bg-card p-3 text-muted-foreground'
      }
    >
      <div className="flex flex-wrap items-baseline gap-x-2 text-xs">
        {sender}
        {message.sender_username && <span>@{message.sender_username}</span>}
        <span>{formatDateTime(message.sent_at)}</span>
        {message.edited_at && <span>edited</span>}
        {message.reported && <span className="font-semibold uppercase tracking-wide text-destructive">Reported</span>}
      </div>
      {message.forwarded_from_name && <p className="mt-1 text-xs italic">Forwarded from {message.forwarded_from_name}</p>}
      {message.text && <p className={`mt-1 whitespace-pre-wrap break-words text-sm ${message.reported ? 'text-foreground' : ''}`}>{message.text}</p>}
      {message.files.length > 0 && (
        <ul aria-label="Files" className="mt-1 flex flex-col gap-0.5 text-xs">
          {message.files.map((name, i) => (
            <li key={`${name}-${i}`} className="flex items-center gap-1">
              <Paperclip aria-hidden className="size-3" />
              {name}
            </li>
          ))}
        </ul>
      )}
      {!message.text && message.files.length === 0 && <p className="mt-1 text-xs italic">No text ({message.type})</p>}
    </li>
  );
}

export function CopyView({ copy, accountHref }: CopyViewProps) {
  if (copy.group) {
    const g = copy.group;
    return (
      <dl className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-3 gap-y-2 rounded-md border-2 border-destructive bg-destructive-soft p-3 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="break-words font-medium">{g.title || '(no name)'}</dd>
        <dt className="text-muted-foreground">Description</dt>
        <dd className="whitespace-pre-wrap break-words">{g.description || '(none)'}</dd>
        <dt className="text-muted-foreground">Picture</dt>
        <dd>{g.avatar_attachment_id ? 'Has a picture' : 'No picture'}</dd>
        <dt className="text-muted-foreground">Owner</dt>
        <dd>
          {g.owner_id ? (
            <Link to={accountHref(g.owner_id)} className="hover:underline">
              Account {g.owner_id}
            </Link>
          ) : (
            'Unknown'
          )}
        </dd>
        <dt className="text-muted-foreground">Members</dt>
        <dd>{g.member_count}</dd>
      </dl>
    );
  }
  const messages = copy.messages ?? [];
  if (messages.length === 0) return <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">The copy is empty.</p>;
  return (
    <ol aria-label="Copied messages" className="flex flex-col gap-2">
      {messages.map((m) => (
        <MessageRow key={m.id} message={m} accountHref={accountHref} />
      ))}
    </ol>
  );
}
