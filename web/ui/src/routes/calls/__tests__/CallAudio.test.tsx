import { render, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  start: vi.fn().mockResolvedValue(undefined),
  stop: vi.fn(),
}));

vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ token: 'test-token' }) }));
vi.mock('../audio', () => ({
  BrowserCallAudioSession: class {
    start = mocks.start;
    stop = mocks.stop;
    setMicMuted() {}
    setSpeakerMuted() {}
    sendTestTone() {}
  },
}));

import { CallAudio } from '../CallAudio';

describe('CallAudio', () => {
  it('connects once when an answered call requests automatic audio', async () => {
    const handled = vi.fn();
    render(<CallAudio callID="call-1" autoConnect onAutoConnectHandled={handled} />);

    await waitFor(() => expect(mocks.start).toHaveBeenCalledOnce());
    expect(handled).toHaveBeenCalledOnce();
  });
});
