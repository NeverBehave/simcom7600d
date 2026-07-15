import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useAdminAt } from '@/api/queries';
import { CodeBlock } from '@/components/CodeBlock';

interface Entry {
  cmd: string;
  lines: string[];
  final: string;
  code: number;
}

export function AtConsole({ disabled }: { disabled: boolean }) {
  const [cmd, setCmd] = useState('');
  const [history, setHistory] = useState<Entry[]>([]);
  const at = useAdminAt();

  async function run() {
    if (!cmd.trim()) return;
    try {
      const out = await at.mutateAsync({ cmd });
      const o = out as { lines?: string[] | null; final: string; code: number };
      setHistory((h) => [{ cmd, lines: o.lines ?? [], final: o.final, code: o.code }, ...h].slice(0, 50));
      setCmd('');
    } catch (e) {
      setHistory((h) => [{ cmd, lines: [], final: e instanceof Error ? e.message : String(e), code: -1 }, ...h].slice(0, 50));
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex gap-2">
        <Input
          value={cmd}
          onChange={(e) => setCmd(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') run(); }}
          placeholder="AT+CSQ"
          disabled={disabled}
          spellCheck={false}
          autoComplete="off"
          className="font-mono"
        />
        <Button onClick={run} disabled={disabled || at.isPending || !cmd.trim()}>
          {at.isPending ? 'Running…' : 'Run'}
        </Button>
      </div>
      <div className="space-y-2 max-h-96 overflow-auto">
        {history.map((h, i) => (
          <div key={i}>
            <div className="text-xs text-neutral-500 font-mono">&gt; {h.cmd}</div>
            <CodeBlock>{[...h.lines, `→ ${h.final} (${h.code})`].join('\n')}</CodeBlock>
          </div>
        ))}
      </div>
    </div>
  );
}
