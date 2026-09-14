import {
  canDeleteIncident,
  hasIncidentPermission,
  incidentDeadline,
  incidentEscalationOptions,
  incidentNextStatuses,
  normalizeIncidentCollection,
} from '@/lib/incident';
import { describe, expect, it } from 'vitest';
import type { Incident } from '@/types/incident';
import type { User } from '@/types';

const USER = {
  id: '79e79c27-f11e-471c-b7f6-e4327165fa70',
  organization_id: '62b7cbda-30c8-4468-b3e8-e75e5cab32e5',
  email: 'responder@example.test', first_name: 'Iris', last_name: 'Responder', status: 'active',
  is_super_admin: false, language: 'en', created_at: '2026-09-14T00:00:00Z', updated_at: '2026-09-14T00:00:00Z',
} satisfies User;

describe('incident contract helpers', () => {
  it('normalizes the handler data/pagination envelope and rejects drift', () => {
    expect(normalizeIncidentCollection({
      data: [{ id: 'incident-1' }],
      pagination: { page: 2, page_size: 20, total_items: 21, total_pages: 2 },
    })).toEqual({ items: [{ id: 'incident-1' }], page: 2, page_size: 20, total: 21, total_pages: 2 });
    expect(() => normalizeIncidentCollection({ data: null } as never)).toThrow('invalid collection');
    expect(() => normalizeIncidentCollection({ data: [], pagination: { page: 1, page_size: -1, total_items: 0, total_pages: 0 } })).toThrow('invalid pagination');
  });

  it('exposes only lifecycle transitions implemented by migration 046', () => {
    expect(incidentNextStatuses('reported')).toEqual(['triaged']);
    expect(incidentNextStatuses('triaged')).toEqual(['investigating']);
    expect(incidentNextStatuses('investigating')).toEqual(['contained', 'resolved']);
    expect(incidentNextStatuses('contained')).toEqual(['investigating', 'resolved']);
    expect(incidentNextStatuses('resolved')).toEqual(['investigating']);
    expect(incidentNextStatuses('closed')).toEqual([]);
    expect(incidentNextStatuses('cancelled')).toEqual([]);
  });

  it('only offers severity increases', () => {
    expect(incidentEscalationOptions('low')).toEqual(['medium', 'high', 'critical']);
    expect(incidentEscalationOptions('high')).toEqual(['critical']);
    expect(incidentEscalationOptions('critical')).toEqual([]);
  });

  it('fails closed on missing or singular-resource permissions', () => {
    expect(hasIncidentPermission(undefined, USER, 'read')).toBe(false);
    expect(hasIncidentPermission({ incident: ['read'] }, USER, 'read')).toBe(false);
    expect(hasIncidentPermission({ incidents: ['read', 'approve'] }, USER, 'approve')).toBe(true);
    expect(hasIncidentPermission({}, { ...USER, is_super_admin: true }, 'delete')).toBe(true);
  });

  it('classifies the GDPR 72-hour deadline without masking overdue time', () => {
    const base = { notification_deadline: '2026-09-17T12:00:00Z' } as Incident;
    expect(incidentDeadline(base, new Date('2026-09-14T12:00:00Z'))).toEqual({ state: 'upcoming', hoursRemaining: 72 });
    expect(incidentDeadline(base, new Date('2026-09-17T00:30:00Z')).state).toBe('urgent');
    expect(incidentDeadline(base, new Date('2026-09-18T00:00:00Z')).state).toBe('overdue');
    expect(incidentDeadline({ ...base, dpa_notified_at: '2026-09-15T00:00:00Z' }).state).toBe('notified');
  });

  it('enforces terminal-state, retention, and legal-hold deletion gates', () => {
    const incident = { status: 'closed', legal_hold: false } as Incident;
    expect(canDeleteIncident(incident, new Date('2026-09-14T00:00:00Z'))).toBe(true);
    expect(canDeleteIncident({ ...incident, legal_hold: true })).toBe(false);
    expect(canDeleteIncident({ ...incident, status: 'resolved' })).toBe(false);
    expect(canDeleteIncident({ ...incident, retention_until: '2026-09-15T00:00:00Z' }, new Date('2026-09-14T00:00:00Z'))).toBe(false);
  });
});
