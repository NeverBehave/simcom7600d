import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerFooter,
} from '@/components/ui/drawer';
import { useSmsSend } from '@/api/queries';
import { toast } from 'sonner';
	import { useNavigate } from 'react-router-dom';
import { phoneKey } from '@/lib/phone';

export function Compose({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [to, setTo] = useState('');
  const [body, setBody] = useState('');
  const send = useSmsSend();
	const navigate = useNavigate();

  async function submit() {
    try {
		await send.mutateAsync({ to, body });
		toast.success(`Message submitted to ${to}`);
      setTo('');
      setBody('');
      onClose();
		navigate(`/sms/${encodeURIComponent(phoneKey(to) || to)}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <Drawer direction="right" open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>Compose SMS</DrawerTitle>
        </DrawerHeader>
        <div className="px-4 pb-4 space-y-3">
          <div className="space-y-1">
            <Label htmlFor="sms-to">To</Label>
            <Input
              id="sms-to"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder="+15551234567"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="sms-body">Body</Label>
            <Textarea
              id="sms-body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={5}
            />
            <div className="text-xs text-neutral-500">{body.length} chars</div>
			{send.isPending && <p className="text-sm text-neutral-600" aria-live="polite">Sending through the modem…</p>}
			{send.isError && <p className="text-sm text-red-700" role="alert">Send failed: {send.error.message}</p>}
          </div>
        </div>
        <DrawerFooter>
          <Button onClick={submit} disabled={!to || !body || send.isPending}>
            {send.isPending ? 'Sending…' : 'Send'}
          </Button>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
