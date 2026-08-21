import { useEffect, useState } from 'react';
import { Link, Route, Routes, useMatch, useNavigate, useParams } from 'react-router-dom';
import { ArrowLeft, Mail, MailOpen, PhoneCall, RefreshCw, Trash2 } from 'lucide-react';
import type { VoicemailJSON } from '@sim7600d/client/types';
import { useAuth } from '@/auth/AuthProvider';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/EmptyState';
import { RelativeTime } from '@/components/RelativeTime';
import { cn } from '@/lib/cn';
import { formatPhoneNumber } from '@/lib/phone';
import { useCallDial, useVoicemailDelete, useVoicemailItem, useVoicemailReadState, useVoicemailsList, useVoicemailSync } from '@/api/queries';
import { fetchVoicemailAudio } from './audio';

export default function Voicemail() {
  const detailOpen = !!useMatch('/voicemail/:id');
  return (
    <div className="h-full min-h-0 overflow-hidden md:grid md:grid-cols-[22rem_minmax(0,1fr)]">
      <Inbox className={detailOpen ? 'hidden md:flex' : 'flex'} />
      <div className={cn('h-full min-h-0 bg-neutral-50', !detailOpen && 'hidden md:block')}>
        <Routes>
          <Route index element={<EmptyState title="Select a voicemail" />} />
          <Route path=":id" element={<Detail />} />
        </Routes>
      </div>
    </div>
  );
}

function Inbox({ className }: { className?: string }) {
  const list = useVoicemailsList({ limit: 200 });
  const items = (list.data?.items ?? []) as VoicemailJSON[];
  const unread = items.filter((item) => !item.read).length;
  const sync = useVoicemailSync();
  return (
    <aside className={cn('h-full min-h-0 w-full flex-col border-r bg-white', className)}>
      <header className="flex h-16 shrink-0 items-center justify-between border-b px-4">
        <div>
          <h1 className="text-lg font-semibold">Voicemail</h1>
          <p className="text-xs text-neutral-500">{unread ? `${unread} unheard` : 'All caught up'}</p>
        </div>
        <button
          type="button"
          title="Sync voicemail"
          aria-label="Sync voicemail"
          disabled={sync.isPending}
          onClick={() => sync.mutate()}
          className="grid h-9 w-9 place-items-center rounded-md text-neutral-500 hover:bg-neutral-100 disabled:opacity-50"
        >
          <RefreshCw className={cn('h-4 w-4', sync.isPending && 'animate-spin')} />
        </button>
      </header>
      {sync.isError && <p className="border-b bg-red-50 px-4 py-2 text-xs text-red-800" role="alert">{sync.error.message}</p>}
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
        {list.isLoading && <p className="p-4 text-sm text-neutral-500">Loading voicemail…</p>}
        {list.isError && <p className="m-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="alert">Could not load voicemail: {list.error.message}</p>}
        {!list.isLoading && items.length === 0 && <EmptyState title="No voicemail" />}
        <ul className="py-2">
          {items.map((item) => (
            <li key={item.id} className="px-2 py-0.5">
              <Link to={`/voicemail/${item.id}`} className="flex items-center gap-3 rounded-md px-3 py-3 hover:bg-neutral-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500">
                <span className={cn('grid h-10 w-10 shrink-0 place-items-center rounded-full', item.read ? 'bg-neutral-100 text-neutral-500' : 'bg-brand-50 text-brand-700')}>
                  {item.read ? <MailOpen className="h-5 w-5" /> : <Mail className="h-5 w-5" />}
                </span>
                <span className="min-w-0 flex-1">
                  <span className={cn('block truncate', !item.read && 'font-semibold')}>{formatPhoneNumber(item.from)}</span>
                  <span className="block text-xs text-neutral-500"><RelativeTime ts={item.received_at} /> · {formatDuration(item.duration_ms)}</span>
                </span>
                {!item.read && <span className="h-2.5 w-2.5 rounded-full bg-brand-600" aria-label="Unheard" />}
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  );
}

function Detail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { token } = useAuth();
  const query = useVoicemailItem(id);
  const readState = useVoicemailReadState();
  const remove = useVoicemailDelete();
  const dial = useCallDial();
  const [audioURL, setAudioURL] = useState('');
  const [audioError, setAudioError] = useState('');
  const item = query.data as VoicemailJSON | undefined;

  useEffect(() => {
    if (!item?.audio_url || !token) return;
    let objectURL = '';
    let cancelled = false;
    setAudioError('');
    fetchVoicemailAudio(item.audio_url, token).then((blob) => {
      if (cancelled) return;
      objectURL = URL.createObjectURL(blob);
      setAudioURL(objectURL);
    }).catch((error) => {
      if (!cancelled) setAudioError(error instanceof Error ? error.message : String(error));
    });
    return () => {
      cancelled = true;
      if (objectURL) URL.revokeObjectURL(objectURL);
      setAudioURL('');
    };
  }, [item?.audio_url, token]);

  if (query.isLoading) return <p className="p-6 text-neutral-500">Loading voicemail…</p>;
  if (query.isError) return <p className="m-6 rounded-md border border-red-200 bg-red-50 p-4 text-red-800" role="alert">Could not load voicemail: {query.error.message}</p>;
  if (!item || !id) return <EmptyState title="Voicemail not found" />;

  async function deleteItem() {
    if (!window.confirm('Delete this voicemail? It will not return after the next sync.')) return;
    await remove.mutateAsync(id!);
    navigate('/voicemail', { replace: true });
  }

  async function callBack() {
    const call = await dial.mutateAsync({ to: item!.from });
    if (call?.id) navigate(`/calls/${call.id}`);
  }

  return (
    <article className="h-full min-h-0 overflow-y-auto overscroll-contain">
      <div className="mx-auto max-w-3xl space-y-5 p-4 sm:p-6">
        <Link to="/voicemail" className="inline-flex items-center gap-2 text-sm text-neutral-600 hover:text-neutral-950 md:hidden"><ArrowLeft className="h-4 w-4" /> Voicemail</Link>
        <header className="border-b pb-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h1 className="text-xl font-semibold">{formatPhoneNumber(item.from)}</h1>
              <p className="mt-1 text-sm text-neutral-500"><RelativeTime ts={item.received_at} /> · {formatDuration(item.duration_ms)}</p>
            </div>
            {!item.read && <span className="rounded-full bg-brand-50 px-2.5 py-1 text-xs font-medium text-brand-700">New</span>}
          </div>
        </header>

        <section aria-label="Voicemail playback">
          {audioURL ? (
            <audio className="w-full" controls preload="metadata" src={audioURL} onPlay={() => {
              if (!item.read && !readState.isPending) readState.mutate({ id, read: true });
            }} />
          ) : !audioError ? <p className="text-sm text-neutral-500">Preparing audio…</p> : null}
          {audioError && <p className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="alert">{audioError}</p>}
        </section>

        {item.transcript && (
          <section className="border-t pt-4">
            <h2 className="mb-2 text-sm font-semibold text-neutral-700">Transcript</h2>
            <p className="whitespace-pre-wrap text-sm leading-6 text-neutral-700">{item.transcript}</p>
          </section>
        )}

        <div className="flex flex-wrap gap-2 border-t pt-4">
          <Button onClick={callBack} disabled={dial.isPending || !item.from}><PhoneCall className="h-4 w-4" /> Call back</Button>
          <Button variant="outline" onClick={() => readState.mutate({ id, read: item.read ? false : true })} disabled={readState.isPending}>
            {item.read ? <Mail className="h-4 w-4" /> : <MailOpen className="h-4 w-4" />}{item.read ? 'Mark unheard' : 'Mark heard'}
          </Button>
          <Button variant="destructive" onClick={deleteItem} disabled={remove.isPending}><Trash2 className="h-4 w-4" /> Delete</Button>
        </div>
        {(readState.error || remove.error || dial.error) && <p className="rounded-md bg-red-50 p-3 text-sm text-red-800">{(readState.error || remove.error || dial.error)?.message}</p>}
      </div>
    </article>
  );
}

export function formatDuration(durationMS: number) {
  const totalSeconds = Math.max(0, Math.round(durationMS / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}
