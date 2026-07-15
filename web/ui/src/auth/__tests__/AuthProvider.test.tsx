import { describe, expect, it, beforeEach, vi } from 'vitest';
import { render, renderHook, act } from '@testing-library/react';
import { AuthProvider, useAuth } from '../AuthProvider';
import { api, configureClient } from '@/api/client';

function wrap({ children }: { children: React.ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>;
}

describe('AuthProvider', () => {
  beforeEach(() => {
	configureClient(null);
    localStorage.clear();
    sessionStorage.clear();
    window.history.replaceState({}, '', '/');
  });

  it('starts with no token when storage is empty', () => {
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    expect(result.current.token).toBeNull();
  });

  it('reads token from localStorage on mount', () => {
    localStorage.setItem('sim7600d_token', 'abc123');
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    expect(result.current.token).toBe('abc123');
  });

  it('configures the SDK before authenticated children render', () => {
	localStorage.setItem('sim7600d_token', 'abc123');
	expect(() => render(<AuthProvider><ClientReadySentinel /></AuthProvider>)).not.toThrow();
  });

  it('reads token from sessionStorage if local is absent', () => {
    sessionStorage.setItem('sim7600d_token', 'sess-xyz');
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    expect(result.current.token).toBe('sess-xyz');
  });

  it('never authenticates with ?token= and strips it from browser history', () => {
    window.history.replaceState({}, '', '/sms?token=urltok&keep=1');
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    expect(result.current.token).toBeNull();
    expect(localStorage.getItem('sim7600d_token')).toBeNull();
    expect(sessionStorage.getItem('sim7600d_token')).toBeNull();
    expect(window.location.search).toBe('?keep=1');
  });

  it('login(remember=true) persists to localStorage', () => {
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    act(() => result.current.login('newtok', true));
    expect(localStorage.getItem('sim7600d_token')).toBe('newtok');
    expect(sessionStorage.getItem('sim7600d_token')).toBeNull();
    expect(result.current.token).toBe('newtok');
  });

  it('login(remember=false) persists to sessionStorage only', () => {
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    act(() => result.current.login('newtok', false));
    expect(localStorage.getItem('sim7600d_token')).toBeNull();
    expect(sessionStorage.getItem('sim7600d_token')).toBe('newtok');
  });

  it('logout clears both storages and the token', () => {
    localStorage.setItem('sim7600d_token', 'a');
    sessionStorage.setItem('sim7600d_token', 'b');
    const { result } = renderHook(() => useAuth(), { wrapper: wrap });
    act(() => result.current.logout());
    expect(localStorage.getItem('sim7600d_token')).toBeNull();
    expect(sessionStorage.getItem('sim7600d_token')).toBeNull();
    expect(result.current.token).toBeNull();
  });

  it('useAuth outside of AuthProvider throws', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(() => render(<UseAuthSentinel />)).toThrow(/AuthProvider/);
    spy.mockRestore();
  });
});

function UseAuthSentinel() {
  useAuth();
  return null;
}

function ClientReadySentinel() {
	api();
	return null;
}
