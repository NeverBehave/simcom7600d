import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { Link, NavLink, Route, Routes, useMatch, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { ArrowLeft, Check, CheckCheck, CircleAlert, Clock3, Info, MessageCircle, Pencil, Phone, Send, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useCallDial, useSmsDelete, useSmsList, useSmsSend } from '@/api/queries';
import { RelativeTime } from '@/components/RelativeTime';
import { EmptyState } from '@/components/EmptyState';
import { Compose } from './Compose';
import { Toaster } from '@/components/ui/sonner';
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer';
import { cn } from '@/lib/cn';
import { formatPhoneNumber, phoneKey } from '@/lib/phone';
import { toast } from 'sonner';

export interface InboundSMS {
  id: string;
  direction: 'in';
  from: string;
  body: string;
  received_at: string;
  smsc_ts: string;
  parts: number;
  encoding: string;
  incomplete: boolean;
}

export interface OutboundSMS {
  id: string;
  direction: 'out';
  to: string;
  body: string;
  state: string;
  parts: Array<{ mr: number }> | null;
  encoding: string;
  ts: string;
  error_code?: string;
  error_detail?: string;
}

export type SmsItem = InboundSMS | OutboundSMS;

export interface SmsThread {
  key: string;
  number: string;
  messages: SmsItem[];
  latest: SmsItem;
}

export function isInbound(message: SmsItem): message is InboundSMS {
  return message.direction === 'in';
}

export function messageTime(message: SmsItem) {
  return isInbound(message) ? message.received_at : message.ts;
}

export function messagePeer(message: SmsItem) {
  return isInbound(message) ? message.from : message.to;
}

export function groupMessages(messages: SmsItem[]): SmsThread[] {
  const groups = new Map<string, SmsItem[]>();
  for (const message of messages) {
    const peer = messagePeer(message);
    const key = phoneKey(peer) || peer;
    const existing = groups.get(key) ?? [];
    existing.push(message);
    groups.set(key, existing);
  }
  return [...groups.entries()].map(([key, items]) => {
    items.sort((a, b) => Date.parse(messageTime(a)) - Date.parse(messageTime(b)));
    const latest = items[items.length - 1];
    return { key, number: messagePeer(latest), messages: items, latest };
  }).sort((a, b) => Date.parse(messageTime(b.latest)) - Date.parse(messageTime(a.latest)));
}

export type DeliveryIndicatorKind = 'single-check' | 'double-check' | 'pending' | 'failed';

export function deliveryIndicator(state: string): { kind: DeliveryIndicatorKind; label: string } {
  switch (state) {
    case 'queued': return { kind: 'single-check', label: 'Queued' };
    case 'submitted': return { kind: 'double-check', label: 'Submitted' };
    case 'accepted': return { kind: 'double-check', label: 'Accepted by carrier' };
    case 'delivered': return { kind: 'double-check', label: 'Delivered' };
    case 'failed': return { kind: 'failed', label: 'Failed' };
    default: return { kind: 'pending', label: 'Delivery status unknown' };
  }
}

export default function Sms() {
  const [params, setParams] = useSearchParams();
  const detailOpen = !!useMatch('/sms/:threadKey');
  const composeOpen = params.get('compose') === '1';
  const inbound = useSmsList({ direction: 'in', limit: 500 });
  const outbound = useSmsList({ direction: 'out', limit: 500 });
  const messages = useMemo(() => [
    ...((inbound.data?.items ?? []) as InboundSMS[]),
    ...((outbound.data?.items ?? []) as OutboundSMS[]),
  ], [inbound.data, outbound.data]);
  const threads = useMemo(() => groupMessages(messages), [messages]);

  function setCompose(open: boolean) {
    const next = new URLSearchParams(params);
    if (open) next.set('compose', '1'); else next.delete('compose');
    setParams(next, { replace: true });
  }

  return (
    <div className="h-full min-h-0 overflow-hidden md:grid md:grid-cols-[22rem_minmax(0,1fr)] md:grid-rows-[minmax(0,1fr)]">
      <ConversationList
        threads={threads}
        loading={inbound.isLoading || outbound.isLoading}
        error={inbound.error || outbound.error}
        onCompose={() => setCompose(true)}
        className={detailOpen ? 'hidden md:flex' : 'flex'}
      />
      <div className={cn('h-full min-h-0 bg-neutral-100', !detailOpen && 'hidden md:block')}>
        <Routes>
          <Route index element={<EmptyState title="Choose a conversation" />} />
          <Route path=":threadKey" element={<Conversation threads={threads} />} />
        </Routes>
      </div>
      <Compose open={composeOpen} onClose={() => setCompose(false)} />
      <Toaster richColors position="bottom-right" />
    </div>
  );
}

function ConversationList({ threads, loading, error, onCompose, className }: {
  threads: SmsThread[];
  loading: boolean;
  error: Error | null;
  onCompose: () => void;
  className?: string;
}) {
  const [search, setSearch] = useState('');
  const term = search.trim().toLowerCase();
  const visible = term ? threads.filter((thread) =>
    thread.number.toLowerCase().includes(term) ||
    formatPhoneNumber(thread.number).toLowerCase().includes(term) ||
    thread.messages.some((message) => message.body.toLowerCase().includes(term)),
  ) : threads;

  return (
    <aside className={cn('h-full min-h-0 w-full flex-col border-r bg-white', className)}>
      <header className="shrink-0 space-y-3 border-b p-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold">Messages</h1>
            <p className="text-xs text-neutral-500">SMS conversations</p>
          </div>
          <Button size="icon-sm" aria-label="New message" title="New message" onClick={onCompose}><Pencil /></Button>
        </div>
        <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search conversations" aria-label="Search conversations" />
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
        {loading && <div className="p-4 text-sm text-neutral-500">Loading conversations…</div>}
        {error && <div className="m-3 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="alert">Could not load all messages: {error.message}</div>}
        <ul className="p-2">
          {visible.map((thread) => <ConversationRow key={thread.key} thread={thread} />)}
        </ul>
        {!loading && visible.length === 0 && <EmptyState title={search ? 'No matching conversations' : 'No messages yet'} />}
      </div>
    </aside>
  );
}

function ConversationRow({ thread }: { thread: SmsThread }) {
  const latest = thread.latest;
  return (
    <li>
      <NavLink to={`/sms/${encodeURIComponent(thread.key)}`} className={({ isActive }) => cn('flex items-center gap-3 rounded-xl px-3 py-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500', isActive ? 'bg-brand-50 text-brand-950' : 'hover:bg-neutral-100')}>
        <span className="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-brand-50 text-brand-700"><MessageCircle className="h-5 w-5" /></span>
        <span className="min-w-0 flex-1">
          <span className="flex items-baseline justify-between gap-3">
            <span className="truncate font-medium">{formatPhoneNumber(thread.number)}</span>
            <span className="shrink-0 text-[11px] text-neutral-400"><RelativeTime ts={messageTime(latest)} /></span>
          </span>
          <span className="block truncate text-sm text-neutral-500">{isInbound(latest) ? '' : 'You: '}{latest.body}</span>
        </span>
      </NavLink>
    </li>
  );
}

function Conversation({ threads }: { threads: SmsThread[] }) {
  const { threadKey } = useParams();
  const navigate = useNavigate();
  const send = useSmsSend();
  const dial = useCallDial();
  const remove = useSmsDelete();
  const [body, setBody] = useState('');
  const [selectedMessage, setSelectedMessage] = useState<SmsItem | null>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const thread = threads.find((candidate) => candidate.key === threadKey);

  useEffect(() => {
    const element = viewport.current;
    if (!element) return;
    element.scrollTop = element.scrollHeight;
  }, [threadKey, thread?.messages.length]);

  if (!thread) {
    return <div className="flex h-full flex-col"><MobileConversationHeader title="Conversation" /><EmptyState title="Conversation not found" /></div>;
  }
  const number = thread.number;

  async function submit(event: FormEvent) {
    event.preventDefault();
    const text = body.trim();
    if (!text || !thread) return;
    try {
      await send.mutateAsync({ to: number, body: text });
      setBody('');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    }
  }

  async function callNumber() {
    try {
      const call = await dial.mutateAsync({ to: number });
      if (call && typeof call === 'object' && 'id' in call) navigate(`/calls/${(call as { id: string }).id}`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    }
  }

  return (
    <section className="flex h-full min-h-0 flex-col" aria-label={`Conversation with ${thread.number}`}>
      <header className="flex h-16 shrink-0 items-center gap-3 border-b bg-white px-3 sm:px-5">
        <Link to="/sms" className="grid h-9 w-9 place-items-center rounded-full text-neutral-600 hover:bg-neutral-100 md:hidden" aria-label="Back to conversations"><ArrowLeft className="h-5 w-5" /></Link>
        <span className="grid h-10 w-10 place-items-center rounded-full bg-brand-50 text-brand-700"><MessageCircle className="h-5 w-5" /></span>
        <div className="min-w-0 flex-1">
          <h1 className="truncate font-semibold">{formatPhoneNumber(thread.number)}</h1>
          <p className="text-xs text-neutral-500">{thread.messages.length} {thread.messages.length === 1 ? 'message' : 'messages'}</p>
        </div>
        <Button variant="ghost" size="icon" aria-label={`Call ${thread.number}`} title="Call" disabled={dial.isPending} onClick={callNumber}><Phone /></Button>
      </header>

      <div ref={viewport} className="min-h-0 flex-1 overflow-y-auto overscroll-contain scroll-smooth px-3 py-5 sm:px-6">
        <div className="mx-auto flex min-h-full max-w-4xl flex-col justify-end gap-2">
          {thread.messages.map((message, index) => {
            const previous = thread.messages[index - 1];
            const showDate = !previous || !sameDay(messageTime(previous), messageTime(message));
            return (
              <div key={message.id}>
                {showDate && <div className="my-4 text-center text-xs font-medium text-neutral-400">{formatMessageDate(messageTime(message))}</div>}
                <MessageBubble
                  message={message}
                  deleting={remove.isPending && remove.variables === message.id}
                  onDelete={() => remove.mutate(message.id)}
                  onDetails={() => setSelectedMessage(message)}
                />
              </div>
            );
          })}
        </div>
      </div>

      <form onSubmit={submit} className="shrink-0 border-t bg-white p-3 sm:p-4">
        <div className="mx-auto flex max-w-4xl items-end gap-2">
          <Textarea
            value={body}
            onChange={(event) => setBody(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
            rows={1}
            className="max-h-32 min-h-10 resize-none rounded-2xl"
            placeholder="Text message"
            aria-label="Message"
          />
          <Button size="icon-lg" className="rounded-full" type="submit" disabled={!body.trim() || send.isPending} aria-label="Send message"><Send /></Button>
        </div>
        {send.isError && <p className="mx-auto mt-2 max-w-4xl text-sm text-red-700" role="alert">Message failed: {send.error.message}</p>}
      </form>
      <MessageDetails message={selectedMessage} onClose={() => setSelectedMessage(null)} />
    </section>
  );
}

function MobileConversationHeader({ title }: { title: string }) {
  return <header className="flex h-16 shrink-0 items-center gap-3 border-b bg-white px-3"><Link to="/sms" className="grid h-9 w-9 place-items-center rounded-full"><ArrowLeft /></Link><h1 className="font-semibold">{title}</h1></header>;
}

function MessageBubble({ message, deleting, onDelete, onDetails }: { message: SmsItem; deleting: boolean; onDelete: () => void; onDetails: () => void }) {
  const inbound = isInbound(message);
  const actions = (
    <div className="flex gap-0.5 opacity-70 sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100 sm:group-focus-within:opacity-100">
      <Button variant="ghost" size="icon-xs" className="text-neutral-500" aria-label="Message details" title="Message details" onClick={onDetails}><Info /></Button>
      <Button variant="ghost" size="icon-xs" className="text-neutral-500" aria-label="Delete message" title="Delete message" disabled={deleting} onClick={onDelete}><Trash2 /></Button>
    </div>
  );
  return (
    <div className={cn('group flex items-end gap-1.5', inbound ? 'justify-start' : 'justify-end')}>
      {!inbound && actions}
      <div className={cn('max-w-[82%] rounded-2xl px-3.5 py-2 shadow-sm sm:max-w-[70%]', inbound ? 'rounded-bl-md border bg-white text-neutral-900' : 'rounded-br-md bg-brand-700 text-white')}>
        <p className="whitespace-pre-wrap break-words text-[15px] leading-snug">{message.body}</p>
        <div className={cn('mt-1 flex items-center justify-end gap-1 text-[10px]', inbound ? 'text-neutral-400' : 'text-white/70')}>
          <span>{formatClock(messageTime(message))}</span>
          {!inbound && <MessageDeliveryStatus state={message.state} />}
        </div>
        {!inbound && message.error_detail && <p className="mt-1 text-xs text-red-100">{message.error_detail}</p>}
      </div>
      {inbound && actions}
    </div>
  );
}

function MessageDeliveryStatus({ state }: { state: string }) {
  const indicator = deliveryIndicator(state);
  const Icon = indicator.kind === 'single-check' ? Check
    : indicator.kind === 'double-check' ? CheckCheck
      : indicator.kind === 'failed' ? CircleAlert
        : Clock3;
  return (
    <span className="inline-flex items-center gap-0.5" title={indicator.label} aria-label={indicator.label}>
      <span>· {state}</span><Icon className="h-3 w-3" />
    </span>
  );
}

function MessageDetails({ message, onClose }: { message: SmsItem | null; onClose: () => void }) {
  if (!message) return null;
  const inbound = isInbound(message);
  const parts = inbound ? message.parts : message.parts?.length;
  return (
    <Drawer direction="right" open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>Message details</DrawerTitle>
          <DrawerDescription>{inbound ? 'Received SMS' : 'Sent SMS'} · {formatExactDate(messageTime(message))}</DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 pb-6">
          <div className={cn('rounded-2xl px-4 py-3 text-sm', inbound ? 'border bg-neutral-50' : 'bg-brand-700 text-white')}>
            <p className="whitespace-pre-wrap break-words">{message.body}</p>
          </div>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-3 text-sm">
            <DetailRow label={inbound ? 'From' : 'To'} value={messagePeer(message)} />
            <DetailRow label={inbound ? 'Received' : 'Created'} value={formatExactDate(messageTime(message))} />
            {!inbound && <DetailRow label="Status" value={deliveryIndicator(message.state).label} />}
            <DetailRow label="Encoding" value={message.encoding.toUpperCase()} />
            <DetailRow label="Parts" value={parts == null ? 'Pending' : String(parts)} />
            {inbound && <DetailRow label="Complete" value={message.incomplete ? 'No' : 'Yes'} />}
            {!inbound && message.error_code && <DetailRow label="Error code" value={message.error_code} />}
            {!inbound && message.error_detail && <DetailRow label="Error" value={message.error_detail} />}
            <DetailRow label="Message ID" value={message.id} mono />
          </dl>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function DetailRow({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <><dt className="text-neutral-500">{label}</dt><dd className={cn('min-w-0 break-words text-neutral-900', mono && 'font-mono text-xs')}>{value}</dd></>;
}

function sameDay(a: string, b: string) {
  return new Date(a).toDateString() === new Date(b).toDateString();
}

function formatMessageDate(value: string) {
  const date = new Date(value);
  const today = new Date();
  if (date.toDateString() === today.toDateString()) return 'Today';
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: date.getFullYear() === today.getFullYear() ? undefined : 'numeric' }).format(date);
}

function formatClock(value: string) {
  return new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit' }).format(new Date(value));
}

function formatExactDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
