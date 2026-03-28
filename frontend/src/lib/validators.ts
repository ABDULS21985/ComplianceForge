// ComplianceForge Zod Validators
// Form validation schemas for every major entity

import { z } from "zod";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

const uuid = z.string().uuid();
const optionalUuid = z.string().uuid().optional();
const optionalString = z.string().optional();
const tagArray = z.array(z.string()).optional();

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

export const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(1, "Password is required"),
});
export type LoginInput = z.infer<typeof loginSchema>;

// ---------------------------------------------------------------------------
// Risk
// ---------------------------------------------------------------------------

export const riskSourceEnum = z.enum([
  "internal",
  "external",
  "regulatory",
  "operational",
  "strategic",
  "third_party",
  "technology",
  "environmental",
]);

export const riskVelocityEnum = z.enum(["immediate", "days", "weeks", "months", "years"]);

export const reviewFrequencyEnum = z.enum(["monthly", "quarterly", "semi_annual", "annual"]);

const likelihoodImpact = z.number().int().min(1).max(5);

export const createRiskSchema = z.object({
  title: z
    .string()
    .min(5, "Title must be at least 5 characters")
    .max(500, "Title must be at most 500 characters"),
  description: optionalString,
  risk_category_id: uuid,
  risk_source: riskSourceEnum,
  owner_user_id: optionalUuid,
  inherent_likelihood: likelihoodImpact,
  inherent_impact: likelihoodImpact,
  residual_likelihood: z.number().int().min(1).max(5).optional(),
  residual_impact: z.number().int().min(1).max(5).optional(),
  financial_impact_eur: z.number().nonnegative().optional(),
  risk_velocity: riskVelocityEnum.optional(),
  review_frequency: reviewFrequencyEnum.optional(),
  tags: tagArray,
});
export type CreateRiskInput = z.infer<typeof createRiskSchema>;

// ---------------------------------------------------------------------------
// Policy
// ---------------------------------------------------------------------------

export const policyClassificationEnum = z.enum([
  "public",
  "internal",
  "confidential",
  "restricted",
]);

export const createPolicySchema = z.object({
  title: z.string().min(5, "Title must be at least 5 characters"),
  category_id: uuid,
  classification: policyClassificationEnum,
  content_html: optionalString,
  summary: optionalString,
  owner_user_id: uuid,
  approver_user_id: uuid,
  review_frequency_months: z.number().int().min(1).max(36),
  is_mandatory: z.boolean(),
  requires_attestation: z.boolean(),
  tags: tagArray,
});
export type CreatePolicyInput = z.infer<typeof createPolicySchema>;

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

export const auditTypeEnum = z.enum([
  "internal",
  "external",
  "certification",
  "surveillance",
  "follow_up",
]);

export const createAuditSchema = z.object({
  title: z.string().min(1, "Title is required"),
  description: z.string().min(1, "Description is required"),
  audit_type: auditTypeEnum,
  lead_auditor_id: uuid,
  scope: z.string().min(1, "Scope is required"),
  scheduled_start_date: z.string().min(1, "Start date is required"),
  scheduled_end_date: z.string().min(1, "End date is required"),
  framework_id: optionalUuid,
});
export type CreateAuditInput = z.infer<typeof createAuditSchema>;

// ---------------------------------------------------------------------------
// Audit Finding
// ---------------------------------------------------------------------------

export const findingSeverityEnum = z.enum([
  "critical",
  "high",
  "medium",
  "low",
  "informational",
]);

export const createFindingSchema = z.object({
  title: z.string().min(1, "Title is required"),
  description: z.string().min(1, "Description is required"),
  severity: findingSeverityEnum,
  finding_type: z.string().min(1, "Finding type is required"),
  control_id: optionalUuid,
  root_cause: z.string().min(1, "Root cause is required"),
  recommendation: z.string().min(1, "Recommendation is required"),
  responsible_user_id: uuid,
  due_date: z.string().min(1, "Due date is required"),
});
export type CreateFindingInput = z.infer<typeof createFindingSchema>;

// ---------------------------------------------------------------------------
// Incident
// ---------------------------------------------------------------------------

export const incidentSeverityEnum = z.enum([
  "critical",
  "high",
  "medium",
  "low",
]);

export const reportIncidentSchema = z
  .object({
    title: z.string().min(1, "Title is required"),
    description: z.string().min(1, "Description is required"),
    incident_type: z.string().min(1, "Incident type is required"),
    severity: incidentSeverityEnum,
    category: z.string().min(1, "Category is required"),
    is_data_breach: z.boolean(),
    data_subjects_affected: z.number().int().nonnegative().optional(),
    data_categories: z.array(z.string()).optional(),
  })
  .superRefine((data, ctx) => {
    if (data.is_data_breach) {
      if (data.data_subjects_affected === undefined || data.data_subjects_affected === null) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Number of data subjects affected is required for data breaches",
          path: ["data_subjects_affected"],
        });
      }
      if (!data.data_categories || data.data_categories.length === 0) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Data categories are required for data breaches",
          path: ["data_categories"],
        });
      }
    }
  });
export type ReportIncidentInput = z.infer<typeof reportIncidentSchema>;

// ---------------------------------------------------------------------------
// Vendor
// ---------------------------------------------------------------------------

export const vendorRiskTierEnum = z.enum(["critical", "high", "medium", "low"]);

export const onboardVendorSchema = z
  .object({
    name: z.string().min(1, "Name is required"),
    legal_name: z.string().min(1, "Legal name is required"),
    website: z.string().url("Must be a valid URL"),
    country_code: z.string().length(2, "Must be a 2-letter country code"),
    contact_name: z.string().min(1, "Contact name is required"),
    contact_email: z.string().email("Invalid email"),
    risk_tier: vendorRiskTierEnum,
    service_description: z.string().min(1, "Service description is required"),
    data_processing: z.boolean(),
    data_categories: z.array(z.string()).optional(),
    certifications: z.array(z.string()).optional(),
    owner_user_id: uuid,
  })
  .superRefine((data, ctx) => {
    if (data.data_processing) {
      if (!data.data_categories || data.data_categories.length === 0) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Data categories are required when vendor processes data",
          path: ["data_categories"],
        });
      }
    }
  });
export type OnboardVendorInput = z.infer<typeof onboardVendorSchema>;

// ---------------------------------------------------------------------------
// Asset
// ---------------------------------------------------------------------------

export const assetTypeEnum = z.enum([
  "hardware",
  "software",
  "data",
  "service",
  "network",
  "facility",
  "personnel",
]);

export const assetCriticalityEnum = z.enum([
  "critical",
  "high",
  "medium",
  "low",
]);

export const assetClassificationEnum = z.enum([
  "public",
  "internal",
  "confidential",
  "restricted",
]);

export const registerAssetSchema = z.object({
  name: z.string().min(1, "Name is required"),
  asset_type: assetTypeEnum,
  category: z.string().min(1, "Category is required"),
  description: z.string().min(1, "Description is required"),
  criticality: assetCriticalityEnum,
  owner_user_id: uuid,
  location: z.string().min(1, "Location is required"),
  classification: assetClassificationEnum,
  processes_personal_data: z.boolean(),
  tags: tagArray,
});
export type RegisterAssetInput = z.infer<typeof registerAssetSchema>;

// ---------------------------------------------------------------------------
// User
// ---------------------------------------------------------------------------

export const createUserSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(12, "Password must be at least 12 characters"),
  first_name: z.string().min(1, "First name is required"),
  last_name: z.string().min(1, "Last name is required"),
  job_title: z.string().min(1, "Job title is required"),
  department: z.string().min(1, "Department is required"),
  role_ids: z.array(uuid).min(1, "At least one role is required"),
});
export type CreateUserInput = z.infer<typeof createUserSchema>;
