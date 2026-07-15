import { useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import type { CallForwardingRuleJSON } from '@sim7600d/client/types';
import { useCallForwardingStatus, useCallForwardingUpdate } from '@/api/queries';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Toaster } from '@/components/ui/sonner';
import { toast } from 'sonner';
import { cn } from '@/lib/cn';

const DETAILS: Record<CallForwardingRuleJSON['reason'], { title: string; description: string }> = {
  unconditional: {
    title: 'Always',
    description: 'Forward every incoming voice call without ringing this line.',
  },
  busy: {
    title: 'When busy',
    description: 'Forward when this line is already handling another call.',
  },
  no_reply: {
    title: 'When unanswered',
    description: 'Forward if nobody answers before the selected ring time.',
  },
  unreachable: {
    title: 'When unreachable',
    description: 'Forward when the modem is offline or outside cellular coverage.',
  },
};

export default function CallForwarding() {
  const [loaded, setLoaded] = useState(false);
  const status = useCallForwardingStatus(loaded);
  const [updatingReason, setUpdatingReason] = useState<CallForwardingRuleJSON['reason'] | null>(null);
  const rules = status.data?.items ?? [];
  const alwaysEnabled = rules.some((rule) => rule.reason === 'unconditional' && rule.available && rule.enabled);
  const carrierBusy = (loaded && status.isFetching) || updatingReason !== null;
  const carrierBusyMessage = status.isFetching
    ? 'Refreshing carrier status…'
    : updatingReason
      ? `Updating ${DETAILS[updatingReason].title.toLowerCase()} forwarding…`
      : '';

  return (
    <div className="p-6 space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-xl font-semibold">Call forwarding</h1>
          <p className="max-w-2xl text-sm text-neutral-600">
            These settings are stored by the mobile network. Changes apply to voice calls on this SIM even when the web service is not open.
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => loaded ? status.refetch() : setLoaded(true)}
          disabled={carrierBusy}
        >
          <RefreshCw size={16} className={status.isFetching ? 'mr-2 animate-spin' : 'mr-2'} />
          {status.isFetching ? 'Checking…' : loaded ? 'Refresh status' : 'Load status'}
        </Button>
      </header>

      {!loaded && (
        <div className="rounded-xl border bg-white p-5 text-sm text-neutral-600">
          <p className="font-medium text-neutral-900">Load carrier status when needed</p>
          <p className="mt-1">Reading forwarding rules temporarily occupies the modem command channel. This page waits until you choose Load status.</p>
        </div>
      )}

      {loaded && status.isError && (
        <div className="rounded border border-red-200 bg-red-50 p-4 text-sm text-red-800" role="alert">
          Could not read call-forwarding status: {status.error.message}
        </div>
      )}

      {loaded && status.isFetching && !status.isLoading && (
        <div className="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800" role="status">
          Refreshing carrier status. Forwarding controls are temporarily locked.
        </div>
      )}

      {loaded && alwaysEnabled && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" role="status">
          <strong>Always forwarding is active.</strong> Conditional forwarding settings are retained, but they will not be used and cannot be changed until Always is turned off.
        </div>
      )}

      {loaded && (status.isLoading ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2" aria-label="Loading call-forwarding status">
          {[0, 1, 2, 3].map((i) => <div key={i} className="h-64 animate-pulse rounded-lg bg-neutral-100" />)}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2" aria-busy={carrierBusy || undefined}>
          {rules.map((rule) => {
            const overriddenByAlways = alwaysEnabled && rule.reason !== 'unconditional';
            return (
              <RuleCard
                key={rule.reason}
                rule={rule}
                locked={carrierBusy || overriddenByAlways}
                lockReason={overriddenByAlways
                  ? 'Always forwarding is active. Turn it off to manage this conditional rule.'
                  : carrierBusyMessage}
                overriddenByAlways={overriddenByAlways}
                onUpdateStart={setUpdatingReason}
                onUpdateEnd={(reason) => setUpdatingReason((current) => current === reason ? null : current)}
              />
            );
          })}
        </div>
      ))}

      {loaded && !status.isLoading && !status.isError && rules.length === 0 && (
        <div className="rounded border bg-white p-6 text-sm text-neutral-600">No call-forwarding rules were returned by the modem.</div>
      )}
      <Toaster richColors position="bottom-right" />
    </div>
  );
}

export function RuleCard({
  rule,
  locked = false,
  lockReason = '',
  overriddenByAlways = false,
  onUpdateStart,
  onUpdateEnd,
}: {
  rule: CallForwardingRuleJSON;
  locked?: boolean;
  lockReason?: string;
  overriddenByAlways?: boolean;
  onUpdateStart?: (reason: CallForwardingRuleJSON['reason']) => void;
  onUpdateEnd?: (reason: CallForwardingRuleJSON['reason']) => void;
}) {
  const update = useCallForwardingUpdate();
  const [enabled, setEnabled] = useState(rule.enabled);
  const [number, setNumber] = useState(rule.number ?? '');
  const [timeout, setTimeoutSeconds] = useState(rule.timeout_seconds ?? 20);
  const detail = DETAILS[rule.reason];

  useEffect(() => {
    setEnabled(rule.enabled);
    setNumber(rule.number ?? '');
    setTimeoutSeconds(rule.timeout_seconds ?? 20);
  }, [rule.enabled, rule.number, rule.timeout_seconds, locked]);

  const dirty = !rule.available || enabled !== rule.enabled || (enabled && (
    number.trim() !== (rule.number ?? '') ||
    (rule.reason === 'no_reply' && timeout !== (rule.timeout_seconds ?? 20))
  ));
  const destinationMissing = enabled && number.trim() === '';

  async function save() {
    if (locked) return;
    onUpdateStart?.(rule.reason);
    try {
      await update.mutateAsync({
        reason: rule.reason,
        enabled,
        ...(enabled ? { number: number.trim() } : {}),
        ...(rule.reason === 'no_reply' ? { timeout_seconds: timeout } : {}),
      });
      toast.success(`${detail.title} forwarding updated`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      onUpdateEnd?.(rule.reason);
    }
  }

  return (
    <Card className={cn(locked && 'bg-neutral-50')} aria-disabled={locked || undefined}>
      <CardHeader className="space-y-2">
        <div className="flex items-center justify-between gap-3">
          <CardTitle>{detail.title}</CardTitle>
          <span className={`rounded-full px-2 py-1 text-xs font-medium ${!rule.available ? 'bg-amber-100 text-amber-800' : overriddenByAlways ? 'bg-amber-100 text-amber-800' : rule.enabled ? 'bg-green-100 text-green-800' : 'bg-neutral-100 text-neutral-600'}`}>
            {!rule.available ? 'Unavailable' : overriddenByAlways ? 'Inactive while Always is on' : rule.enabled ? 'Enabled' : 'Off'}
          </span>
        </div>
        <p className="text-sm font-normal text-neutral-600">{detail.description}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        {locked && lockReason && (
          <p className="rounded-lg border bg-white p-3 text-sm text-neutral-600" role="status">{lockReason}</p>
        )}
        <Label className="w-fit cursor-pointer">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
            disabled={locked || update.isPending}
            className="size-4 rounded border-neutral-300"
          />
          Forward calls
        </Label>

        <div className="space-y-2">
          <Label htmlFor={`forwarding-number-${rule.reason}`}>Destination number</Label>
          <Input
            id={`forwarding-number-${rule.reason}`}
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            placeholder="+1 202 555 0123"
            value={number}
            onChange={(event) => setNumber(event.target.value)}
            disabled={locked || update.isPending || !enabled}
            aria-invalid={destinationMissing || undefined}
          />
          {destinationMissing && <p className="text-xs text-red-700">Enter the number that should receive forwarded calls.</p>}
        </div>

        {rule.reason === 'no_reply' && (
          <div className="space-y-2">
            <Label htmlFor="forwarding-timeout">Ring time</Label>
            <select
              id="forwarding-timeout"
              value={timeout}
              onChange={(event) => setTimeoutSeconds(Number(event.target.value))}
              disabled={locked || update.isPending || !enabled}
              className="h-9 w-full rounded-md border border-input bg-transparent px-3 text-sm disabled:cursor-not-allowed disabled:opacity-50"
            >
              {[5, 10, 15, 20, 25, 30].map((seconds) => (
                <option key={seconds} value={seconds}>{seconds} seconds</option>
              ))}
            </select>
          </div>
        )}

        <div className="flex items-center justify-between gap-3 border-t pt-4">
          <span className={`text-xs ${rule.available ? 'text-neutral-500' : 'text-amber-700'}`} title={rule.error}>
            Current: {!rule.available
              ? 'Carrier status unavailable'
              : overriddenByAlways
                ? `Saved as ${rule.enabled ? `forwarding to ${rule.number || 'unknown number'}` : 'off'}; inactive while Always is on`
                : rule.enabled ? `Forwarding to ${rule.number || 'unknown number'}` : 'Not forwarding'}
          </span>
          <Button
            onClick={save}
            disabled={locked || !dirty || destinationMissing || update.isPending}
            aria-label={`Update ${detail.title}`}
          >
            {update.isPending ? 'Updating…' : 'Update'}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
