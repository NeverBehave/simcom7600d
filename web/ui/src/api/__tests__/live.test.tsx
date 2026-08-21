import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';

// Mock the SDK service surface; useLiveEvents will call api().events.list().
vi.mock('../client', async () => {
  const actual = await vi.importActual<typeof import('../client')>('../client');
  return {
    ...actual,
    api: () => ({
      events: { list: mockEventsList },
    }),
  };
});

let mockEventsList: ReturnType<typeof vi.fn>;
let shownNotifications: Array<{ title: string; options?: NotificationOptions }>;

import { useLiveEvents } from '../live';

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(qc, 'invalidateQueries');
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { wrapper, qc, invalidate };
}

beforeEach(() => {
	vi.useFakeTimers();
	mockEventsList = vi.fn();
	shownNotifications = [];
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => 'visible',
  });
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe('useLiveEvents', () => {
  it('polls events every 2500ms while visible', async () => {
    mockEventsList.mockResolvedValue({ data: { items: [] }, error: undefined });
    const { wrapper } = makeWrapper();
    renderHook(() => useLiveEvents(), { wrapper });

    await vi.advanceTimersByTimeAsync(0);
    expect(mockEventsList).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(2500);
    expect(mockEventsList).toHaveBeenCalledTimes(2);

    await vi.advanceTimersByTimeAsync(2500);
    expect(mockEventsList).toHaveBeenCalledTimes(3);
  });

	it('keeps polling when document is hidden so alerts still arrive', async () => {
    mockEventsList.mockResolvedValue({ data: { items: [] }, error: undefined });
    const { wrapper } = makeWrapper();
    renderHook(() => useLiveEvents(), { wrapper });

    await vi.advanceTimersByTimeAsync(0);
    expect(mockEventsList).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' });
		await vi.advanceTimersByTimeAsync(10_000);
		expect(mockEventsList).toHaveBeenCalledTimes(5);
  });

  it('advances since cursor to the highest seen id', async () => {
    mockEventsList
      .mockResolvedValueOnce({ data: { items: [{ id: 5, ts: 't', kind: 'sms.arrived' }] } })
      .mockResolvedValueOnce({ data: { items: [{ id: 9, ts: 't', kind: 'call.updated' }] } });
    const { wrapper } = makeWrapper();
    renderHook(() => useLiveEvents(), { wrapper });

    await vi.advanceTimersByTimeAsync(0);
    expect(mockEventsList).toHaveBeenNthCalledWith(1, { query: { since: 0, limit: 200 } });
    await vi.advanceTimersByTimeAsync(2500);
    expect(mockEventsList).toHaveBeenNthCalledWith(2, { query: { since: 5, limit: 200 } });
  });

  it('invalidates the matching queries for message, call, voicemail, and status events', async () => {
    mockEventsList.mockResolvedValueOnce({
      data: {
        items: [
          { id: 1, ts: 't', kind: 'sms.arrived' },
          { id: 2, ts: 't', kind: 'call.updated', ref_id: 'c-9' },
          { id: 3, ts: 't', kind: 'modem.status' },
          { id: 4, ts: 't', kind: 'voicemail.arrived', ref_id: 'vm-2' },
        ],
      },
    });
    const { wrapper, invalidate } = makeWrapper();
    renderHook(() => useLiveEvents(), { wrapper });

    await vi.advanceTimersByTimeAsync(0);
    await vi.runOnlyPendingTimersAsync();

    const keys = invalidate.mock.calls.map((c) => c[0]?.queryKey);
    expect(keys).toContainEqual(['sms']);
    expect(keys).toContainEqual(['calls']);
    expect(keys).toContainEqual(['calls', 'item', 'c-9']);
		expect(keys).toContainEqual(['status']);
		expect(keys).toContainEqual(['voicemails']);
		expect(keys).toContainEqual(['voicemails', 'item', 'vm-2']);
	});

	it('suppresses historical alerts, then notifies for a new call, SMS, and voicemail', async () => {
		class FakeNotification {
			static permission: NotificationPermission = 'granted';
			static requestPermission = vi.fn(async () => 'granted' as NotificationPermission);
			onclick: (() => void) | null = null;
			close = vi.fn();
			constructor(title: string, options?: NotificationOptions) {
				shownNotifications.push({ title, options });
			}
		}
		vi.stubGlobal('Notification', FakeNotification);
		mockEventsList
			.mockResolvedValueOnce({ data: { items: [{ id: 10, kind: 'sms.arrived', detail: '{"from":"+15550000000","body":"old"}' }] } })
			.mockResolvedValueOnce({ data: { items: [
				{ id: 11, kind: 'call.ringing', ref_id: 'call-1', detail: '{"from":"+15551234567"}' },
				{ id: 12, kind: 'sms.arrived', ref_id: 'sms-1', detail: '{"from":"+15559876543","body":"Hello there"}' },
				{ id: 13, kind: 'voicemail.arrived', ref_id: 'vm-1' },
			] } });
		const { wrapper } = makeWrapper();
		renderHook(() => useLiveEvents(), { wrapper });

		await vi.advanceTimersByTimeAsync(0);
		expect(shownNotifications).toHaveLength(0);
		await vi.advanceTimersByTimeAsync(2500);
		expect(shownNotifications).toHaveLength(3);
		expect(shownNotifications[0]).toMatchObject({
			title: 'Incoming call',
			options: { body: 'Call from (555) 123-4567', tag: 'call-call-1', requireInteraction: true },
		});
		expect(shownNotifications[1]).toMatchObject({
			title: 'Message from (555) 987-6543',
			options: { body: 'Hello there', tag: 'sms-sms-1' },
		});
		expect(shownNotifications[2]).toMatchObject({
			title: 'New voicemail',
			options: { body: 'A new voicemail is ready to play.', tag: 'voicemail-vm-1' },
		});
	});
});
