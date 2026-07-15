import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
}));

vi.mock('@/api/queries', () => ({
  useCallDtmf: () => ({
    mutateAsync: mocks.mutateAsync,
    isPending: false,
  }),
}));

import { Dtmf } from '../Dtmf';

beforeEach(() => {
  mocks.mutateAsync.mockReset();
});

describe('Dtmf', () => {
  it('sends a visible-duration tone and confirms success', async () => {
    mocks.mutateAsync.mockResolvedValue(undefined);
    render(<Dtmf id="call-1" />);

    await userEvent.click(screen.getByRole('button', { name: 'Send DTMF 5' }));

    expect(mocks.mutateAsync).toHaveBeenCalledWith({ id: 'call-1', digits: '5', durationMS: 200 });
    expect(await screen.findByText('Sent 5')).toBeVisible();
  });

  it('shows the modem error instead of silently failing', async () => {
    mocks.mutateAsync.mockRejectedValue(new Error('modem rejected tone'));
    render(<Dtmf id="call-1" />);

    await userEvent.click(screen.getByRole('button', { name: 'Send DTMF #' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not send #: modem rejected tone');
  });
});
