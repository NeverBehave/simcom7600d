import { Link } from 'react-router-dom';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { useStatus, useEventsList } from '@/api/queries';
import { RelativeTime } from '@/components/RelativeTime';
import { EmptyState } from '@/components/EmptyState';

function Bars({ pct }: { pct: number }) {
  // pct: 0-100. Renders 5 bars of increasing height.
  const lit = Math.max(0, Math.min(5, Math.round((pct / 100) * 5)));
  return (
    <div className="flex items-end gap-0.5 h-5">
      {[1, 2, 3, 4, 5].map((i) => (
        <div
          key={i}
          className={`w-1.5 rounded-sm ${i <= lit ? 'bg-brand-500' : 'bg-neutral-300'}`}
          style={{ height: `${20 + i * 16}%` }}
        />
      ))}
    </div>
  );
}

export default function Dashboard() {
  const status = useStatus({ refetchInterval: 5000 });
  const events = useEventsList({ limit: 20 });

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Dashboard</h1>
        <div className="flex gap-2">
          <Button asChild variant="outline"><Link to="/sms?compose=1">Send SMS</Link></Button>
          <Button asChild><Link to="/calls?dial=1">Dial</Link></Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardHeader><CardTitle>Network</CardTitle></CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span>Registration</span>
              <span className="font-mono">{status.data?.network?.registered !== undefined ? (status.data.network.registered ? 'Registered' : 'Not registered') : '—'}</span>
            </div>
            <div className="flex justify-between">
              <span>Technology</span>
              <span className="font-mono">{status.data?.network?.tech ?? '—'}</span>
            </div>
            <div className="flex justify-between">
              <span>Band</span>
              <span className="font-mono">{status.data?.network?.band ?? '—'}</span>
            </div>
            <div className="flex items-center justify-between">
              <span>Signal</span>
              <div className="flex items-center gap-2">
                <span className="font-mono">{status.data?.network?.rsrp_dbm ?? '—'} dBm</span>
                <Bars pct={rsrpToPct(status.data?.network?.rsrp_dbm)} />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>SIM</CardTitle></CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between"><span>State</span><span className="font-mono">{status.data?.sim?.state ?? '—'}</span></div>
            <div className="flex justify-between"><span>Operator</span><span className="font-mono">{status.data?.sim?.operator ?? '—'}</span></div>
            <div className="flex justify-between"><span>IMSI</span><span className="font-mono">{status.data?.sim?.imsi ?? '—'}</span></div>
            <div className="flex justify-between"><span>ICCID</span><span className="font-mono">{status.data?.sim?.iccid ?? '—'}</span></div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Modem</CardTitle></CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between"><span>Model</span><span className="font-mono">{status.data?.modem?.model ?? '—'}</span></div>
            <div className="flex justify-between"><span>IMEI</span><span className="font-mono">{status.data?.modem?.imei ?? '—'}</span></div>
            <div className="flex justify-between"><span>Firmware</span><span className="font-mono">{status.data?.modem?.firmware ?? '—'}</span></div>
            <div className="flex justify-between"><span>Uptime</span><span className="font-mono">{status.data?.uptime_s !== undefined ? `${status.data.uptime_s}s` : '—'}</span></div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle>Recent events</CardTitle></CardHeader>
        <CardContent>
          {events.isLoading ? (
            <div className="space-y-2">{[...Array(5)].map((_, i) => <div key={i} className="h-6 bg-neutral-100 rounded animate-pulse" />)}</div>
          ) : events.data?.items?.length ? (
            <ul className="text-sm divide-y">
              {events.data.items.slice(0, 20).map((e) => (
                <li key={e.id} className="py-2 flex justify-between gap-4">
                  <span className="font-mono text-neutral-600">{e.kind}</span>
                  <span className="text-neutral-500"><RelativeTime ts={e.ts} /></span>
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState title="No events yet" />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function rsrpToPct(rsrp: number | undefined): number {
  if (rsrp === undefined) return 0;
  // RSRP from -140 (worst) to -44 (best). Clamp and convert.
  const clamped = Math.max(-140, Math.min(-44, rsrp));
  return Math.round(((clamped + 140) / (140 - 44)) * 100);
}
