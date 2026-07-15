import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
  refetch: vi.fn(),
  mutateAsync: vi.fn(),
  statusEnabled: vi.fn(),
  status: {
    data: { items: [] as Array<Record<string, unknown>> },
    isLoading: false,
    isFetching: false,
    isError: false,
    error: new Error('status error'),
  },
}));

vi.mock('@/api/queries', () => ({
  useCallForwardingStatus: (enabled: boolean) => {
    mocks.statusEnabled(enabled);
    return { ...mocks.status, refetch: mocks.refetch };
  },
  useCallForwardingUpdate: () => ({
    mutateAsync: mocks.mutateAsync,
    isPending: false,
  }),
}));

vi.mock('@/components/ui/sonner', () => ({ Toaster: () => null }));

import CallForwarding, { RuleCard } from '..';

beforeEach(() => {
  mocks.refetch.mockReset();
  mocks.mutateAsync.mockReset();
  mocks.statusEnabled.mockReset();
  mocks.mutateAsync.mockResolvedValue(undefined);
  mocks.status.data = {
    items: [
      { reason: 'unconditional', enabled: false, available: true },
      { reason: 'busy', enabled: false, available: true },
      { reason: 'no_reply', enabled: true, number: '+12025550123', timeout_seconds: 20, available: true },
      { reason: 'unreachable', enabled: false, available: true },
    ],
  };
  mocks.status.isLoading = false;
  mocks.status.isFetching = false;
  mocks.status.isError = false;
});

describe('CallForwarding', () => {
  it('shows the network status and submits a changed rule', async () => {
    render(<CallForwarding />);
    expect(mocks.statusEnabled).toHaveBeenLastCalledWith(false);
    expect(screen.queryByText(/Forwarding to \+12025550123/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Load status' }));
    expect(mocks.statusEnabled).toHaveBeenLastCalledWith(true);

    expect(screen.getByText(/Forwarding to \+12025550123/)).toBeVisible();
    const busyCard = screen.getByText('When busy').closest('[data-slot="card"]');
    expect(busyCard).not.toBeNull();

    await userEvent.click(withinCard(busyCard!, 'checkbox'));
    await userEvent.type(withinCard(busyCard!, 'textbox'), '+1 202 555 0199');
    await userEvent.click(screen.getByRole('button', { name: 'Update When busy' }));

    expect(mocks.mutateAsync).toHaveBeenCalledWith({
      reason: 'busy', enabled: true, number: '+1 202 555 0199',
    });
  });

  it('can fetch fresh carrier status on demand', async () => {
    render(<CallForwarding />);
    await userEvent.click(screen.getByRole('button', { name: 'Load status' }));
    expect(mocks.refetch).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Refresh status' }));
    expect(mocks.refetch).toHaveBeenCalledOnce();
  });

  it('shows unknown carrier state without guessing that forwarding is off', () => {
    render(<RuleCard rule={{
      reason: 'unreachable', enabled: false, available: false, error: 'network timeout',
    }} />);

    expect(screen.getByText('Unavailable')).toBeVisible();
    expect(screen.getByText(/Carrier status unavailable/)).toHaveAttribute('title', 'network timeout');
    expect(screen.getByRole('button', { name: 'Update When unreachable' })).toBeEnabled();
  });

  it('locks conditional rules while Always forwarding is active', async () => {
    mocks.status.data.items[0] = { reason: 'unconditional', enabled: true, number: '+12025550100', available: true };
    render(<CallForwarding />);
    await userEvent.click(screen.getByRole('button', { name: 'Load status' }));

    const alwaysCard = screen.getByText('Always').closest('[data-slot="card"]')!;
    const busyCard = screen.getByText('When busy').closest('[data-slot="card"]')!;
    expect(withinCard(alwaysCard, 'checkbox')).toBeEnabled();
    expect(withinCard(busyCard, 'checkbox')).toBeDisabled();
    expect(screen.getAllByText('Inactive while Always is on')).toHaveLength(3);
    expect(screen.getByRole('button', { name: 'Update When busy' })).toBeDisabled();
    expect(screen.getByText(/Conditional forwarding settings are retained/)).toBeVisible();
  });

  it('locks every rule while carrier status is refreshing', async () => {
    const view = render(<CallForwarding />);
    await userEvent.click(screen.getByRole('button', { name: 'Load status' }));
    mocks.status.isFetching = true;
    view.rerender(<CallForwarding />);

    expect(screen.getByRole('button', { name: 'Checking…' })).toBeDisabled();
    expect(screen.getAllByRole('checkbox')).toHaveLength(4);
    for (const checkbox of screen.getAllByRole('checkbox')) expect(checkbox).toBeDisabled();
    expect(screen.getByText(/Forwarding controls are temporarily locked/)).toBeVisible();
  });
});

function withinCard(card: Element, role: 'checkbox' | 'textbox'): HTMLElement {
  const element = card.querySelector(`[role="${role}"]`) ?? card.querySelector(role === 'checkbox' ? 'input[type="checkbox"]' : 'input[type="tel"]');
  if (!(element instanceof HTMLElement)) throw new Error(`${role} not found`);
  return element;
}
