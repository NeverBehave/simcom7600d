import { resetSim7600Client, Sim7600Client } from '@sim7600d/client';

const BASE_URL =
  (import.meta.env.VITE_API_BASE as string | undefined) ?? window.location.origin;

let _client: Sim7600Client | null = null;

/**
 * Configure the SDK singleton with the given token, or clear it when null.
 * Calling Sim7600Client's constructor reconfigures the underlying hey-api
 * singleton; passing null resets the singleton with no auth header so
 * stale tokens do not leak into post-logout requests.
 */
export function configureClient(token: string | null): void {
  if (token) {
    _client = new Sim7600Client({ baseUrl: BASE_URL, token });
  } else {
    _client = null;
    resetSim7600Client(BASE_URL);
  }
}

export function api(): Sim7600Client {
  if (!_client) throw new Error('sim7600d SDK not configured');
  return _client;
}

export class ApiError extends Error {
  constructor(
    public readonly status: number | undefined,
    public readonly body: unknown,
  ) {
    super(typeof body === 'object' && body && 'error' in body
      ? String((body as { error: { message?: string } }).error?.message ?? 'API error')
      : 'API error');
    this.name = 'ApiError';
  }
}

/**
 * Unwrap the {data,error,response} envelope returned by the generated fetch client.
 * Throws an ApiError if the request failed; returns data otherwise.
 */
export function unwrap<T>(res: {
  data?: T;
  error?: unknown;
  response?: Response;
}): T {
  if (res.error !== undefined) {
    throw new ApiError(res.response?.status, res.error);
  }
  if (res.data === undefined) {
    // Successful no-content operations (DTMF, delete, reconcile, vacuum,
    // reset) legitimately have no decoded response body.
    if (res.response?.ok && res.response.status === 204) {
      return undefined as T;
    }
    throw new ApiError(res.response?.status, { error: { message: 'empty response' } });
  }
  return res.data;
}
