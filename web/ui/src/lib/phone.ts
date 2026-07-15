export function phoneKey(value: string | undefined) {
  const digits = (value ?? '').replace(/\D/g, '');
  if (digits.length === 11 && digits.startsWith('1')) return digits.slice(1);
  return digits;
}

export function formatPhoneNumber(value: string | undefined) {
  if (!value) return 'Unknown number';
  const key = phoneKey(value);
  if (key.length === 10) return `(${key.slice(0, 3)}) ${key.slice(3, 6)}-${key.slice(6)}`;
  return value;
}
