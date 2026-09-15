import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import type {
  DataGovernancePolicy,
  GovernanceChainVerification,
  LegalHold,
  RecordDispositionDecision,
  RecordRetentionAssignment,
  RetentionSchedule,
} from '@/types/data-governance';
import api from '@/lib/api';
import { CSRF_TOKEN_HEADER } from '@/lib/auth-constants';
import type { PaginatedDataEnvelope } from '@/types/enterprise-settings';
import { resetCsrfToken } from '@/lib/csrf-client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('data lifecycle governance API contract', () => {
  const fetchMock = vi.fn<(input: RequestInfo | URL, request?: RequestInit) => Promise<Response>>();
  beforeEach(() => {
    resetCsrfToken();
    fetchMock.mockImplementation(async (input, request) => {
      const url = String(input);
      if (url === '/api/auth/csrf') return jsonResponse({ csrf_token: 'csrf-governance' });
      if (request?.method === 'DELETE' || url.includes('/records/hold-record-1/release'))
        return new Response(null, { status: 204 });
      if (
        (url.endsWith('/retention-schedules') ||
          url.endsWith('/legal-holds') ||
          url.includes('/events?')) &&
        request?.method === 'GET'
      ) {
        return jsonResponse({
          data: [],
          pagination: { page: 1, page_size: 20, total_items: 0, total_pages: 0 },
        });
      }
      if (url.includes('/exceptions') || url.includes('/records?'))
        return jsonResponse({ data: [] });
      return jsonResponse({});
    });
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('exposes exact typed single and paginated return contracts', () => {
    expectTypeOf(api.dataGovernance.getPolicy).returns.toEqualTypeOf<
      Promise<DataGovernancePolicy>
    >();
    expectTypeOf(api.dataGovernance.listSchedules).returns.toEqualTypeOf<
      Promise<PaginatedDataEnvelope<RetentionSchedule>>
    >();
    expectTypeOf(api.dataGovernance.getAssignment).returns.toEqualTypeOf<
      Promise<RecordRetentionAssignment>
    >();
    expectTypeOf(api.dataGovernance.getDisposition).returns.toEqualTypeOf<
      Promise<RecordDispositionDecision>
    >();
    expectTypeOf(api.dataGovernance.getHold).returns.toEqualTypeOf<Promise<LegalHold>>();
    expectTypeOf(api.dataGovernance.verifyEvents).returns.toEqualTypeOf<
      Promise<GovernanceChainVerification>
    >();
  });

  it('uses canonical collection, record, exception, hold, and event routes', async () => {
    await api.dataGovernance.getPolicy();
    await api.dataGovernance.listSchedules({
      record_type: 'incident',
      classification: 'personal',
      status: 'active',
      sort_by: 'priority',
      sort_direction: 'asc',
      page: 2,
      page_size: 20,
    });
    await api.dataGovernance.getSchedule('schedule/1');
    await api.dataGovernance.getAssignment('assignment/1');
    await api.dataGovernance.listExceptions('assignment/1');
    await api.dataGovernance.getDisposition('audit_finding', 'record/1');
    await api.dataGovernance.listHolds({ status: 'active', page: 1, page_size: 20 });
    await api.dataGovernance.getHold('hold/1');
    await api.dataGovernance.listHoldRecords('hold/1', false);
    await api.dataGovernance.listEvents({
      entity_type: 'legal_hold',
      entity_id: 'hold/1',
      event_type: 'hold_released',
      page: 1,
      page_size: 20,
    });
    await api.dataGovernance.verifyEvents();

    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      '/api/bff/settings/data-governance/policy',
      '/api/bff/settings/data-governance/retention-schedules?record_type=incident&classification=personal&status=active&sort_by=priority&sort_direction=asc&page=2&page_size=20',
      '/api/bff/settings/data-governance/retention-schedules/schedule%2F1',
      '/api/bff/settings/data-governance/retention-assignments/assignment%2F1',
      '/api/bff/settings/data-governance/retention-assignments/assignment%2F1/exceptions',
      '/api/bff/settings/data-governance/records/audit_finding/record%2F1/disposition',
      '/api/bff/settings/data-governance/legal-holds?status=active&page=1&page_size=20',
      '/api/bff/settings/data-governance/legal-holds/hold%2F1',
      '/api/bff/settings/data-governance/legal-holds/hold%2F1/records?active_only=false',
      '/api/bff/settings/data-governance/events?entity_type=legal_hold&entity_id=hold%2F1&event_type=hold_released&page=1&page_size=20',
      '/api/bff/settings/data-governance/events/verify',
    ]);
  });

  it('carries versions, audit reasons, DELETE bodies, and CSRF without bearer headers', async () => {
    await api.dataGovernance.savePolicy({
      primary_region: 'eu-west-1',
      allowed_regions: ['eu-west-1'],
      cross_border_transfer_mode: 'approved_regions',
      default_retention_days: 365,
      deletion_grace_days: 30,
      disposition_approval_mode: 'single',
      require_processor_confirmation: true,
      legal_hold_enabled: true,
      policy_statement: '',
      metadata: {},
      expected_version: 7,
      reason: 'Annual review',
    });
    await api.dataGovernance.updateSchedule('schedule-1', {
      expected_version: 4,
      name: 'Updated schedule',
      reason: 'Legal requirement changed',
    });
    await api.dataGovernance.retireSchedule('schedule-1', {
      expected_version: 5,
      reason: 'Superseded by schedule 2',
    });
    await api.dataGovernance.reviewAssignment('assignment-1', {
      expected_version: 2,
      decision: 'approve',
      reason: 'Evidence checked',
    });
    await api.dataGovernance.decideException('exception-1', {
      expected_version: 3,
      decision: 'reject',
      reason: 'No legal basis',
    });
    await api.dataGovernance.releaseHold('hold-1', {
      expected_version: 6,
      outcome: 'release',
      reason: 'Counsel authorized release',
    });
    await api.dataGovernance.releaseHoldRecord('hold-1', 'hold-record-1', {
      reason: 'Removed from matter scope',
    });

    const calls = fetchMock.mock.calls.filter(([url]) => String(url) !== '/api/auth/csrf');
    expect(calls.map(([url, request]) => [String(url), request?.method])).toEqual([
      ['/api/bff/settings/data-governance/policy', 'PUT'],
      ['/api/bff/settings/data-governance/retention-schedules/schedule-1', 'PATCH'],
      ['/api/bff/settings/data-governance/retention-schedules/schedule-1', 'DELETE'],
      ['/api/bff/settings/data-governance/retention-assignments/assignment-1/review', 'POST'],
      ['/api/bff/settings/data-governance/retention-exceptions/exception-1/decision', 'POST'],
      ['/api/bff/settings/data-governance/legal-holds/hold-1/release', 'POST'],
      [
        '/api/bff/settings/data-governance/legal-holds/hold-1/records/hold-record-1/release',
        'POST',
      ],
    ]);
    expect(JSON.parse(String(calls[0][1]?.body))).toMatchObject({
      expected_version: 7,
      reason: 'Annual review',
    });
    expect(JSON.parse(String(calls[2][1]?.body))).toEqual({
      expected_version: 5,
      reason: 'Superseded by schedule 2',
    });
    expect(JSON.parse(String(calls[5][1]?.body))).toEqual({
      expected_version: 6,
      outcome: 'release',
      reason: 'Counsel authorized release',
    });
    for (const [, request] of calls) {
      expect(new Headers(request?.headers).get(CSRF_TOKEN_HEADER)).toBe('csrf-governance');
      expect(new Headers(request?.headers).has('authorization')).toBe(false);
    }
  });
});
