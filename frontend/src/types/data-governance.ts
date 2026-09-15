export type GovernanceRecordType =
  | 'asset'
  | 'audit'
  | 'audit_finding'
  | 'comment'
  | 'control'
  | 'evidence'
  | 'incident'
  | 'policy'
  | 'report'
  | 'risk'
  | 'vendor';

export type DataClassification =
  'public' | 'internal' | 'confidential' | 'restricted' | 'personal' | 'special_category';

export type RetentionTrigger =
  | 'record_created'
  | 'record_closed'
  | 'contract_ended'
  | 'employment_ended'
  | 'consent_withdrawn'
  | 'superseded'
  | 'case_closed';

export type DispositionAction = 'review' | 'delete' | 'anonymize' | 'archive';
export type RetentionScheduleStatus = 'draft' | 'active' | 'retired';
export type RetentionDecision = 'approve' | 'reject';
export type LegalHoldStatus = 'active' | 'released' | 'cancelled';
export type LegalHoldOutcome = 'release' | 'cancel';

export interface GovernanceTenantModel {
  id: string;
  organization_id: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

export interface DataGovernancePolicy {
  organization_id: string;
  primary_region: string;
  allowed_regions: string[];
  cross_border_transfer_mode: 'prohibited' | 'approved_regions' | 'contractual_safeguards';
  default_retention_days: number;
  default_archive_after_days?: number;
  deletion_grace_days: number;
  disposition_approval_mode: 'none' | 'single' | 'dual';
  require_processor_confirmation: boolean;
  legal_hold_enabled: boolean;
  policy_statement: string;
  version: number;
  metadata: Record<string, unknown>;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface DataGovernancePolicyInput {
  primary_region: string;
  allowed_regions: string[];
  cross_border_transfer_mode: DataGovernancePolicy['cross_border_transfer_mode'];
  default_retention_days: number;
  default_archive_after_days?: number;
  deletion_grace_days: number;
  disposition_approval_mode: DataGovernancePolicy['disposition_approval_mode'];
  require_processor_confirmation: boolean;
  legal_hold_enabled: boolean;
  policy_statement: string;
  metadata?: Record<string, unknown>;
  expected_version?: number;
  reason: string;
}

export interface RetentionSchedule extends GovernanceTenantModel {
  name: string;
  description: string;
  record_type: GovernanceRecordType;
  data_classification?: DataClassification;
  jurisdiction?: string;
  trigger_event: RetentionTrigger;
  legal_basis: string;
  retention_days: number;
  archive_after_days?: number;
  disposition_action: DispositionAction;
  review_required: boolean;
  priority: number;
  status: RetentionScheduleStatus;
  effective_from: string;
  effective_until?: string;
  version: number;
  created_by: string;
  updated_by: string;
}

export interface RetentionScheduleInput {
  name: string;
  description?: string;
  record_type: GovernanceRecordType;
  data_classification?: DataClassification;
  jurisdiction?: string;
  trigger_event: RetentionTrigger;
  legal_basis: string;
  retention_days: number;
  archive_after_days?: number;
  disposition_action: DispositionAction;
  review_required: boolean;
  priority: number;
  status: RetentionScheduleStatus;
  effective_from: string;
  effective_until?: string;
  reason: string;
}

export interface RetentionSchedulePatch {
  expected_version: number;
  name?: string;
  description?: string;
  data_classification?: DataClassification;
  clear_data_classification?: boolean;
  jurisdiction?: string;
  clear_jurisdiction?: boolean;
  trigger_event?: RetentionTrigger;
  legal_basis?: string;
  retention_days?: number;
  archive_after_days?: number;
  clear_archive_after_days?: boolean;
  disposition_action?: DispositionAction;
  review_required?: boolean;
  priority?: number;
  status?: RetentionScheduleStatus;
  effective_from?: string;
  effective_until?: string;
  clear_effective_until?: boolean;
  reason: string;
}

export interface RetentionScheduleListParams {
  record_type?: GovernanceRecordType;
  status?: RetentionScheduleStatus;
  classification?: DataClassification;
  jurisdiction?: string;
  search?: string;
  sort_by?:
    'name' | 'record_type' | 'retention_days' | 'priority' | 'effective_from' | 'updated_at';
  sort_direction?: 'asc' | 'desc';
  page?: number;
  page_size?: number;
}

export interface RecordRetentionAssignment extends GovernanceTenantModel {
  schedule_id: string;
  record_type: GovernanceRecordType;
  record_id: string;
  data_classification?: DataClassification;
  jurisdiction?: string;
  trigger_event: RetentionTrigger;
  retention_started_at: string;
  archive_eligible_at?: string;
  disposition_due_at: string;
  disposition_action: DispositionAction;
  review_required: boolean;
  review_status: string;
  reviewed_by?: string;
  reviewed_at?: string;
  review_reason?: string;
  state: string;
  source: string;
  reason: string;
  version: number;
  created_by: string;
  disposed_at?: string;
}

export interface RetentionAssignmentInput {
  schedule_id: string;
  record_type: GovernanceRecordType;
  record_id: string;
  data_classification?: DataClassification;
  jurisdiction?: string;
  retention_started_at: string;
  source?: string;
  reason: string;
}

export interface RetentionReviewInput {
  expected_version: number;
  decision: RetentionDecision;
  reason: string;
}

export interface RetentionException extends GovernanceTenantModel {
  assignment_id: string;
  requested_until: string;
  reason: string;
  status: string;
  requested_by: string;
  decided_by?: string;
  decided_at?: string;
  decision_reason?: string;
  version: number;
}

export interface RetentionExceptionInput {
  requested_until: string;
  reason: string;
}

export interface RetentionExceptionDecisionInput {
  expected_version: number;
  decision: RetentionDecision;
  reason: string;
}

export interface LegalHold extends GovernanceTenantModel {
  hold_ref: string;
  name: string;
  matter_reference?: string;
  description: string;
  legal_authority: string;
  scope: Record<string, unknown>;
  status: LegalHoldStatus;
  owner_user_id: string;
  placed_by: string;
  placed_at: string;
  review_due_at?: string;
  released_by?: string;
  released_at?: string;
  release_reason?: string;
  version: number;
  custodian_ids: string[];
  record_count: number;
}

export interface LegalHoldInput {
  name: string;
  matter_reference?: string;
  description: string;
  legal_authority: string;
  scope?: Record<string, unknown>;
  owner_user_id: string;
  review_due_at?: string;
  custodian_ids?: string[];
  reason: string;
}

export interface LegalHoldPatch {
  expected_version: number;
  name?: string;
  matter_reference?: string;
  clear_matter_reference?: boolean;
  description?: string;
  legal_authority?: string;
  scope?: Record<string, unknown>;
  owner_user_id?: string;
  review_due_at?: string;
  clear_review_due_at?: boolean;
  reason: string;
}

export interface LegalHoldReleaseInput {
  expected_version: number;
  outcome: LegalHoldOutcome;
  reason: string;
}

export interface LegalHoldRecord {
  id: string;
  organization_id: string;
  hold_id: string;
  record_type: GovernanceRecordType;
  record_id: string;
  reason: string;
  placed_by: string;
  placed_at: string;
  released_at?: string;
  released_by?: string;
  release_reason?: string;
}

export interface LegalHoldRecordInput {
  record_type: GovernanceRecordType;
  record_id: string;
  reason: string;
}

export interface LegalHoldRecordReleaseInput {
  reason: string;
}

export interface DataGovernanceEvent {
  id: string;
  chain_sequence: number;
  previous_hash: string;
  event_hash: string;
  entity_type:
    'policy' | 'retention_schedule' | 'retention_assignment' | 'retention_exception' | 'legal_hold';
  entity_id: string;
  event_type: string;
  actor_user_id: string;
  reason: string;
  before_state?: Record<string, unknown>;
  after_state?: Record<string, unknown>;
  source: string;
  request_id?: string;
  created_at: string;
}

export interface DataGovernanceEventListParams {
  entity_type?: DataGovernanceEvent['entity_type'];
  entity_id?: string;
  event_type?: string;
  page?: number;
  page_size?: number;
}

export interface GovernanceChainVerification {
  valid: boolean;
  event_count: number;
  last_sequence: number;
  last_hash: string;
  verified_at: string;
  failure_reason?: string;
}

export interface RecordDispositionDecision {
  record_type: GovernanceRecordType;
  record_id: string;
  allowed: boolean;
  reason: string;
  assignment?: RecordRetentionAssignment;
  active_legal_holds: LegalHoldRecord[];
  evaluated_at: string;
}

export interface RetireRetentionScheduleInput {
  expected_version: number;
  reason: string;
}
