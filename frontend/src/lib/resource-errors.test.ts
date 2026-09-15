import { describe, expect, it } from 'vitest';
import { isForbiddenResourceError, resourceErrorStatus } from '@/lib/resource-errors';

describe('resource error classification', () => {
  it('recognizes permission denial without inspecting or displaying error details', () => {
    expect(isForbiddenResourceError({ status: 403, detail: 'withheld' })).toBe(true);
    expect(isForbiddenResourceError({ status: 503 })).toBe(false);
    expect(resourceErrorStatus(new Error('withheld'))).toBeNull();
    expect(resourceErrorStatus(null)).toBeNull();
  });
});
