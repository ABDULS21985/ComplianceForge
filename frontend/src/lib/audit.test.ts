import type { AuditCollectionEnvelope, AuditFinding } from '@/types/audit';
import {
  auditLifecycleActions,
  auditPersonName,
  findingNextStatuses,
  hasAuditPermission,
  humanizeAuditToken,
  isFindingOverdue,
  normalizeAuditCollection,
  toDateInputValue,
} from '@/lib/audit';
import { describe, expect, it } from 'vitest';
import type { User } from '@/types';

const USER: User = {
  id: '79e79c27-f11e-471c-b7f6-e4327165fa70',
  organization_id: '62b7cbda-30c8-4468-b3e8-e75e5cab32e5',
  email: 'auditor@example.test',
  first_name: 'Ada',
  last_name: 'Auditor',
  status: 'active',
  is_super_admin: false,
  language: 'en',
  created_at: '2026-09-14T00:00:00Z',
  updated_at: '2026-09-14T00:00:00Z',
};

describe('audit contract helpers', () => {
  it('normalizes the backend data/pagination envelope once at the boundary', () => {
    const response: AuditCollectionEnvelope<{ id: string }> = {
      data: [{ id: 'audit-1' }],
      pagination: { page: 2, page_size: 20, total_items: 41, total_pages: 3 },
    };
    expect(normalizeAuditCollection(response)).toEqual({
      items: [{ id: 'audit-1' }],
      page: 2,
      page_size: 20,
      total: 41,
      total_pages: 3,
    });
  });

  it('rejects malformed collection and pagination responses instead of hiding drift', () => {
    expect(() => normalizeAuditCollection({ data: null } as never)).toThrow('invalid collection');
    expect(() =>
      normalizeAuditCollection({
        data: [],
        pagination: { page: 1, page_size: -20, total_items: 0, total_pages: 0 },
      }),
    ).toThrow('invalid pagination');
  });

  it('offers only lifecycle actions accepted by the audit service', () => {
    expect(auditLifecycleActions('planned')).toEqual(['start', 'cancel']);
    expect(auditLifecycleActions('in_progress')).toEqual(['complete']);
    expect(auditLifecycleActions('completed')).toEqual(['close']);
    expect(auditLifecycleActions('closed')).toEqual([]);
    expect(auditLifecycleActions('cancelled')).toEqual([]);
  });

  it('offers only directed finding transitions accepted by the service', () => {
    expect(findingNextStatuses('open')).toEqual(['in_progress', 'resolved', 'accepted']);
    expect(findingNextStatuses('in_progress')).toEqual(['open', 'resolved', 'accepted']);
    expect(findingNextStatuses('resolved')).toEqual(['in_progress', 'closed']);
    expect(findingNextStatuses('accepted')).toEqual(['in_progress', 'closed']);
    expect(findingNextStatuses('closed')).toEqual([]);
  });

  it('marks active past-due findings overdue using UTC date boundaries', () => {
    const finding = { due_date: '2026-09-13T00:00:00Z', status: 'open' } as AuditFinding;
    expect(isFindingOverdue(finding, new Date('2026-09-14T23:59:00-08:00'))).toBe(true);
    expect(
      isFindingOverdue({ ...finding, status: 'resolved' }, new Date('2026-09-14T00:00:00Z')),
    ).toBe(false);
    expect(
      isFindingOverdue({ ...finding, due_date: 'not-a-date' }, new Date('2026-09-14T00:00:00Z')),
    ).toBe(false);
  });

  it('fails closed on absent audit permissions and honors explicit plural resource actions', () => {
    expect(hasAuditPermission(undefined, USER, 'read')).toBe(false);
    expect(hasAuditPermission({ audit: ['read'] }, USER, 'read')).toBe(false);
    expect(hasAuditPermission({ audits: ['read', 'update'] }, USER, 'update')).toBe(true);
    expect(hasAuditPermission({}, { ...USER, is_super_admin: true }, 'delete')).toBe(true);
  });

  it('formats projections and backend dates without unsafe casts', () => {
    expect(auditPersonName({ first_name: 'Ada', last_name: 'Auditor' })).toBe('Ada Auditor');
    expect(auditPersonName(undefined, USER.id)).toBe('User 79e79c27');
    expect(toDateInputValue('2026-09-14T00:00:00Z')).toBe('2026-09-14');
    expect(humanizeAuditToken('in_progress')).toBe('In Progress');
  });
});
