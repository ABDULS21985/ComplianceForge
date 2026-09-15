import type { ControlEvidence as ExactControlEvidence } from './control-evidence';

export * from './access';
export * from './audit';
export * from './asset';
export * from './control-evidence';
export * from './incident';

// === API Response Wrappers ===
export interface APIResponse<T> {
  data: T;
  message?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: Pagination;
}

export interface Pagination {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
}

export interface APIError {
  error: string;
  message: string;
  status_code: number;
  details?: Record<string, string>;
}

// === Enums ===
export type RiskLevel = 'critical' | 'high' | 'medium' | 'low' | 'very_low';
export type ControlStatus = 'not_applicable' | 'not_implemented' | 'planned' | 'partial' | 'implemented' | 'effective';
export type ImplementationStatus = 'not_started' | 'in_progress' | 'completed' | 'failed';
export type PolicyStatus = 'draft' | 'under_review' | 'pending_approval' | 'approved' | 'published' | 'archived' | 'retired' | 'superseded';
export type VendorRiskTier = 'critical' | 'high' | 'medium' | 'low';
export type Classification = 'public' | 'internal' | 'confidential' | 'restricted';
export type ReviewStatus = 'current' | 'review_due' | 'overdue' | 'not_applicable';
export type TreatmentType = 'mitigate' | 'transfer' | 'avoid' | 'accept';

// === Auth ===
export interface LoginRequest { email: string; password: string; }
/** Browser-safe session response. Bearer credentials are HttpOnly cookies. */
export interface LoginResponse { expires_at: string; user: User; }
export interface User {
  id: string; organization_id: string; email: string; first_name: string; last_name: string;
  job_title?: string; department?: string; phone?: string; avatar_url?: string;
  status: string; is_super_admin: boolean; timezone?: string; language: string;
  last_login_at?: string; created_at: string; updated_at: string;
  roles?: Role[];
}
export interface Role {
  id: string; name: string; slug: string; description?: string;
  is_system_role: boolean; is_custom: boolean;
}

// === Organizations ===
export interface Organization {
  id: string; name: string; slug: string; legal_name?: string; industry?: string;
  country_code?: string; status: string; tier: string; timezone: string;
  default_language: string; employee_count_range?: string; settings: Record<string, unknown>;
  branding: Record<string, unknown>; created_at: string; updated_at: string;
}

// === Frameworks ===
export interface ComplianceFramework {
  id: string; code: string; name: string; full_name?: string; version: string;
  description?: string; issuing_body?: string; category?: string;
  applicable_regions: string[]; applicable_industries: string[];
  is_system_framework: boolean; is_active: boolean; effective_date?: string;
  sunset_date?: string; total_controls: number; icon_url?: string;
  color_hex?: string; metadata: Record<string, unknown>;
  created_at: string; updated_at: string;
}

export interface FrameworkDomain {
  id: string; framework_id: string; code: string; name: string;
  description?: string; sort_order: number; parent_domain_id?: string;
  depth_level: number; total_controls: number;
}

export interface FrameworkControl {
  id: string; framework_id: string; domain_id?: string; code: string;
  title: string; description?: string; guidance?: string; objective?: string;
  control_type?: string; implementation_type?: string; is_mandatory: boolean;
  priority?: string; sort_order: number; parent_control_id?: string;
  depth_level: number; evidence_requirements: unknown[]; test_procedures: unknown[];
  keywords: string[]; created_at: string; updated_at: string;
}

// === Compliance ===
export interface ComplianceScore {
  organization_id: string; org_framework_id: string; framework_id: string;
  framework_code: string; framework_name: string; framework_version: string;
  adoption_status: string; compliance_score: number; avg_maturity_level: number;
  total_controls: number; effective_count: number; implemented_count: number;
  partial_count: number; planned_count: number; not_implemented_count: number;
  not_applicable_count: number; overdue_remediations: number;
  tested_controls: number; failed_tests: number;
}

export interface GapAnalysisEntry {
  organization_id: string; control_implementation_id: string; framework_code: string;
  framework_name: string; domain_code?: string; domain_name?: string;
  control_code: string; control_title: string; control_priority?: string;
  control_type?: string; status: ControlStatus; implementation_status: ImplementationStatus;
  maturity_level: number; gap_description?: string; remediation_plan?: string;
  remediation_due_date?: string; risk_if_not_implemented?: string;
  owner_user_id?: string; is_overdue: boolean; days_until_due?: number;
  current_evidence_count: number;
}

export interface CrossFrameworkMapping {
  source_framework_code: string; source_framework_name: string;
  source_control_code: string; source_control_title: string;
  target_framework_code: string; target_framework_name: string;
  target_control_code: string; target_control_title: string;
  mapping_type: string; mapping_strength: number; is_verified: boolean;
  effective_coverage: number;
}

// === Control Implementation ===
export interface ControlImplementation {
  id: string; organization_id: string; framework_control_id: string;
  org_framework_id: string; status: ControlStatus;
  implementation_status: ImplementationStatus; maturity_level: number;
  owner_user_id?: string; reviewer_user_id?: string;
  implementation_description?: string; implementation_notes?: string;
  compensating_control_description?: string; gap_description?: string;
  remediation_plan?: string; remediation_due_date?: string;
  test_frequency?: string; last_tested_at?: string; last_tested_by?: string;
  last_test_result?: string; effectiveness_score?: number;
  risk_if_not_implemented?: string; automation_level?: string;
  tags: string[]; created_at: string; updated_at: string;
  control?: FrameworkControl; evidence?: ExactControlEvidence[];
  test_results?: ControlTestResult[];
}

export interface ControlTestResult {
  id: string; organization_id: string; control_implementation_id: string;
  test_type: string; test_procedure?: string; result: string;
  findings?: string; recommendations?: string; tested_by?: string;
  tested_at: string; next_test_date?: string; created_at: string;
}

// === Risks ===
export interface Risk {
  id: string; organization_id: string; risk_ref: string; title: string;
  description?: string; risk_category_id?: string; risk_source?: string;
  risk_type?: string; status: string; owner_user_id?: string;
  delegate_user_id?: string; risk_matrix_id?: string;
  inherent_likelihood?: number; inherent_impact?: number;
  inherent_risk_score?: number; inherent_risk_level?: RiskLevel;
  residual_likelihood?: number; residual_impact?: number;
  residual_risk_score?: number; residual_risk_level?: RiskLevel;
  target_likelihood?: number; target_impact?: number;
  target_risk_score?: number; target_risk_level?: RiskLevel;
  financial_impact_eur?: number; impact_description?: string;
  impact_categories?: Record<string, number>; risk_velocity?: string;
  risk_proximity?: string; identified_date: string; last_assessed_date?: string;
  next_review_date?: string; review_frequency?: string;
  linked_regulations: string[]; linked_control_ids: string[];
  tags: string[]; is_emerging: boolean; created_at: string; updated_at: string;
  category?: RiskCategory; owner?: User; treatments?: RiskTreatment[];
}

export interface RiskCategory {
  id: string; name: string; code: string; description?: string;
  color_hex?: string; icon?: string; is_system_default: boolean;
}

export interface RiskHeatmapEntry {
  risk_id: string; risk_ref: string; title: string; status: string;
  category_name?: string; inherent_likelihood?: number; inherent_impact?: number;
  inherent_risk_score?: number; inherent_risk_level?: RiskLevel;
  residual_likelihood?: number; residual_impact?: number;
  residual_risk_score?: number; residual_risk_level?: RiskLevel;
  owner_name?: string; financial_impact_eur?: number;
}

export interface RiskTreatment {
  id: string; risk_id: string; treatment_type: TreatmentType;
  title: string; description?: string; status: string; priority?: string;
  owner_user_id?: string; start_date?: string; target_date?: string;
  completed_date?: string; estimated_cost_eur?: number; actual_cost_eur?: number;
  expected_risk_reduction?: number; progress_percentage: number;
  created_at: string; updated_at: string;
}

// === Policies ===
export interface Policy {
  id: string; organization_id: string; policy_ref: string; title: string;
  category_id?: string; status: PolicyStatus; classification: Classification;
  owner_user_id?: string; author_user_id?: string; approver_user_id?: string;
  current_version: number; current_version_id?: string;
  review_frequency_months: number; last_review_date?: string;
  next_review_date?: string; review_status: ReviewStatus;
  applies_to_all: boolean; effective_date?: string; expiry_date?: string;
  tags: string[]; priority?: string; is_mandatory: boolean;
  requires_attestation: boolean; attestation_frequency_months: number;
  created_at: string; updated_at: string;
  category?: PolicyCategory; owner?: User; current_version_detail?: PolicyVersion;
  attestation_rate?: number;
}

export interface PolicyCategory {
  id: string; name: string; code: string; description?: string;
  is_system_default: boolean;
}

export interface PolicyVersion {
  id: string; policy_id: string; version_number: number; version_label: string;
  title: string; content_html?: string; content_text?: string;
  summary?: string; change_description?: string; change_type?: string;
  language: string; word_count?: number; status: string;
  created_by?: string; published_at?: string; created_at: string;
}

export interface PolicyAttestation {
  id: string; policy_id: string; user_id: string; status: string;
  attested_at?: string; due_date?: string; user?: User;
}

export interface AttestationStats {
  total_policies: number; total_attestations: number; attested: number;
  pending: number; overdue: number; declined: number; average_rate: number;
}

// === Vendors ===
export interface Vendor {
  id: string; organization_id: string; vendor_ref: string; name: string;
  legal_name?: string; website?: string; industry?: string; country_code?: string;
  contact_name?: string; contact_email?: string; risk_tier?: VendorRiskTier;
  risk_score?: number; status: string; service_description?: string;
  data_processing: boolean; data_categories?: string[];
  dpa_in_place: boolean; dpa_signed_date?: string;
  certifications: string[]; contract_start_date?: string;
  contract_end_date?: string; contract_value_eur?: number;
  assessment_frequency?: string; last_assessment_date?: string;
  next_assessment_date?: string; owner_user_id?: string;
  created_at: string; updated_at: string; owner?: User;
}

export interface VendorStats {
  total: number; critical_risk: number; high_risk: number;
  missing_dpa: number; total_contract_value_eur: number;
}

// === Dashboard ===
export interface DashboardSummary {
  compliance_score: number; total_open_risks: number; critical_risks: number;
  open_incidents: number; critical_incidents: number;
  open_audit_findings: number; overdue_findings: number;
  policies_due_for_review: number; high_risk_vendors: number;
  risk_summary: Record<RiskLevel, number>;
  recent_activity: AuditLogEntry[];
}

export interface AuditLogEntry {
  id: string; user_id?: string; user_name?: string; action: string;
  entity_type: string; entity_id?: string; changes?: Record<string, unknown>;
  ip_address?: string; created_at: string;
}
