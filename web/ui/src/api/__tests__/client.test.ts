import { describe, expect, it } from 'vitest';
import { ApiError, unwrap } from '../client';

describe('unwrap', () => {
  it('accepts successful 204 no-content responses', () => {
    expect(unwrap<void>({ response: new Response(null, { status: 204 }) })).toBeUndefined();
  });

  it('still rejects an unexpectedly empty response body', () => {
    expect(() => unwrap({ response: new Response(null, { status: 200 }) })).toThrow(ApiError);
  });
});
