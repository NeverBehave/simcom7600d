import type { ReactNode } from 'react';

export function EmptyState({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div className="p-8 text-center text-neutral-500">
      <div className="text-base">{title}</div>
      {hint && <div className="text-sm mt-2">{hint}</div>}
    </div>
  );
}
