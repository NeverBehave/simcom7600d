import { useMemo } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from './auth/AuthProvider';
import { LoginScreen } from './auth/LoginScreen';
import { makeQueryClient } from './api/queryClient';
import { AppShell } from './components/AppShell';
import Dashboard from './routes/dashboard';
import Sms from './routes/sms';
import Calls from './routes/calls';
import Voicemail from './routes/voicemail';
import CallForwarding from './routes/call-forwarding';
import Events from './routes/events';
import Admin from './routes/admin';

function Inner() {
  const auth = useAuth();
  const client = useMemo(() => makeQueryClient(auth.logout), [auth.logout]);
  return (
    <QueryClientProvider client={client}>
      {auth.token ? (
        <BrowserRouter>
          <Routes>
            <Route element={<AppShell />}>
              <Route index element={<Dashboard />} />
              <Route path="/sms/*" element={<Sms />} />
              <Route path="/calls/*" element={<Calls />} />
              <Route path="/voicemail/*" element={<Voicemail />} />
              <Route path="/call-forwarding" element={<CallForwarding />} />
              <Route path="/events" element={<Events />} />
              <Route path="/admin" element={<Admin />} />
            </Route>
          </Routes>
        </BrowserRouter>
      ) : (
        <LoginScreen />
      )}
    </QueryClientProvider>
  );
}

export default function App() {
  return (
    <AuthProvider>
      <Inner />
    </AuthProvider>
  );
}
