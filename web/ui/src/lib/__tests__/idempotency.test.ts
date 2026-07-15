import { describe, expect, it } from 'vitest';
import { newIdempotencyKey } from '../idempotency';

describe('newIdempotencyKey', () => {
  it('returns a UUID v4 string', () => {
    const k = newIdempotencyKey();
    expect(k).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
  });

  it('returns different values on repeated calls', () => {
    const a = newIdempotencyKey();
    const b = newIdempotencyKey();
    expect(a).not.toEqual(b);
  });
});
