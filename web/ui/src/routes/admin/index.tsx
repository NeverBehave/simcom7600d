import { useEffect, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { useAdminAtReset, useAdminQueue, useAdminReconcile, useAdminVacuum } from '@/api/queries';
import { api } from '@/api/client';
import { AtConsole } from './AtConsole';
import { CodeBlock } from '@/components/CodeBlock';
import { toast } from 'sonner';
import { Toaster } from '@/components/ui/sonner';

/** Read feature flags without invoking either privileged operation. */
async function probeAvailability(): Promise<{ at: boolean; reset: boolean }> {
	const res = await api().admin.capabilities({} as any);
	if (res.error || !res.data) return { at: false, reset: false };
	return {
		at: res.data.at_passthrough,
		reset: res.data.modem_reset,
	};
}

export default function Admin() {
  const [caps, setCaps] = useState<{ at: boolean; reset: boolean } | null>(null);
  const reconcile = useAdminReconcile();
  const vacuum = useAdminVacuum();
  const atReset = useAdminAtReset();
  const queue = useAdminQueue({ refetchInterval: 4000 });
  const [reconcileMsg, setReconcileMsg] = useState<string | null>(null);

  useEffect(() => {
    probeAvailability().then(setCaps).catch(() => setCaps({ at: false, reset: false }));
  }, []);

  async function doReconcile() {
    try {
      await reconcile.mutateAsync();
      setReconcileMsg(`Last run: ${new Date().toLocaleTimeString()}`);
      toast.success('Reconcile complete');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  }

  async function doVacuum() {
    try {
      await vacuum.mutateAsync();
      toast.success('VACUUM complete');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  }

  async function doReset() {
    if (!confirm('Reboot the modem (AT+CFUN=1,1)? Active calls and SMS in flight will be interrupted.')) return;
    try {
      await atReset.mutateAsync();
      toast.success('Modem reset requested');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <div className="p-6 space-y-4">
      <h1 className="text-xl font-semibold">Admin</h1>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <CardHeader><CardTitle>Reconcile</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-neutral-600">Run the reconciler synchronously against the current modem state.</p>
            <Button onClick={doReconcile} disabled={reconcile.isPending}>
              {reconcile.isPending ? 'Running…' : 'Run reconcile'}
            </Button>
            {reconcileMsg && <div className="text-xs text-neutral-500">{reconcileMsg}</div>}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Queue</CardTitle></CardHeader>
          <CardContent>
            <CodeBlock>{queue.data ? JSON.stringify(queue.data, null, 2) : '—'}</CodeBlock>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>SQLite VACUUM</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-neutral-600">Compact the database file. Safe but may briefly hold a write lock.</p>
            <Button variant="outline" onClick={doVacuum} disabled={vacuum.isPending}>
              {vacuum.isPending ? 'Running…' : 'Run VACUUM'}
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Modem reset</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-neutral-600">
              Issues <code>AT+CFUN=1,1</code>. {caps && !caps.reset ? '(Disabled on this server: start with --allow-modem-reset.)' : ''}
            </p>
            <Button variant="destructive" onClick={doReset} disabled={!caps?.reset || atReset.isPending}>
              {atReset.isPending ? 'Resetting…' : 'Reset modem'}
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>AT console {caps && !caps.at && <span className="ml-2 text-sm text-neutral-500">(disabled — server started without --allow-at-passthrough)</span>}</CardTitle>
        </CardHeader>
        <CardContent>
          <AtConsole disabled={!caps?.at} />
        </CardContent>
      </Card>
      <Toaster richColors position="bottom-right" />
    </div>
  );
}
