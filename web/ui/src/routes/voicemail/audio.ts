import { apiURL } from '@/api/client';

export async function fetchVoicemailAudio(
  path: string,
  token: string,
  fetchImpl: typeof fetch = fetch,
): Promise<Blob> {
  const response = await fetchImpl(apiURL(path), {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok) {
    throw new Error(`Could not load voicemail audio (HTTP ${response.status})`);
  }
  return response.blob();
}
