import { useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useEventsList } from '@/api/queries';
import { RelativeTime } from '@/components/RelativeTime';
import { EmptyState } from '@/components/EmptyState';
import { CodeBlock } from '@/components/CodeBlock';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { EventJSON } from '@sim7600d/client/types';

const KIND_GROUPS = ['(all)', 'sms.', 'call.', 'modem.', 'net.'];

export default function Events() {
  const [kindPrefix, setKindPrefix] = useState('(all)');
  const [q, setQ] = useState('');
  const [autoTail, setAutoTail] = useState(true);
  const list = useEventsList({ limit: 500 });
  const queryClient = useQueryClient();

  useQuery({
    queryKey: ['events', 'tail'],
    queryFn: async () => { await queryClient.invalidateQueries({ queryKey: ['events'] }); return 0; },
    refetchInterval: autoTail ? 3000 : false,
    enabled: autoTail,
  });

  const filtered = useMemo(() => {
    const items = (list.data?.items ?? []) as EventJSON[];
    return items.filter((e) => {
      if (kindPrefix !== '(all)' && !e.kind.startsWith(kindPrefix)) return false;
      if (q) {
        const hay = `${e.kind} ${e.detail ?? ''} ${e.ref_id ?? ''} ${e.raw ?? ''}`.toLowerCase();
        if (!hay.includes(q.toLowerCase())) return false;
      }
      return true;
    });
  }, [list.data, kindPrefix, q]);

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">Events</h1>
        <div className="flex items-center gap-2">
          <Select value={kindPrefix} onValueChange={setKindPrefix}>
            <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
            <SelectContent>{KIND_GROUPS.map((k) => <SelectItem key={k} value={k}>{k === '(all)' ? 'All kinds' : `${k}*`}</SelectItem>)}</SelectContent>
          </Select>
          <Input placeholder="Search…" value={q} onChange={(e) => setQ(e.target.value)} className="w-64" />
          <Button variant={autoTail ? 'default' : 'outline'} onClick={() => setAutoTail((v) => !v)}>
            {autoTail ? 'Auto-tail on' : 'Auto-tail off'}
          </Button>
        </div>
      </div>
      {filtered.length === 0 ? (
        <EmptyState title="No events match" />
      ) : (
        <ul className="bg-white rounded-md border divide-y">
          {filtered.map((e) => (
            <li key={e.id} className="p-3">
              <div className="flex justify-between gap-3">
                <span className="font-mono text-sm">{e.kind}</span>
                <span className="text-xs text-neutral-500"><RelativeTime ts={e.ts} /></span>
              </div>
              {e.detail && <div className="text-sm mt-1">{e.detail}</div>}
              {e.raw && (
                <details className="mt-2">
                  <summary className="text-xs text-neutral-500 cursor-pointer">Raw payload</summary>
                  <div className="mt-2"><CodeBlock>{e.raw}</CodeBlock></div>
                </details>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
