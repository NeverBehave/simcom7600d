import { useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { Bell, BellOff, LayoutDashboard, MessageSquare, Phone, PhoneForwarded, ListTree, Wrench, LogOut, MoreHorizontal } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from '@/components/ui/drawer';
import { useAuth } from '@/auth/AuthProvider';
import { cn } from '@/lib/cn';
import { useLiveEvents } from '@/api/live';

const NAV = [
  { to: '/',       label: 'Dashboard', icon: LayoutDashboard },
  { to: '/sms',    label: 'SMS',       icon: MessageSquare },
  { to: '/calls',  label: 'Calls',     icon: Phone },
  { to: '/call-forwarding', label: 'Call forwarding', icon: PhoneForwarded },
  { to: '/events', label: 'Events',    icon: ListTree },
  { to: '/admin',  label: 'Admin',     icon: Wrench },
];

export function AppShell() {
  const notifications = useLiveEvents();
  const [moreOpen, setMoreOpen] = useState(false);
  const location = useLocation();
  const { logout } = useAuth();
  return (
    <div className="h-dvh overflow-hidden md:grid md:grid-cols-[14rem_minmax(0,1fr)]">
      <aside className="hidden bg-neutral-900 text-neutral-100 md:flex flex-col p-3 gap-1">
        <div className="px-2 py-3 text-sm font-semibold tracking-wide">sim7600d</div>
        <nav className="flex-1 space-y-1">
          {NAV.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 px-3 py-2 rounded-md text-sm',
                  isActive ? 'bg-neutral-700' : 'hover:bg-neutral-800',
                )
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>
        <NotificationControl {...notifications} />
        <Button
          variant="ghost"
          className="justify-start text-neutral-200 hover:bg-neutral-800"
          onClick={logout}
        >
          <LogOut size={16} className="mr-2" />
          Sign out
        </Button>
      </aside>
      <main className="h-full min-h-0 min-w-0 overflow-auto bg-neutral-50 pb-16 md:pb-0">
        <Outlet />
      </main>
      <nav className="fixed inset-x-0 bottom-0 z-40 grid h-16 grid-cols-5 border-t bg-white/95 px-1 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden" aria-label="Main navigation">
        {NAV.slice(0, 4).map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) => cn(
              'flex min-w-0 flex-col items-center justify-center gap-1 rounded-md text-[11px] font-medium',
              isActive ? 'text-brand-700' : 'text-neutral-500',
            )}
          >
            <Icon size={20} />
            <span className="max-w-full truncate">{label === 'Call forwarding' ? 'Forwarding' : label}</span>
          </NavLink>
        ))}
        <button
          type="button"
          className={cn(
            'flex min-w-0 flex-col items-center justify-center gap-1 rounded-md text-[11px] font-medium',
            location.pathname === '/events' || location.pathname === '/admin' ? 'text-brand-700' : 'text-neutral-500',
          )}
          onClick={() => setMoreOpen(true)}
        >
          <MoreHorizontal size={20} />
          <span>More</span>
        </button>
      </nav>
      <Drawer open={moreOpen} onOpenChange={setMoreOpen}>
        <DrawerContent>
          <DrawerHeader><DrawerTitle>More</DrawerTitle></DrawerHeader>
          <nav className="space-y-1 px-4 pb-3" aria-label="More navigation">
            {NAV.slice(4).map(({ to, label, icon: Icon }) => (
              <NavLink key={to} to={to} onClick={() => setMoreOpen(false)} className={({ isActive }) => cn('flex items-center gap-3 rounded-xl px-3 py-3 text-sm font-medium', isActive ? 'bg-brand-50 text-brand-700' : 'text-neutral-700 hover:bg-neutral-100')}>
                <Icon size={18} />{label}
              </NavLink>
            ))}
            <NotificationControl {...notifications} mobile />
            <button type="button" className="flex w-full items-center gap-3 rounded-xl px-3 py-3 text-sm font-medium text-red-700 hover:bg-red-50" onClick={logout}>
              <LogOut size={18} />Sign out
            </button>
          </nav>
        </DrawerContent>
      </Drawer>
    </div>
  );
}

function NotificationControl({ notificationPermission, enableNotifications, mobile = false }: ReturnType<typeof useLiveEvents> & { mobile?: boolean }) {
  if (notificationPermission === 'granted') {
    return <div className="flex items-center gap-2 px-3 py-2 text-xs text-neutral-400"><Bell size={16} /> Notifications on</div>;
  }
  if (notificationPermission === 'denied') {
    return <div className="flex items-center gap-2 px-3 py-2 text-xs text-neutral-400" title="Allow notifications in your browser's site settings"><BellOff size={16} /> Notifications blocked</div>;
  }
  if (notificationPermission === 'unsupported') return null;
  return (
    <Button variant="ghost" className={cn('justify-start', mobile ? 'text-neutral-700 hover:bg-neutral-100' : 'text-neutral-200 hover:bg-neutral-800')} onClick={enableNotifications}>
      <Bell size={16} className="mr-2" /> Enable notifications
    </Button>
  );
}
