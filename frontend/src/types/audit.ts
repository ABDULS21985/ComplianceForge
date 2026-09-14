export type AuditType = 'internal' | 'external' | 'certification';

export type AuditStatus = 'planned' | 'in_progress' | 'completed' | 'closed' | 'cancelled';

export type AuditLifecycleAction = 'start' | 'complete' | 'close' | 'cancel';

export type FindingSeverity = 'critical' | 'high' | 'medium' | 'low' | 'informational';

export type FindingStatus = 'open' | 'in_progress' | 'resolved' | 'closed' | 'accepted';

export interface AuditPerson {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
}

export interface AuditFramework {
  id: string;
  code: string;
  name: string;
}

interface AuditTenantFields {
  id: string;
  organization_id: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

export interface Audit extends AuditTenantFields {
  audit_ref: string;
  title: string;
  description: string;
  audit_type: AuditType;
  status: AuditStatus;
  lead_auditor_id: string;
  lead_auditor?: AuditPerson;
  scope: string;
  scheduled_start_date?: string;
  scheduled_end_date?: string;
  actual_start_date?: string;
  actual_end_date?: string;
  framework_id?: string;
  framework?: AuditFramework;
  created_by: string;
  metadata: Record<string, unknown> | null;
  findings_count: number;
  critical_findings_open: number;
  high_findings_open: number;
}

export interface AuditFinding extends AuditTenantFields {
  audit_id: string;
  finding_ref: string;
  control_id?: string;
  title: string;
  description: string;
  severity: FindingSeverity;
  status: FindingStatus;
  finding_type: string;
  root_cause: string;
  recommendation: string;
  remediation_plan: string;
  responsible_user_id: string;
  responsible_user?: AuditPerson;
  due_date?: string;
  resolved_at?: string;
  accepted_risk_reason?: string;
  created_by: string;
  metadata: Record<string, unknown> | null;
}

export interface AuditCreateInput {
  title: string;
  description: string;
  audit_type: AuditType;
  lead_auditor_id: string;
  scope: string;
  scheduled_start_date: string;
  scheduled_end_date: string;
  framework_id?: string;
  metadata?: Record<string, unknown>;
}

export interface AuditPatch {
  title?: string;
  description?: string;
  audit_type?: AuditType;
  lead_auditor_id?: string;
  scope?: string;
  scheduled_start_date?: string;
  scheduled_end_date?: string;
  framework_id?: string;
  clear_framework?: boolean;
  metadata?: Record<string, unknown>;
}

export interface AuditListParams {
  page?: number;
  page_size?: number;
  status?: AuditStatus;
  audit_type?: AuditType;
  lead_auditor_id?: string;
  framework_id?: string;
  search?: string;
}

export interface AuditFindingCreateInput {
  control_id?: string;
  title: string;
  description: string;
  severity: FindingSeverity;
  finding_type: string;
  root_cause?: string;
  recommendation: string;
  remediation_plan?: string;
  responsible_user_id: string;
  due_date: string;
  metadata?: Record<string, unknown>;
}

export interface AuditFindingPatch {
  control_id?: string;
  clear_control?: boolean;
  title?: string;
  description?: string;
  severity?: FindingSeverity;
  status?: FindingStatus;
  finding_type?: string;
  root_cause?: string;
  recommendation?: string;
  remediation_plan?: string;
  responsible_user_id?: string;
  due_date?: string;
  accepted_risk_reason?: string;
  metadata?: Record<string, unknown>;
}

export interface AuditFindingListParams {
  page?: number;
  page_size?: number;
}

export interface AuditFindingStats {
  total: number;
  open: number;
  in_progress: number;
  resolved: number;
  closed: number;
  accepted: number;
  critical_open: number;
  high_open: number;
  overdue: number;
}

/** Exact collection envelope produced by the Go handlers. */
export interface AuditCollectionEnvelope<T> {
  data: T[];
  pagination: {
    page: number;
    page_size: number;
    total_items: number;
    total_pages: number;
  };
}

/** Stable page shape consumed by audit UI after boundary normalization. */
export interface AuditPage<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export type FindingsStats = AuditFindingStats;
