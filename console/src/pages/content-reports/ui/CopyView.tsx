import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { BarChart3, MapPin, Paperclip, UserRound } from 'lucide-react';
import type { ContentCopy, CopiedContact, CopiedLocation, CopiedMessage, CopiedPoll } from '@/shared/api/content';
import { formatDateTime } from '@/shared/lib/time';

interface CopyViewProps {
  copy: ContentCopy;
  accountHref: (id: string) => string;
}

const openStreetMapUrl = (latitude: number, longitude: number) =>
  `https://www.openstreetmap.org/?mlat=${latitude}&mlon=${longitude}#map=16/${latitude}/${longitude}`;

function Tag({ children }: { children: ReactNode }) {
  return <span className="inline-flex items-center whitespace-nowrap rounded-full bg-muted px-2 py-0.5 text-xs font-medium">{children}</span>;
}

function PollView({ poll }: { poll: CopiedPoll }) {
  return (
    <section aria-label="Poll" className="mt-1 flex flex-col gap-1.5 text-sm">
      <p className="flex items-start gap-1.5 whitespace-pre-wrap break-words font-medium">
        <BarChart3 aria-hidden className="mt-0.5 size-4 shrink-0" />
        {poll.question}
      </p>
      <ul aria-label="Poll tags" className="flex flex-wrap gap-1">
        <li><Tag>{poll.anonymous ? 'Anonymous' : 'Public'}</Tag></li>
        {poll.multiple_answers && <li><Tag>Multiple answers</Tag></li>}
        {poll.quiz && <li><Tag>Quiz</Tag></li>}
        {poll.closed && <li><Tag>Closed</Tag></li>}
      </ul>
      <ol aria-label="Poll options" className="flex flex-col gap-0.5">
        {poll.options.map((option, i) => (
          <li key={i} className="flex items-baseline justify-between gap-3 rounded border bg-card px-2 py-1">
            <span className="whitespace-pre-wrap break-words">{option.text}</span>
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{option.votes === 1 ? '1 vote' : `${option.votes} votes`}</span>
          </li>
        ))}
      </ol>
    </section>
  );
}

function LocationView({ location }: { location: CopiedLocation }) {
  const { latitude, longitude, title } = location;
  const valid = Number.isFinite(latitude) && Number.isFinite(longitude);
  return (
    <section aria-label="Location" className="mt-1 flex flex-col gap-0.5 text-sm">
      <p className="flex items-start gap-1.5 break-words font-medium">
        <MapPin aria-hidden className="mt-0.5 size-4 shrink-0" />
        {title || 'Location without a name'}
      </p>
      <p className="text-xs tabular-nums text-muted-foreground">
        {latitude}, {longitude}
      </p>
      {valid && (
        <a href={openStreetMapUrl(latitude, longitude)} target="_blank" rel="noopener noreferrer" className="text-xs text-primary hover:underline">
          Open in OpenStreetMap
        </a>
      )}
    </section>
  );
}

function ContactView({ contact, accountHref }: { contact: CopiedContact; accountHref: (id: string) => string }) {
  return (
    <section aria-label="Contact" className="mt-1 flex flex-col gap-0.5 text-sm">
      <p className="flex items-start gap-1.5 break-words font-medium">
        <UserRound aria-hidden className="mt-0.5 size-4 shrink-0" />
        {contact.name}
      </p>
      {contact.username && <p className="text-xs text-muted-foreground">@{contact.username}</p>}
      <p className="text-xs text-muted-foreground">
        User ID{' '}
        <Link to={accountHref(contact.user_id)} className="tabular-nums hover:underline">
          {contact.user_id}
        </Link>
      </p>
    </section>
  );
}

function MessageRow({ message, accountHref }: { message: CopiedMessage; accountHref: (id: string) => string }) {
  const sender = message.sender_id ? (
    <Link to={accountHref(message.sender_id)} className="font-medium hover:underline">
      {message.sender_name}
    </Link>
  ) : (
    <span className="font-medium">{message.sender_name || 'System'}</span>
  );
  const structured = message.poll || message.location || message.contact;
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
      {message.poll && <PollView poll={message.poll} />}
      {message.location && <LocationView location={message.location} />}
      {message.contact && <ContactView contact={message.contact} accountHref={accountHref} />}
      {!structured && message.text && (
        <p className={`mt-1 whitespace-pre-wrap break-words text-sm ${message.reported ? 'text-foreground' : ''}`}>{message.text}</p>
      )}
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
      {!structured && !message.text && message.files.length === 0 && <p className="mt-1 text-xs italic">No text ({message.type})</p>}
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
