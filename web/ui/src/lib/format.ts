export function shortTs(iso: string | undefined | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export function relativeTs(iso: string | undefined | null, now: Date = new Date()): string {
  if (!iso) return '—';
  const d = new Date(iso);
  const diff = now.getTime() - d.getTime();
  if (Number.isNaN(diff)) return iso;
  const s = Math.round(diff / 1000);
  if (s < 5) return 'just now';
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  return d.toLocaleDateString();
}
