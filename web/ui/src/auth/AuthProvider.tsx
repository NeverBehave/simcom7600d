import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';
import { configureClient } from '@/api/client';

const STORAGE_KEY = 'sim7600d_token';

type AuthState = {
  token: string | null;
  login: (token: string, remember: boolean) => void;
  logout: () => void;
};

const AuthContext = createContext<AuthState | null>(null);

function stripTokenFromURL(): void {
  const url = new URL(window.location.href);
  if (!url.searchParams.has('token')) return;
  url.searchParams.delete('token');
  const search = url.searchParams.toString();
  const next = url.pathname + (search ? `?${search}` : '') + url.hash;
  window.history.replaceState(window.history.state, '', next);
}

function readPersistedToken(): string | null {
  return localStorage.getItem(STORAGE_KEY) ?? sessionStorage.getItem(STORAGE_KEY);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => {
	stripTokenFromURL();
	const persisted = readPersistedToken();
	configureClient(persisted);
	return persisted;
  });

  const value = useMemo<AuthState>(
    () => ({
      token,
      login(t, remember) {
        if (remember) {
          localStorage.setItem(STORAGE_KEY, t);
          sessionStorage.removeItem(STORAGE_KEY);
	  } else {
          sessionStorage.setItem(STORAGE_KEY, t);
          localStorage.removeItem(STORAGE_KEY);
	  }
	  configureClient(t);
	  setToken(t);
      },
      logout() {
	  localStorage.removeItem(STORAGE_KEY);
	  sessionStorage.removeItem(STORAGE_KEY);
	  configureClient(null);
	  setToken(null);
      },
    }),
    [token],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside an AuthProvider');
  return ctx;
}
