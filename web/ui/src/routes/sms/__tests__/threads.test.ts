import { describe, expect, it } from 'vitest';
import { deliveryIndicator, groupMessages, type SmsItem } from '..';

describe('SMS conversations', () => {
  it('combines inbound and outbound messages by normalized number', () => {
    const messages: SmsItem[] = [
      {
        id: 'in-1', direction: 'in', from: '+1 (415) 555-0100', body: 'Hello',
        received_at: '2026-07-14T18:00:00Z', smsc_ts: '2026-07-14T18:00:00Z',
        parts: 1, encoding: 'gsm7', incomplete: false,
      },
      {
        id: 'out-1', direction: 'out', to: '4155550100', body: 'Hi back', state: 'accepted',
        ts: '2026-07-14T18:01:00Z', parts: [], encoding: 'gsm7',
      },
    ];

    const threads = groupMessages(messages);
    expect(threads).toHaveLength(1);
    expect(threads[0].messages.map((message) => message.id)).toEqual(['in-1', 'out-1']);
    expect(threads[0].latest.id).toBe('out-1');
  });

  it('orders conversations by their most recent message', () => {
    const messages: SmsItem[] = [
      {
        id: 'older', direction: 'out', to: '+14155550100', body: 'Older', state: 'accepted',
        ts: '2026-07-14T18:00:00Z', parts: [], encoding: 'gsm7',
      },
      {
        id: 'newer', direction: 'out', to: '+12125550100', body: 'Newer', state: 'accepted',
        ts: '2026-07-14T19:00:00Z', parts: [], encoding: 'gsm7',
      },
    ];

    expect(groupMessages(messages).map((thread) => thread.latest.id)).toEqual(['newer', 'older']);
  });
});

describe('SMS delivery indicators', () => {
  it('uses one check while queued and two after submission', () => {
    expect(deliveryIndicator('queued').kind).toBe('single-check');
    expect(deliveryIndicator('submitted').kind).toBe('double-check');
    expect(deliveryIndicator('accepted').kind).toBe('double-check');
    expect(deliveryIndicator('delivered').kind).toBe('double-check');
  });

  it('does not show checks for failed or unknown delivery', () => {
    expect(deliveryIndicator('failed').kind).toBe('failed');
    expect(deliveryIndicator('indeterminate').kind).toBe('pending');
  });
});
