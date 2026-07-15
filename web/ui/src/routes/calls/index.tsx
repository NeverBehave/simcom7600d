import { useState } from 'react';
import { Link, Route, Routes, useMatch, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
  ArrowDownLeft, ArrowLeft, ArrowUpRight, Clock3, Pause, Phone, PhoneCall,
  PhoneIncoming, PhoneMissed, PhoneOff, Play, UsersRound,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Toaster } from '@/components/ui/sonner';
import {
  useCallItem, useCallAnswer, useCallReject, useCallHangup, useCallsList,
  useCallDial, useCallHold, useCallResume, useCallMerge,
} from '@/api/queries';
import { StatusBadge } from '@/components/StatusBadge';
import { RelativeTime } from '@/components/RelativeTime';
import { EmptyState } from '@/components/EmptyState';
import { cn } from '@/lib/cn';
import { formatPhoneNumber } from '@/lib/phone';
import { Dialer } from './Dialer';
import { Dtmf } from './Dtmf';
import { CallAudio } from './CallAudio';
import { toast } from 'sonner';
import type { CallJSON } from '@sim7600d/client/types';

const ACTIVE_STATES = new Set(['ringing', 'dialing', 'alerting', 'active', 'held']);
const AUTO_AUDIO_STORAGE_KEY = 'sim7600d_auto_connect_call_audio';

export default function Calls() {
  const [params, setParams] = useSearchParams();
  const detailOpen = !!useMatch('/calls/:id');
  const dialOpen = params.get('dial') === '1';
  function setDial(v: boolean) {
    const p = new URLSearchParams(params);
    if (v) p.set('dial', '1'); else p.delete('dial');
    setParams(p, { replace: true });
  }
  return (
    <div className="h-full min-h-0 overflow-hidden md:grid md:grid-cols-[22rem_minmax(0,1fr)]">
      <List onDial={() => setDial(true)} className={detailOpen ? 'hidden md:flex' : 'flex'} />
      <div className={cn('h-full min-h-0 bg-neutral-50', !detailOpen && 'hidden md:block')}>
        <Routes>
          <Route index element={<EmptyState title="Select a call" />} />
          <Route path=":id" element={<Detail />} />
        </Routes>
      </div>
      <Dialer open={dialOpen} onClose={() => setDial(false)} />
      <Toaster richColors position="bottom-right" />
    </div>
  );
}

function List({ onDial, className }: { onDial: () => void; className?: string }) {
  const list = useCallsList({ limit: 200 });
  const all = (list.data?.items ?? []) as CallJSON[];
  const active = all.filter((c) => ACTIVE_STATES.has(c.state));
  const history = all.filter((c) => !ACTIVE_STATES.has(c.state));

  return (
    <aside className={cn('h-full min-h-0 w-full flex-col border-r bg-white', className)}>
      <div className="flex h-16 shrink-0 items-center justify-between border-b px-4">
        <div>
          <h1 className="text-lg font-semibold">Calls</h1>
          <p className="text-xs text-neutral-500">Cellular call history</p>
        </div>
        <Button size="sm" onClick={onDial}><Phone className="h-4 w-4" /> New call</Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
        {list.isLoading && <div className="p-4 text-sm text-neutral-500">Loading calls…</div>}
        {list.isError && <div className="m-3 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="alert">Could not load calls: {list.error.message}</div>}
        {active.length > 0 && (
          <section>
            <h2 className="px-4 pb-1 pt-4 text-xs font-semibold uppercase tracking-wide text-neutral-400">Now</h2>
            <ul>{active.map((c) => <Row key={c.id} call={c} />)}</ul>
          </section>
        )}
        <section>
          <h2 className="px-4 pb-1 pt-4 text-xs font-semibold uppercase tracking-wide text-neutral-400">Recent</h2>
          {history.length === 0 && !list.isLoading ? <EmptyState title="No calls yet" /> : (
            <ul>{history.map((c) => <Row key={c.id} call={c} />)}</ul>
          )}
        </section>
      </div>
    </aside>
  );
}

function Row({ call }: { call: CallJSON }) {
  const peer = call.direction === 'in' ? call.from : call.to;
  const active = ACTIVE_STATES.has(call.state);
  const missed = call.state === 'missed';
  const Icon = missed ? PhoneMissed : call.direction === 'in' ? ArrowDownLeft : ArrowUpRight;
  return (
    <li className="px-2 py-0.5">
      <Link to={`/calls/${call.id}`} className="flex items-center gap-3 rounded-xl px-3 py-3 hover:bg-neutral-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500">
        <span className={cn('grid h-10 w-10 shrink-0 place-items-center rounded-full', missed ? 'bg-red-50 text-red-600' : active ? 'bg-green-50 text-green-700' : 'bg-neutral-100 text-neutral-600')}>
          <Icon className="h-5 w-5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className={cn('block truncate font-medium', missed && 'text-red-700')}>{formatPhoneNumber(peer)}</span>
          <span className="block text-xs text-neutral-500"><RelativeTime ts={call.started_at} /> · {call.direction === 'in' ? 'Incoming' : 'Outgoing'}</span>
        </span>
        <StatusBadge value={call.state} />
      </Link>
    </li>
  );
}

function Detail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const q = useCallItem(id);
  const answer = useCallAnswer();
  const reject = useCallReject();
  const hangup = useCallHangup();
  const hold = useCallHold();
  const resume = useCallResume();
  const merge = useCallMerge();
  const dial = useCallDial();
  const calls = useCallsList({ limit: 50 });
  const call = q.data as CallJSON | undefined;
  const [autoConnectAudio, setAutoConnectAudio] = useStatePreference(AUTO_AUDIO_STORAGE_KEY);
  const [connectAudioAfterAnswer, setConnectAudioAfterAnswer] = useState(false);

  if (q.isLoading) return <div className="p-6 text-neutral-500">Loading…</div>;
  if (q.isError) return <div className="m-6 rounded-xl border border-red-200 bg-red-50 p-4 text-red-800" role="alert">Could not load call: {q.error.message}</div>;
  if (!call || !id) return <EmptyState title="Not found" />;

  const peer = call.direction === 'in' ? call.from : call.to;
  const canAnswer = call.state === 'ringing' && call.direction === 'in';
  const canReject = canAnswer;
  const canHangup = ACTIVE_STATES.has(call.state);
  const isActive = call.state === 'active';
  const isHeld = call.state === 'held';
  const openCalls = ((calls.data?.items ?? []) as CallJSON[]).filter((candidate) => ACTIVE_STATES.has(candidate.state));
  const hasOtherActive = openCalls.some((candidate) => candidate.id !== id && candidate.state === 'active');
  const hasOtherHeld = openCalls.some((candidate) => candidate.id !== id && candidate.state === 'held');
  const canMerge = (isActive && hasOtherHeld) || (isHeld && hasOtherActive);
  const isConference = isActive && openCalls.filter((candidate) => candidate.state === 'active').length > 1;
  const canConnectAudio = isActive || isHeld || (call.direction === 'out' && (call.state === 'dialing' || call.state === 'alerting'));
  const actionError = answer.error || reject.error || hangup.error || hold.error || resume.error || merge.error;

  async function callBack() {
    if (!peer) return;
    try {
      const next = await dial.mutateAsync({ to: peer });
      toast.success(`Calling ${formatPhoneNumber(peer)}`);
      if (next && typeof next === 'object' && 'id' in next) navigate(`/calls/${(next as { id: string }).id}`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    }
  }

  async function answerCall() {
    if (!id) return;
    try {
      await answer.mutateAsync(id);
      if (autoConnectAudio) setConnectAudioAfterAnswer(true);
    } catch {
      // The mutation exposes its error in the call action alert below.
    }
  }

  return (
    <article className="h-full min-h-0 overflow-y-auto overscroll-contain">
      <div className="mx-auto max-w-3xl space-y-5 p-4 sm:p-6">
        <Link to="/calls" className="inline-flex items-center gap-2 text-sm text-neutral-600 hover:text-neutral-950 md:hidden">
          <ArrowLeft className="h-4 w-4" /> Calls
        </Link>

        <header className="rounded-2xl border bg-white px-5 py-6 text-center shadow-sm">
          <span className={cn('mx-auto mb-3 grid h-16 w-16 place-items-center rounded-full', isActive ? 'bg-green-100 text-green-700' : isHeld ? 'bg-amber-100 text-amber-700' : 'bg-neutral-100 text-neutral-600')}>
            {call.state === 'ringing' ? <PhoneIncoming className="h-7 w-7" /> : ACTIVE_STATES.has(call.state) ? <PhoneCall className="h-7 w-7" /> : <PhoneOff className="h-7 w-7" />}
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">{formatPhoneNumber(peer)}</h1>
          <div className="mt-2 flex items-center justify-center gap-2 text-sm text-neutral-500">
            <StatusBadge value={call.state} />
            <span>·</span>
            <span>{call.direction === 'in' ? 'Incoming' : 'Outgoing'} <RelativeTime ts={call.started_at} /></span>
          </div>
          <p className="mx-auto mt-4 max-w-lg text-sm text-neutral-600" aria-live="polite">{callStatusText(call.state, isConference)}</p>

          <div className="mt-5 flex flex-wrap justify-center gap-3">
            {canAnswer && <Button size="lg" disabled={answer.isPending} onClick={answerCall}><Phone className="h-5 w-5" />{answer.isPending ? 'Answering…' : 'Answer'}</Button>}
            {canReject && <Button variant="destructive" size="lg" disabled={reject.isPending} onClick={() => reject.mutate(id)}><PhoneOff className="h-5 w-5" />{reject.isPending ? 'Rejecting…' : 'Reject'}</Button>}
            {isActive && <Button variant="outline" size="lg" disabled={hold.isPending} onClick={() => hold.mutate(id)}><Pause className="h-5 w-5" />{hold.isPending ? 'Holding…' : 'Hold'}</Button>}
            {isHeld && <Button size="lg" disabled={resume.isPending} onClick={() => resume.mutate(id)}><Play className="h-5 w-5" />{resume.isPending ? 'Resuming…' : 'Resume'}</Button>}
            {canMerge && <Button variant="outline" size="lg" disabled={merge.isPending} onClick={() => merge.mutate(id)}><UsersRound className="h-5 w-5" />{merge.isPending ? 'Merging…' : 'Merge calls'}</Button>}
            {canHangup && !canReject && <Button variant="destructive" size="lg" disabled={hangup.isPending} onClick={() => hangup.mutate(id)}><PhoneOff className="h-5 w-5" />{hangup.isPending ? 'Hanging up…' : 'Hang up'}</Button>}
            {!canHangup && peer && <Button size="lg" disabled={dial.isPending} onClick={callBack}><PhoneCall className="h-5 w-5" />{dial.isPending ? 'Calling…' : 'Call back'}</Button>}
          </div>
          {call.direction === 'in' && ACTIVE_STATES.has(call.state) && (
            <label className="mx-auto mt-4 flex w-fit cursor-pointer items-center gap-2 text-sm text-neutral-600">
              <input
                type="checkbox"
                checked={autoConnectAudio}
                onChange={(event) => setAutoConnectAudio(event.target.checked)}
                className="size-4 rounded border-neutral-300"
              />
              Automatically connect browser audio after answering
            </label>
          )}
          {actionError && <p className="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-800" role="alert">The call action failed: {actionError.message}</p>}
        </header>

        {canConnectAudio && (
          <CallAudio
            callID={id}
            autoConnect={connectAudioAfterAnswer}
            onAutoConnectHandled={() => setConnectAudioAfterAnswer(false)}
          />
        )}

        {isActive && (
          <section className="rounded-2xl border bg-white p-4 shadow-sm">
            <h2 className="mb-3 text-sm font-semibold text-neutral-700">Keypad</h2>
            <Dtmf id={id} />
          </section>
        )}

        {(call.end_reason || call.duration_ms) && (
          <section className="rounded-2xl border bg-white p-4 text-sm shadow-sm">
            <h2 className="mb-3 flex items-center gap-2 font-semibold"><Clock3 className="h-4 w-4" /> Call details</h2>
            <dl className="grid grid-cols-[auto_1fr] gap-x-5 gap-y-2">
              {call.end_reason && <><dt className="text-neutral-500">Ended because</dt><dd>{call.end_reason.replace(/_/g, ' ')}</dd></>}
              {!!call.duration_ms && <><dt className="text-neutral-500">Connected time</dt><dd>{formatDuration(call.duration_ms)}</dd></>}
            </dl>
          </section>
        )}
      </div>
    </article>
  );
}

function useStatePreference(key: string) {
  const [value, setValue] = useState(() => window.localStorage.getItem(key) === 'true');
  function update(next: boolean) {
    setValue(next);
    window.localStorage.setItem(key, String(next));
  }
  return [value, update] as const;
}

function formatDuration(durationMS: number) {
  const total = Math.round(durationMS / 1000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return minutes ? `${minutes}m ${seconds}s` : `${seconds}s`;
}

function callStatusText(state: string, conference = false) {
  if (conference) return 'Conference call in progress.';
  switch (state) {
    case 'dialing': return 'The modem is placing the call.';
    case 'alerting': return 'The other phone is ringing.';
    case 'ringing': return 'Incoming call — answer or reject it.';
    case 'active': return 'Call connected. Browser audio stays connected if you place the call on hold.';
    case 'held': return 'The other person is on hold. Resume when you are ready to continue.';
    case 'missed': return 'You missed this incoming call. Call back with one tap.';
    case 'rejected': return 'This incoming call was declined.';
    default: return 'This call has ended. You can call this number again.';
  }
}
