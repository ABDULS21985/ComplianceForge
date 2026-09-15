import type { PaginatedDataEnvelope } from '@/types/enterprise-settings';

export type EvidenceType =
  | 'document'
  | 'screenshot'
  | 'log'
  | 'configuration'
  | 'report'
  | 'certificate'
  | 'interview_notes'
  | 'test_result'
  | 'policy'
  | 'procedure'
  | 'training_record';

export type EvidenceCollectionMethod =
  'manual_upload' | 'automated' | 'api_pull' | 'scan_result' | 'integration';

export type EvidenceReviewStatus = 'pending' | 'accepted' | 'rejected' | 'expired';
export type EvidenceLifecycleStatus = 'active' | 'superseded' | 'expired';

export type EvidenceCustodyEventType =
  | 'uploaded'
  | 'download_authorized'
  | 'reviewed'
  | 'superseded'
  | 'expired'
  | 'deleted'
  | 'integrity_verified'
  | 'integrity_failed'
  | 'legal_hold_placed'
  | 'legal_hold_released';

export interface ControlEvidence {
  id: string;
  organization_id: string;
  control_implementation_id: string;
  title: string;
  description?: string;
  evidence_type: EvidenceType;
  file_name?: string;
  file_size_bytes?: number;
  mime_type?: string;
  file_hash?: string;
  collection_method: EvidenceCollectionMethod;
  collected_at: string;
  collected_by?: string;
  valid_from?: string;
  valid_until?: string;
  is_current: boolean;
  review_status: EvidenceReviewStatus;
  reviewed_by?: string;
  reviewed_at?: string;
  review_notes?: string;
  metadata: Record<string, unknown>;
  series_id: string;
  version_number: number;
  supersedes_evidence_id?: string;
  superseded_by_evidence_id?: string;
  superseded_at?: string;
  lifecycle_status: EvidenceLifecycleStatus;
  expires_at?: string;
  version_reason: string;
  content_fingerprint: string;
  created_at: string;
  updated_at: string;
}

export interface ControlEvidenceUploadInput {
  file: File;
  title: string;
  evidence_type: EvidenceType;
  description?: string;
  valid_from?: string;
  valid_until?: string;
  metadata?: Record<string, unknown>;
}

export interface ControlEvidenceReviewInput {
  status: 'accepted' | 'rejected';
  comment?: string;
}

export interface ControlEvidenceSupersedeInput extends ControlEvidenceUploadInput {
  version_reason: string;
}

export interface EvidenceReview {
  id: string;
  organization_id: string;
  evidence_id: string;
  decision: 'accepted' | 'rejected';
  comment?: string;
  reviewer_id: string;
  evidence_sha256: string;
  request_id?: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface EvidenceCustodyEvent {
  id: string;
  organization_id: string;
  evidence_id: string;
  series_id: string;
  sequence: number;
  previous_hash: string;
  event_hash: string;
  event_type: EvidenceCustodyEventType;
  actor_user_id?: string;
  actor_type: 'user' | 'system';
  reason: string;
  object_sha256?: string;
  request_id?: string;
  details: Record<string, unknown>;
  created_at: string;
}

export interface EvidenceCustodyChainVerification {
  evidence_id: string;
  valid: boolean;
  event_count: number;
  head_sequence: number;
  head_hash: string;
  first_invalid_sequence?: number;
}

export interface EvidenceLifecycleRecord {
  evidence: ControlEvidence;
  versions: ControlEvidence[];
  reviews: EvidenceReview[];
  custody_events: EvidenceCustodyEvent[];
  chain: EvidenceCustodyChainVerification;
  legal_hold_active: boolean;
}

export interface EvidenceIntegrityResult {
  evidence_id: string;
  valid: boolean;
  sha256: string;
  size_bytes: number;
  verified_at: string;
}

export interface ControlEvidenceListParams {
  page?: number;
  page_size?: number;
}

export type ControlEvidenceEnvelope = PaginatedDataEnvelope<ControlEvidence>;

export type ControlPriority = 'critical' | 'high' | 'medium' | 'low';
export type ControlImplementationStatus = 'not_started' | 'in_progress' | 'completed' | 'failed';
export type ControlImplementationState =
  'not_applicable' | 'not_implemented' | 'planned' | 'partial' | 'implemented' | 'effective';

export interface ControlImplementationRecord {
  id: string;
  organization_id: string;
  framework_control_id: string;
  organization_framework_id: string;
  status: ControlImplementationState;
  implementation_status: ControlImplementationStatus;
  maturity_level: number;
  owner_user_id?: string;
  reviewer_user_id?: string;
  implementation_description?: string;
  implementation_notes?: string;
  gap_description?: string;
  remediation_plan?: string;
  remediation_due_date?: string;
  automation_level?: 'fully_automated' | 'semi_automated' | 'manual';
  tags: string[];
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ControlRecord {
  id: string;
  framework_id: string;
  domain_id?: string;
  code: string;
  title: string;
  description?: string;
  guidance?: string;
  category?: string;
  objective?: string;
  control_type?: string;
  implementation_type?: string;
  is_mandatory?: boolean;
  priority?: ControlPriority;
  sort_order?: number;
  parent_control_id?: string;
  depth_level?: number;
  evidence_requirements?: unknown;
  test_procedures?: unknown;
  references?: unknown;
  keywords?: string[];
  metadata?: Record<string, unknown>;
  implementation?: ControlImplementationRecord;
  created_at?: string;
  updated_at?: string;
}

export interface ControlImplementationPatch {
  status?: ControlImplementationState;
  implementation_status?: ControlImplementationStatus;
  maturity_level?: number;
  owner_user_id?: string;
  reviewer_user_id?: string;
  implementation_description?: string;
  implementation_notes?: string;
  gap_description?: string;
  remediation_plan?: string;
  remediation_due_date?: string;
  automation_level?: 'fully_automated' | 'semi_automated' | 'manual';
  tags?: string[];
}

export type ControlEnvelope = PaginatedDataEnvelope<ControlRecord>;
