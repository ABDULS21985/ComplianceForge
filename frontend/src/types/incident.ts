export type IncidentSeverity = 'critical' | 'high' | 'medium' | 'low';

export type IncidentStatus =
  | 'reported'
  | 'triaged'
  | 'investigating'
  | 'contained'
  | 'resolved'
  | 'closed'
  | 'cancelled';

export type BreachAssessmentStatus = 'pending' | 'not_notifiable' | 'notifiable';

export type IncidentAssignmentRole = 'primary' | 'investigator' | 'observer';

export interface Incident {
  id: string;
  organization_id: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
  incident_ref: string;
  title: string;
  description: string;
  category: string;
  severity: IncidentSeverity;
  status: IncidentStatus;
  reporter_id: string;
  assignee_id?: string;
  detected_at?: string;
  reported_at: string;
  occurred_at?: string;
  triaged_at?: string;
  investigation_started_at?: string;
  contained_at?: string;
  resolved_at?: string;
  closed_at?: string;
  cancelled_at?: string;
  cancellation_reason?: string;
  reopened_at?: string;
  root_cause: string;
  impact: string;
  lessons_learned: string;
  related_asset_id?: string;
  followup_date?: string;
  is_data_breach: boolean;
  breach_assessment_status: BreachAssessmentStatus;
  is_breach_notifiable: boolean;
  breach_assessment_reason?: string;
  breach_assessed_at?: string;
  breach_assessed_by?: string;
  breach_awareness_at?: string;
  notification_deadline?: string;
  data_subjects_affected?: number;
  records_affected?: number;
  data_categories: string[];
  special_category_data: boolean;
  cross_border: boolean;
  breach_nature: string;
  likely_consequences: string;
  mitigation_measures: string;
  dpa_notified_at?: string;
  dpa_notification_reference?: string;
  dpa_notification_reason?: string;
  version: number;
  retention_until?: string;
  legal_hold: boolean;
  metadata: Record<string, unknown>;
  deadline_state?: 'notified' | 'overdue' | 'urgent' | 'upcoming' | string;
  hours_remaining?: number;
}

export interface IncidentCreateInput {
  title: string;
  description: string;
  category: string;
  severity: IncidentSeverity;
  detected_at?: string;
  occurred_at?: string;
  related_asset_id?: string;
  followup_date?: string;
  retention_until?: string;
  metadata?: Record<string, unknown>;
}

export interface IncidentPatch {
  version: number;
  title?: string;
  description?: string;
  category?: string;
  severity?: IncidentSeverity;
  detected_at?: string;
  occurred_at?: string;
  clear_occurred_at?: boolean;
  root_cause?: string;
  impact?: string;
  lessons_learned?: string;
  related_asset_id?: string;
  clear_related_asset_id?: boolean;
  followup_date?: string;
  clear_followup_date?: boolean;
  retention_until?: string;
  legal_hold?: boolean;
  metadata?: Record<string, unknown>;
}

export interface IncidentListParams {
  page?: number;
  page_size?: number;
  status?: IncidentStatus;
  severity?: IncidentSeverity;
  category?: string;
  assignee_id?: string;
  search?: string;
  breach_notifiable?: boolean;
  sort?: 'reported_at' | 'updated_at' | 'severity' | 'status' | 'title' | 'notification_deadline';
  direction?: 'asc' | 'desc';
}

export interface IncidentTransitionInput {
  status: IncidentStatus;
  reason?: string;
  version: number;
}

export interface IncidentReasonInput {
  version: number;
  reason: string;
}

export interface IncidentEscalationInput {
  severity: IncidentSeverity;
  reason: string;
  version: number;
}

export interface IncidentBreachAssessmentInput {
  version: number;
  status: Exclude<BreachAssessmentStatus, 'pending'>;
  reason: string;
  is_data_breach: boolean;
  awareness_at?: string;
  data_subjects_affected?: number;
  records_affected?: number;
  data_categories?: string[];
  special_category_data: boolean;
  cross_border: boolean;
  breach_nature?: string;
  likely_consequences?: string;
  mitigation_measures?: string;
}

export interface IncidentDPANotificationInput {
  version: number;
  idempotency_key: string;
  notified_at: string;
  reference: string;
  reason: string;
}

export interface IncidentAssignmentInput {
  version: number;
  assignee_id: string;
  role: IncidentAssignmentRole;
  reason: string;
}

export interface IncidentUnassignmentInput {
  version: number;
  reason: string;
}

export interface IncidentAssignment {
  id: string;
  organization_id: string;
  incident_id: string;
  assignee_user_id: string;
  role: IncidentAssignmentRole;
  assigned_by: string;
  reason: string;
  assigned_at: string;
  unassigned_at?: string;
  unassigned_by?: string;
  unassign_reason?: string;
  created_at: string;
  updated_at: string;
}

export interface IncidentAssignmentMutationResponse {
  assignment: IncidentAssignment;
  incident: Incident;
}

export interface IncidentEvent {
  id: string;
  organization_id: string;
  incident_id: string;
  event_type: string;
  actor_user_id: string;
  from_status?: IncidentStatus;
  to_status?: IncidentStatus;
  summary: string;
  details: Record<string, unknown>;
  occurred_at: string;
  created_at: string;
}

export interface IncidentStatistics {
  total: number;
  active: number;
  overdue_breaches: number;
  urgent_breaches: number;
  notified_breaches: number;
  average_resolution_hours: number;
  by_status: Partial<Record<IncidentStatus, number>>;
  by_severity: Partial<Record<IncidentSeverity, number>>;
}

export interface IncidentCollectionEnvelope<T> {
  data: T[];
  pagination: {
    page: number;
    page_size: number;
    total_items: number;
    total_pages: number;
  };
}

export interface IncidentDataEnvelope<T> {
  data: T;
}

export interface IncidentPage<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}
