import { useEffect, useState } from 'react';
import { relativeTs, shortTs } from '@/lib/format';

export function RelativeTime({ ts }: { ts?: string | null }) {
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 15_000);
    return () => clearInterval(id);
  }, []);
  return <span title={shortTs(ts)}>{relativeTs(ts)}</span>;
}
