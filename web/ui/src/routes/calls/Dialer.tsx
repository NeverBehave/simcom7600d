import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerFooter,
} from '@/components/ui/drawer';
import { useCallDial } from '@/api/queries';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';

export function Dialer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [to, setTo] = useState('');
  const dial = useCallDial();
  const navigate = useNavigate();

  async function submit() {
    try {
      const c = await dial.mutateAsync({ to });
		toast.success(`Calling ${to}`);
      onClose();
      if (c && typeof c === 'object' && 'id' in c) navigate(`/calls/${(c as { id: string }).id}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <Drawer direction="right" open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DrawerContent>
        <DrawerHeader><DrawerTitle>Dial</DrawerTitle></DrawerHeader>
        <div className="px-4 pb-4 space-y-3">
          <Label htmlFor="dial-to">Number</Label>
          <Input id="dial-to" value={to} onChange={(e) => setTo(e.target.value)} placeholder="+15551234567" autoFocus />
		  <p className="text-sm text-neutral-500" aria-live="polite">
			{dial.isPending ? 'Asking the modem to place the call…' : 'The call page will show dialing, ringing, active, and ended states.'}
		  </p>
		  {dial.isError && <p className="text-sm text-red-700" role="alert">Call failed: {dial.error.message}</p>}
        </div>
        <DrawerFooter>
          <Button onClick={submit} disabled={!to || dial.isPending}>
            {dial.isPending ? 'Dialing…' : 'Dial'}
          </Button>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
