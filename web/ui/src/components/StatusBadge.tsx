import { cn } from '@/lib/cn';
import { Badge } from '@/components/ui/badge';

const VARIANTS: Record<string, string> = {
  ringing:   'bg-amber-100 text-amber-800',
  dialing:   'bg-amber-100 text-amber-800',
  alerting:  'bg-amber-100 text-amber-800',
  active:    'bg-emerald-100 text-emerald-800',
	held:      'bg-blue-100 text-blue-800',
  ended:     'bg-neutral-200 text-neutral-700',
  missed:    'bg-red-100 text-red-800',
  rejected:  'bg-red-100 text-red-800',
  sent:      'bg-emerald-100 text-emerald-800',
  delivered: 'bg-emerald-100 text-emerald-800',
  failed:    'bg-red-100 text-red-800',
  pending:   'bg-amber-100 text-amber-800',
	queued:    'bg-amber-100 text-amber-800',
	submitted: 'bg-blue-100 text-blue-800',
	accepted:  'bg-blue-100 text-blue-800',
	indeterminate: 'bg-neutral-200 text-neutral-700',
};

export function StatusBadge({ value }: { value: string | undefined | null }) {
  if (!value) return null;
	const label = value.replace(/_/g, ' ');
  return <Badge className={cn('border-0', VARIANTS[value] ?? 'bg-neutral-200 text-neutral-700')}>{label}</Badge>;
}
