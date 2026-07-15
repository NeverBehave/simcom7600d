import { useEffect, useRef, useState } from 'react';
import { Headphones, Mic, MicOff, Volume2, VolumeX } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/auth/AuthProvider';
import { BrowserCallAudioSession, type CallAudioLevels, type CallAudioState } from './audio';

export function CallAudio({
  callID,
  autoConnect = false,
  onAutoConnectHandled,
}: {
  callID: string;
  autoConnect?: boolean;
  onAutoConnectHandled?: () => void;
}) {
  const { token } = useAuth();
  const session = useRef<BrowserCallAudioSession>();
  const [state, setState] = useState<CallAudioState>('disconnected');
  const [error, setError] = useState('');
  const [warning, setWarning] = useState('');
  const [toneNotice, setToneNotice] = useState('');
  const [hasMicrophone, setHasMicrophone] = useState(true);
  const [micMuted, setMicMuted] = useState(false);
  const [speakerMuted, setSpeakerMuted] = useState(false);
  const [levels, setLevels] = useState<CallAudioLevels>({ microphone: 0, caller: 0 });
  const autoConnectAttempted = useRef(false);

  useEffect(() => {
    autoConnectAttempted.current = false;
    return () => session.current?.stop();
  }, [callID]);

  function connect() {
    if (!token) return;
    session.current?.stop();
    setError('');
	setWarning('');
	setToneNotice('');
    const next = new BrowserCallAudioSession(
	  callID, token, setState, setError, setLevels, setWarning, setHasMicrophone,
	);
    session.current = next;
    next.setMicMuted(micMuted);
    next.setSpeakerMuted(speakerMuted);
    void next.start();
  }

  useEffect(() => {
    if (!autoConnect || autoConnectAttempted.current) return;
    autoConnectAttempted.current = true;
    connect();
    onAutoConnectHandled?.();
  }, [autoConnect, callID]);

  function disconnect() {
    session.current?.stop();
    session.current = undefined;
  }

  function toggleMic() {
    const muted = !micMuted;
    setMicMuted(muted);
    session.current?.setMicMuted(muted);
  }

  function toggleSpeaker() {
    const muted = !speakerMuted;
    setSpeakerMuted(muted);
    session.current?.setSpeakerMuted(muted);
  }

  function sendTestTone() {
	session.current?.sendTestTone();
	setToneNotice('Sending a one-second test tone to the caller.');
	window.setTimeout(() => setToneNotice(''), 1500);
  }

  const connected = state === 'live';
  const pending = state === 'requesting' || state === 'connecting';

  return (
    <section className="max-w-xl space-y-3 rounded-lg border bg-white p-4" aria-live="polite">
      <div className="flex items-center gap-2">
        <Headphones className="h-5 w-5" aria-hidden="true" />
        <h3 className="font-semibold">Browser audio</h3>
        <span className={`ml-auto rounded-full px-2 py-1 text-xs font-medium ${connected ? 'bg-green-100 text-green-800' : pending ? 'bg-amber-100 text-amber-800' : 'bg-neutral-100 text-neutral-700'}`}>
          {audioStateLabel(state)}
        </span>
      </div>
	  <p className="text-sm text-neutral-600">
		{!hasMicrophone && state === 'connecting'
		  ? 'Connecting caller audio in listen-only mode…'
		  : connected && !hasMicrophone
			? 'Caller audio is connected. Attach a microphone for conversation, or use the test tone to verify audio sent to the caller.'
			: audioStateHelp(state)}
	  </p>
      {error && <p className="rounded bg-red-50 p-2 text-sm text-red-800" role="alert">{error}</p>}
	  {warning && <p className="rounded bg-amber-50 p-2 text-sm text-amber-800" role="status">{warning}</p>}
	  {toneNotice && <p className="rounded bg-blue-50 p-2 text-sm text-blue-800" role="status">{toneNotice}</p>}
	  {connected && (
		<div className="grid grid-cols-2 gap-3 text-sm">
		  <AudioMeter
			label={!hasMicrophone ? 'Test signal to caller' : micMuted ? 'Your microphone (muted)' : 'Your microphone'}
			value={micMuted && hasMicrophone ? 0 : levels.microphone}
		  />
		  <AudioMeter label={speakerMuted ? 'Caller audio (speaker muted)' : 'Caller audio'} value={levels.caller} />
		</div>
	  )}
      <div className="flex flex-wrap gap-2">
		{!connected && !pending && <Button onClick={connect}>Connect call audio</Button>}
        {pending && <Button disabled>Connecting audio…</Button>}
        {connected && (
          <>
			<Button variant="outline" onClick={toggleMic} aria-pressed={micMuted} disabled={!hasMicrophone}>
              {micMuted ? <MicOff className="mr-2 h-4 w-4" /> : <Mic className="mr-2 h-4 w-4" />}
			  {!hasMicrophone ? 'Microphone unavailable' : micMuted ? 'Unmute microphone' : 'Mute microphone'}
            </Button>
            <Button variant="outline" onClick={toggleSpeaker} aria-pressed={speakerMuted}>
              {speakerMuted ? <VolumeX className="mr-2 h-4 w-4" /> : <Volume2 className="mr-2 h-4 w-4" />}
              {speakerMuted ? 'Unmute speaker' : 'Mute speaker'}
            </Button>
			<Button variant="outline" onClick={sendTestTone}>Send test tone to caller</Button>
            <Button variant="outline" onClick={disconnect}>Disconnect audio</Button>
          </>
        )}
      </div>
    </section>
  );
}

function AudioMeter({ label, value }: { label: string; value: number }) {
	const percent = Math.round(Math.max(0, Math.min(1, value)) * 100);
	return (
	  <div className="space-y-1">
		<div className="flex justify-between text-xs text-neutral-600"><span>{label}</span><span>{percent > 2 ? 'Active' : 'Quiet'}</span></div>
		<div
		  className="h-2 overflow-hidden rounded-full bg-neutral-200"
		  role="meter"
		  aria-label={label}
		  aria-valuemin={0}
		  aria-valuemax={100}
		  aria-valuenow={percent}
		>
		  <div className="h-full bg-green-500 transition-[width] duration-100" style={{ width: `${percent}%` }} />
		</div>
	  </div>
	);
}

function audioStateLabel(state: CallAudioState) {
  switch (state) {
    case 'requesting': return 'Waiting for permission';
    case 'connecting': return 'Connecting';
    case 'live': return 'Audio connected';
    case 'error': return 'Needs attention';
    default: return 'Not connected';
  }
}

function audioStateHelp(state: CallAudioState) {
  switch (state) {
    case 'requesting': return 'Allow microphone access in your browser to speak during this call.';
    case 'connecting': return 'Microphone is ready. Connecting it to the cellular call…';
    case 'live': return 'You can now hear the other person and speak through this browser.';
    case 'error': return 'Audio is not connected. Resolve the message below and try again.';
    default: return 'Connect audio now to hear ringback, then speak and listen when the call is answered.';
  }
}
