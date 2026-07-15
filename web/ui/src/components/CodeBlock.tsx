export function CodeBlock({ children }: { children: string }) {
  return (
    <pre className="text-xs bg-neutral-900 text-neutral-100 p-3 rounded-md overflow-auto">
      <code>{children}</code>
    </pre>
  );
}
