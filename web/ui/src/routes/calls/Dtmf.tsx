import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { useCallDtmf } from '@/api/queries';

const KEYS = ['1','2','3','4','5','6','7','8','9','*','0','#'];

export function Dtmf({ id }: { id: string }) {
  const dtmf = useCallDtmf();
  const [feedback, setFeedback] = useState('Tap a key to send a tone.');
  const [failed, setFailed] = useState(false);

  async function send(key: string) {
    setFailed(false);
    setFeedback(`Sending ${key}…`);
    try {
      await dtmf.mutateAsync({ id, digits: key, durationMS: 200 });
      setFeedback(`Sent ${key}`);
    } catch (error) {
      setFailed(true);
      setFeedback(`Could not send ${key}: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  return (
    <div className="space-y-2">
      <div className="grid w-48 grid-cols-3 gap-2">
        {KEYS.map((k) => (
          <Button
            key={k}
            variant="outline"
            onClick={() => void send(k)}
            disabled={dtmf.isPending}
            aria-label={`Send DTMF ${k}`}
          >
            {k}
          </Button>
        ))}
      </div>
      <p className={`text-xs ${failed ? 'text-red-700' : 'text-neutral-600'}`} role={failed ? 'alert' : 'status'}>
        {feedback}
      </p>
    </div>
  );
}
