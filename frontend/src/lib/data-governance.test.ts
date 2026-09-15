import {
  DATA_GOVERNANCE_ROUTES,
  formatGovernanceDate,
  humanizeGovernanceToken,
  isGovernanceConflict,
  parseGovernanceObject,
  toISOStringOrUndefined,
} from '@/lib/data-governance';
import { describe, expect, it } from 'vitest';

describe('data governance helpers', () => {
  it('encodes every dynamic route segment', () => {
    expect(DATA_GOVERNANCE_ROUTES.disposition('audit finding', 'record/1')).toBe(
      '/settings/data-governance/records/audit%20finding/record%2F1/disposition',
    );
    expect(DATA_GOVERNANCE_ROUTES.releaseHoldRecord('hold/1', 'record#1')).toBe(
      '/settings/data-governance/legal-holds/hold%2F1/records/record%231/release',
    );
  });

  it('accepts only JSON objects for policy metadata and hold scope', () => {
    expect(parseGovernanceObject('{"region":"EU"}', 'Scope')).toEqual({ region: 'EU' });
    expect(() => parseGovernanceObject('[]', 'Scope')).toThrow('JSON object');
    expect(() => parseGovernanceObject('{broken', 'Scope')).toThrow('valid JSON');
  });

  it('normalizes labels, dates, and optimistic conflicts safely', () => {
    expect(humanizeGovernanceToken('special_category')).toBe('Special Category');
    expect(formatGovernanceDate('not-a-date')).toBe('not-a-date');
    expect(toISOStringOrUndefined('')).toBeUndefined();
    expect(isGovernanceConflict({ status: 409 })).toBe(true);
    expect(isGovernanceConflict({ status: 400 })).toBe(false);
  });
});
