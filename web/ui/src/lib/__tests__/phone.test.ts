import { describe, expect, it } from 'vitest';
import { formatPhoneNumber, phoneKey } from '../phone';

describe('phone helpers', () => {
  it('groups equivalent North American formats under one key', () => {
    expect(phoneKey('+1 (415) 555-0100')).toBe('4155550100');
    expect(phoneKey('415-555-0100')).toBe('4155550100');
  });

  it('formats ten digit numbers without changing short codes', () => {
    expect(formatPhoneNumber('+14155550100')).toBe('(415) 555-0100');
    expect(formatPhoneNumber('611')).toBe('611');
  });
});
