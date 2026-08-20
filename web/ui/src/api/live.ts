import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from './client';
import { formatPhoneNumber, phoneKey } from '@/lib/phone';

const POLL_MS = 2500;

export interface EventLike {
  id: number;
  kind: string;
  ref_id?: string;
  raw?: string;
  detail?: string;
}

export type WebNotificationPermission = NotificationPermission | 'unsupported';

export interface NotificationPayload {
  title: string;
  body: string;
  tag: string;
  href: string;
  requireInteraction?: boolean;
}

export interface LiveEventsController {
  notificationPermission: WebNotificationPermission;
  enableNotifications: () => Promise<void>;
}

/**
 * Polls /v1/events even while the tab is in the background. New SMS and
 * incoming-call events refresh their screens and, after the user grants
 * permission, produce native browser notifications.
 */
export function useLiveEvents(): LiveEventsController {
  const qc = useQueryClient();
  const lastSeen = useRef<number>(0);
  const initialized = useRef<boolean>(false);
  const inflight = useRef<boolean>(false);
  const [notificationPermission, setNotificationPermission] = useState<WebNotificationPermission>(currentNotificationPermission);

  const enableNotifications = useCallback(async () => {
    if (!('Notification' in window)) {
      setNotificationPermission('unsupported');
      return;
    }
    try {
      setNotificationPermission(await window.Notification.requestPermission());
    } catch {
      setNotificationPermission(currentNotificationPermission());
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function tick() {
      if (cancelled || inflight.current) return;
      inflight.current = true;
      try {
        const res = await api().events.list({
          query: { since: lastSeen.current, limit: 200 },
        } as any);
        if (cancelled) return;
        const events = (res.data as { items?: EventLike[] | null } | undefined)?.items ?? [];
        const shouldNotify = initialized.current;
        for (const ev of events) {
          if (ev.id > lastSeen.current) lastSeen.current = ev.id;
          for (const key of routeInvalidations(ev)) {
            qc.invalidateQueries({ queryKey: key });
          }
          if (shouldNotify) showWebNotification(ev);
        }
        // The first successful response establishes a cursor. Its historical
        // rows refresh the UI but must never trigger a burst of old alerts.
        initialized.current = true;
      } catch {
        // Swallow polling errors; 401 is handled by the global QueryCache handler.
      } finally {
        inflight.current = false;
      }
    }

    tick();
    const interval = setInterval(tick, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [qc]);

  return { notificationPermission, enableNotifications };
}

export function notificationPayload(ev: EventLike): NotificationPayload | null {
  const detail = parseDetail(ev.detail);
  if (ev.kind === 'call.ringing') {
    const from = detail.from || ev.raw || '';
    return {
      title: 'Incoming call',
      body: from ? `Call from ${formatPhoneNumber(from)}` : 'Unknown caller',
      tag: `call-${ev.ref_id || ev.id}`,
      href: ev.ref_id ? `/calls/${encodeURIComponent(ev.ref_id)}` : '/calls',
      requireInteraction: true,
    };
  }
  if (ev.kind === 'sms.arrived') {
    const from = detail.from || '';
    const thread = phoneKey(from) || from;
    return {
      title: from ? `Message from ${formatPhoneNumber(from)}` : 'New message',
      body: detail.body ? truncate(detail.body, 180) : 'You received a new text message.',
      tag: `sms-${ev.ref_id || ev.id}`,
      href: thread ? `/sms/${encodeURIComponent(thread)}` : '/sms',
    };
  }
  if (ev.kind === 'voicemail.arrived') {
    return {
      title: 'New voicemail',
      body: 'A new voicemail is ready to play.',
      tag: `voicemail-${ev.ref_id || ev.id}`,
      href: ev.ref_id ? `/voicemail/${encodeURIComponent(ev.ref_id)}` : '/voicemail',
    };
  }
  return null;
}

function showWebNotification(ev: EventLike): void {
  if (!('Notification' in window) || window.Notification.permission !== 'granted') return;
  const payload = notificationPayload(ev);
  if (!payload) return;
  try {
    const notification = new window.Notification(payload.title, {
      body: payload.body,
      tag: payload.tag,
      requireInteraction: payload.requireInteraction,
    });
    notification.onclick = () => {
      window.focus();
      window.location.assign(payload.href);
      notification.close();
    };
  } catch {
    // Some browsers expose Notification but disallow it outside a secure
    // context. The permission control remains available after HTTPS is used.
  }
}

function currentNotificationPermission(): WebNotificationPermission {
  return 'Notification' in window ? window.Notification.permission : 'unsupported';
}

function parseDetail(value: string | undefined): Record<string, string> {
  if (!value) return {};
  try {
    const parsed = JSON.parse(value) as unknown;
    if (!parsed || typeof parsed !== 'object') return {};
    return Object.fromEntries(Object.entries(parsed).filter((entry): entry is [string, string] => typeof entry[1] === 'string'));
  } catch {
    return {};
  }
}

function truncate(value: string, max: number): string {
  return value.length <= max ? value : `${value.slice(0, max - 1)}…`;
}

function routeInvalidations(ev: EventLike): readonly (readonly unknown[])[] {
  const keys: (readonly unknown[])[] = [];
  if (ev.kind.startsWith('sms.')) {
    keys.push(['sms']);
    if (ev.ref_id) keys.push(['sms', 'item', ev.ref_id]);
	} else if (ev.kind.startsWith('call.')) {
		keys.push(['calls']);
		if (ev.ref_id) keys.push(['calls', 'item', ev.ref_id]);
	} else if (ev.kind.startsWith('voicemail.')) {
		keys.push(['voicemails']);
		if (ev.ref_id) keys.push(['voicemails', 'item', ev.ref_id]);
	} else if (ev.kind.startsWith('modem.') || ev.kind.startsWith('net.')) {
    keys.push(['status']);
  }
  return keys;
}
