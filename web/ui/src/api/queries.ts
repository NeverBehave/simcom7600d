import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { unwrap, api } from './client';
import { newIdempotencyKey } from '@/lib/idempotency';
import type {
  SmsListData,
  CallsListData,
  EventsListData,
  CallForwardingRuleJSON,
  VoicemailJSON,
  VoicemailsListData,
} from '@sim7600d/client/types';

export const qk = {
  status: ['status'] as const,
  callForwarding: ['call-forwarding'] as const,
  sms: (params?: SmsListData['query']) => ['sms', params ?? {}] as const,
  smsItem: (id: string) => ['sms', 'item', id] as const,
  calls: (params?: CallsListData['query']) => ['calls', params ?? {}] as const,
  callsItem: (id: string) => ['calls', 'item', id] as const,
  voicemails: (params?: VoicemailsListData['query']) => ['voicemails', params ?? {}] as const,
  voicemailItem: (id: string) => ['voicemails', 'item', id] as const,
  events: (params?: EventsListData['query']) => ['events', params ?? {}] as const,
  adminQueue: ['admin', 'queue'] as const,
};

export function useStatus(opts?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: qk.status,
    queryFn: async () => unwrap(await api().status.get({} as any)),
    refetchInterval: opts?.refetchInterval,
  });
}

export function useCallForwardingStatus(enabled = true) {
  return useQuery({
    queryKey: qk.callForwarding,
    queryFn: async () => unwrap(await api().callForwarding.get({} as any)),
    enabled,
  });
}

export function useCallForwardingUpdate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (rule: Pick<CallForwardingRuleJSON, 'reason' | 'enabled' | 'number' | 'timeout_seconds'>) =>
      unwrap(await api().callForwarding.update({
        path: { reason: rule.reason },
        body: {
          enabled: rule.enabled,
          ...(rule.enabled && rule.number ? { number: rule.number } : {}),
          ...(rule.reason === 'no_reply' && rule.timeout_seconds
            ? { timeout_seconds: rule.timeout_seconds }
            : {}),
        },
      } as any)),
    onSuccess: (updated) => {
      qc.setQueryData(qk.callForwarding, (current: { items: CallForwardingRuleJSON[] | null } | undefined) => {
        if (!current?.items) return current;
        return {
          ...current,
          items: current.items.map((rule) => rule.reason === updated.reason ? updated : rule),
        };
      });
      qc.invalidateQueries({ queryKey: qk.callForwarding });
    },
  });
}

export function useSmsList(params?: SmsListData['query']) {
  return useQuery({
    queryKey: qk.sms(params),
    queryFn: async () => unwrap(await api().sms.list({ query: params } as any)),
	refetchInterval: 5000,
  });
}

export function useSmsItem(id: string | undefined) {
  return useQuery({
    queryKey: qk.smsItem(id ?? ''),
    enabled: !!id,
    queryFn: async () => unwrap(await api().sms.get({ path: { id: id! } } as any)),
  });
}

export function useCallsList(params?: CallsListData['query']) {
  return useQuery({
    queryKey: qk.calls(params),
    queryFn: async () => unwrap(await api().calls.list({ query: params } as any)),
	refetchInterval: 2000,
  });
}

export function useCallItem(id: string | undefined) {
  return useQuery({
    queryKey: qk.callsItem(id ?? ''),
    enabled: !!id,
    queryFn: async () => unwrap(await api().calls.get({ path: { id: id! } } as any)),
	refetchInterval: 1000,
  });
}

export function useVoicemailsList(params?: VoicemailsListData['query']) {
  return useQuery({
    queryKey: qk.voicemails(params),
    queryFn: async () => unwrap(await api().voicemails.list({ query: params } as any)),
    refetchInterval: 15000,
  });
}

export function useVoicemailItem(id: string | undefined) {
  return useQuery({
    queryKey: qk.voicemailItem(id ?? ''),
    enabled: !!id,
    queryFn: async () => unwrap(await api().voicemails.get({ path: { id: id! } } as any)),
  });
}

export function useVoicemailReadState() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, read }: { id: string; read: boolean }) =>
      unwrap(await api().voicemails.update({ path: { id }, body: { read } } as any)),
    onSuccess: (updated: VoicemailJSON, input) => {
      qc.setQueryData(qk.voicemailItem(input.id), updated);
      qc.invalidateQueries({ queryKey: ['voicemails'] });
    },
  });
}

export function useVoicemailDelete() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api().voicemails.delete({ path: { id } } as any)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['voicemails'] }),
  });
}

export function useVoicemailSync() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api().voicemails.sync({} as any)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['voicemails'] }),
  });
}

export function useEventsList(params?: EventsListData['query']) {
  return useQuery({
    queryKey: qk.events(params),
    queryFn: async () => unwrap(await api().events.list({ query: params } as any)),
  });
}

export function useAdminQueue(opts?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: qk.adminQueue,
    queryFn: async () => unwrap(await api().admin.queue({} as any)),
    refetchInterval: opts?.refetchInterval,
  });
}

export function useSmsSend() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { to: string; body: string }) =>
      unwrap(
        await api().sms.send({
          body: input,
          headers: { 'Idempotency-Key': newIdempotencyKey() },
        } as any),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['sms'] }),
  });
}

export function useSmsDelete() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(await api().sms.delete({ path: { id } } as any)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['sms'] }),
  });
}

export function useCallDial() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { to: string }) =>
      unwrap(
        await api().calls.dial({
          body: input,
          headers: { 'Idempotency-Key': newIdempotencyKey() },
        } as any),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['calls'] }),
  });
}

export function useCallAnswer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api().calls.answer({ path: { id } } as any)),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ['calls'] });
      qc.invalidateQueries({ queryKey: ['calls', 'item', id] });
    },
  });
}

export function useCallReject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api().calls.reject({ path: { id } } as any)),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ['calls'] });
      qc.invalidateQueries({ queryKey: ['calls', 'item', id] });
    },
  });
}

export function useCallHangup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api().calls.hangup({ path: { id } } as any)),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ['calls'] });
      qc.invalidateQueries({ queryKey: ['calls', 'item', id] });
    },
  });
}

function useCallStateAction(action: 'hold' | 'resume') {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api().calls[action]({ path: { id } } as any)),
    onSuccess: (updated, id) => {
      qc.setQueryData(qk.callsItem(id), updated);
      qc.invalidateQueries({ queryKey: ['calls'] });
    },
  });
}

export function useCallHold() {
  return useCallStateAction('hold');
}

export function useCallResume() {
  return useCallStateAction('resume');
}

export function useCallMerge() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api().calls.merge({ path: { id } } as any)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['calls'] }),
  });
}

export function useCallDtmf() {
  return useMutation({
    mutationFn: async (input: { id: string; digits: string; durationMS?: number }) =>
      unwrap(await api().calls.dtmf({
        path: { id: input.id },
        body: { digits: input.digits, duration_ms: input.durationMS ?? 200 },
      } as any)),
  });
}

export function useAdminReconcile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api().admin.reconcile({} as any)),
    onSuccess: () => qc.invalidateQueries(),
  });
}

export function useAdminVacuum() {
  return useMutation({
    mutationFn: async () => unwrap(await api().admin.vacuum({} as any)),
  });
}

export function useAdminAt() {
  return useMutation({
    mutationFn: async (input: { cmd: string; timeout_ms?: number }) =>
      unwrap(await api().admin.at({ body: input } as any)),
  });
}

export function useAdminAtReset() {
  return useMutation({
    mutationFn: async () => unwrap(await api().admin.atReset({} as any)),
  });
}
