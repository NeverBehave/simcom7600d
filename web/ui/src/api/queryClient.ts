import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { ApiError } from './client';

export function makeQueryClient(onUnauthorized: () => void): QueryClient {
  const handle = (err: unknown) => {
    if (err instanceof ApiError && err.status === 401) onUnauthorized();
  };
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, refetchOnWindowFocus: false, staleTime: 1_000 },
      mutations: { retry: false },
    },
    queryCache: new QueryCache({ onError: handle }),
    mutationCache: new MutationCache({ onError: handle }),
  });
}
