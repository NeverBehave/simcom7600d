import { describe, expect, it, vi } from 'vitest';
import { fetchVoicemailAudio } from './audio';

describe('voicemail audio', () => {
  it('fetches playback media with bearer authentication', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response('voice', {
      status: 200,
      headers: { 'Content-Type': 'audio/wav' },
    }));

    const result = await fetchVoicemailAudio('/v1/voicemails/vm-1/audio', 'secret', fetchImpl);

    expect(await result.text()).toBe('voice');
    expect(fetchImpl).toHaveBeenCalledWith(
      expect.stringContaining('/v1/voicemails/vm-1/audio'),
      { headers: { Authorization: 'Bearer secret' } },
    );
  });
});
