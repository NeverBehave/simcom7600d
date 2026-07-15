import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useAuth } from './AuthProvider';
import { ApiError, api, configureClient } from '@/api/client';

export function LoginScreen() {
  const { login } = useAuth();
  const [token, setToken] = useState('');
  const [remember, setRemember] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      configureClient(token);
      const res = await api().status.get({} as any);
      if (res.error) {
        throw new ApiError(res.response?.status, res.error);
      }
      login(token, remember);
    } catch (err) {
      configureClient(null);
      if (err instanceof ApiError && err.status === 401) {
        setError('Token rejected (401). Check it and try again.');
      } else if (err instanceof ApiError) {
        setError(`Server error: ${err.status ?? '?'}`);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="min-h-full flex items-center justify-center bg-neutral-50">
      <form
        onSubmit={submit}
        className="w-[28rem] max-w-full bg-white rounded-xl shadow p-6 space-y-4"
      >
        <h1 className="text-lg font-semibold">Sign in to sim7600d</h1>
        <p className="text-sm text-neutral-600">
          Paste the daemon's bearer token. It is sent only to this host.
        </p>
        <div className="space-y-1">
          <Label htmlFor="token">Bearer token</Label>
          <Input
            id="token"
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            autoFocus
            required
            spellCheck={false}
            autoComplete="current-password"
          />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => setRemember(e.target.checked)}
          />
          Keep me signed in on this device
        </label>
        {error && <div className="text-sm text-red-600">{error}</div>}
        <Button type="submit" disabled={busy || !token} className="w-full">
          {busy ? 'Verifying…' : 'Sign in'}
        </Button>
      </form>
    </div>
  );
}
