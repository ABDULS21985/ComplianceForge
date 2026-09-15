import { describe, expect, it } from 'vitest';
import { ACCESS_ADMIN_ROUTES } from '@/lib/access-admin';
import type { Asset } from '@/types/asset';
import { ASSET_API_ROUTES } from '@/lib/asset';
import { DIRECTORY_ROUTES } from '@/lib/directory';
import type { DirectoryUser } from '@/types/directory';
import { FEATURE_FLAG_ROUTES } from '@/lib/feature-flags';
import { fileURLToPath } from 'node:url';
import type { Incident } from '@/types/incident';
import { INCIDENT_API_ROUTES } from '@/lib/incident';
import type { IntegrationSyncLog } from '@/types/enterprise-settings';
import type { ManagedRole } from '@/types/access-admin';
import { readFileSync } from 'node:fs';

interface OpenAPISchema {
  properties?: Record<string, unknown>;
}

interface OpenAPIDocument {
  components: { schemas: Record<string, OpenAPISchema> };
  paths: Record<string, Record<string, unknown>>;
}

type ExactStringKeys<T, Keys extends readonly string[]> =
  Exclude<Extract<keyof T, string>, Keys[number]> extends never
    ? Exclude<Keys[number], Extract<keyof T, string>> extends never
      ? true
      : false
    : false;

type Assert<T extends true> = T;

const incidentKeys = [
  'id', 'organization_id', 'created_at', 'updated_at', 'deleted_at', 'incident_ref',
  'title', 'description', 'category', 'severity', 'status', 'reporter_id', 'assignee_id',
  'detected_at', 'reported_at', 'occurred_at', 'triaged_at', 'investigation_started_at',
  'contained_at', 'resolved_at', 'closed_at', 'cancelled_at', 'cancellation_reason',
  'reopened_at', 'root_cause', 'impact', 'lessons_learned', 'related_asset_id',
  'followup_date', 'is_data_breach', 'breach_assessment_status', 'is_breach_notifiable',
  'breach_assessment_reason', 'breach_assessed_at', 'breach_assessed_by',
  'breach_awareness_at', 'notification_deadline', 'data_subjects_affected', 'records_affected',
  'data_categories', 'special_category_data', 'cross_border', 'breach_nature',
  'likely_consequences', 'mitigation_measures', 'dpa_notified_at',
  'dpa_notification_reference', 'dpa_notification_reason', 'version', 'retention_until',
  'legal_hold', 'metadata', 'deadline_state', 'hours_remaining',
] as const;

const assetKeys = [
  'id', 'organization_id', 'created_at', 'updated_at', 'deleted_at', 'asset_ref', 'name',
  'asset_type', 'category', 'description', 'criticality', 'owner_user_id', 'owner', 'location',
  'ip_address', 'classification', 'processes_personal_data', 'linked_vendor_id', 'status',
  'tags', 'metadata', 'version', 'created_by',
] as const;

const managedRoleKeys = [
  'id', 'organization_id', 'name', 'slug', 'description', 'is_system_role', 'is_custom',
  'version', 'created_by', 'updated_by', 'permissions', 'assigned_users', 'created_at',
  'updated_at', 'deleted_at',
] as const;

const directoryUserKeys = [
  'id', 'organization_id', 'email', 'first_name', 'last_name', 'job_title', 'department',
  'phone', 'avatar_url', 'status', 'is_super_admin', 'timezone', 'language', 'last_login_at',
  'mfa_enabled', 'manager_user_id', 'manager', 'employee_id', 'location', 'invitation_status',
  'invited_at', 'invitation_expires_at', 'invited_by', 'suspended_at', 'suspended_by',
  'suspension_reason', 'reactivated_at', 'deprovisioned_at', 'deprovisioned_by',
  'deprovision_reason', 'updated_by', 'version', 'role_slugs', 'group_count', 'created_at',
  'updated_at', 'deleted_at',
] as const;

const integrationSyncLogKeys = [
  'id', 'integration_id', 'sync_type', 'status', 'records_processed', 'records_created',
  'records_updated', 'records_failed', 'duration_ms', 'error_message', 'created_at',
] as const;

const incidentTypeKeysAreExact: Assert<ExactStringKeys<Incident, typeof incidentKeys>> = true;
const assetTypeKeysAreExact: Assert<ExactStringKeys<Asset, typeof assetKeys>> = true;
const managedRoleTypeKeysAreExact: Assert<ExactStringKeys<ManagedRole, typeof managedRoleKeys>> = true;
const directoryUserTypeKeysAreExact: Assert<ExactStringKeys<DirectoryUser, typeof directoryUserKeys>> = true;
const integrationSyncLogTypeKeysAreExact: Assert<ExactStringKeys<IntegrationSyncLog, typeof integrationSyncLogKeys>> = true;
void [incidentTypeKeysAreExact, assetTypeKeysAreExact, managedRoleTypeKeysAreExact,
  directoryUserTypeKeysAreExact, integrationSyncLogTypeKeysAreExact];

const contractPath = fileURLToPath(new URL('../../../api/openapi/openapi.json', import.meta.url));
const contract = JSON.parse(readFileSync(contractPath, 'utf8')) as OpenAPIDocument;

function expectSchemaKeys(schemaName: string, expected: readonly string[]): void {
  const actual = Object.keys(contract.components.schemas[schemaName]?.properties ?? {}).sort();
  expect(actual).toEqual([...expected].sort());
}

function apiPath(path: string): string {
  return `/api/v1${path}`;
}

function expectOperation(method: string, path: string): void {
  expect(contract.paths[path]?.[method.toLowerCase()], `${method} ${path}`).toBeDefined();
}

describe('generated OpenAPI contract', () => {
  it('keeps material Go schemas aligned with the canonical frontend payload types', () => {
    expectSchemaKeys('Incident', incidentKeys);
    expectSchemaKeys('Asset', assetKeys);
    expectSchemaKeys('ManagedRole', managedRoleKeys);
    expectSchemaKeys('DirectoryUser', directoryUserKeys);
    expectSchemaKeys('IntegrationSyncLog', integrationSyncLogKeys);
  });

  it('documents the typed directory user collection route', () => {
    expectOperation('GET', apiPath(DIRECTORY_ROUTES.users));
  });

  it('documents the frontend incident and asset route constants', () => {
    const incidentId = 'contract-incident';
    const assignmentId = 'contract-assignment';
    const incidentPath = (path: string) => apiPath(path)
      .replace(incidentId, '{id}')
      .replace(assignmentId, '{assignmentID}');
    expectOperation('GET', apiPath(INCIDENT_API_ROUTES.collection));
    expectOperation('POST', apiPath(INCIDENT_API_ROUTES.collection));
    for (const method of ['GET', 'PUT', 'PATCH', 'DELETE']) {
      expectOperation(method, incidentPath(INCIDENT_API_ROUTES.detail(incidentId)));
    }
    expectOperation('GET', incidentPath(INCIDENT_API_ROUTES.timeline(incidentId)));
    expectOperation('POST', incidentPath(INCIDENT_API_ROUTES.transition(incidentId)));
    expectOperation('POST', incidentPath(INCIDENT_API_ROUTES.unassign(incidentId, assignmentId)));

    const assetId = 'contract-asset';
    const assetPath = (path: string) => apiPath(path).replace(assetId, '{id}');
    expectOperation('GET', apiPath(ASSET_API_ROUTES.collection));
    expectOperation('POST', apiPath(ASSET_API_ROUTES.collection));
    for (const method of ['GET', 'PUT', 'PATCH', 'DELETE']) {
      expectOperation(method, assetPath(ASSET_API_ROUTES.detail(assetId)));
    }
    expectOperation('GET', assetPath(ASSET_API_ROUTES.events(assetId)));
  });

  it('documents Access Administration and product capability route constants', () => {
    const roleId = 'contract-role';
    const userId = 'contract-user';
    const accessPath = (path: string) => apiPath(path)
      .replace(roleId, '{id}')
      .replace(userId, '{userID}');
    expectOperation('GET', apiPath(ACCESS_ADMIN_ROUTES.permissions));
    expectOperation('GET', apiPath(ACCESS_ADMIN_ROUTES.roles));
    expectOperation('POST', apiPath(ACCESS_ADMIN_ROUTES.roles));
    for (const method of ['GET', 'PUT', 'PATCH', 'DELETE']) {
      expectOperation(method, accessPath(ACCESS_ADMIN_ROUTES.role(roleId)));
    }
    expectOperation('POST', accessPath(ACCESS_ADMIN_ROUTES.clone(roleId)));
    expectOperation('POST', accessPath(ACCESS_ADMIN_ROUTES.impact(roleId)));
    expectOperation('POST', accessPath(ACCESS_ADMIN_ROUTES.assignments(roleId)));
    expectOperation('DELETE', accessPath(ACCESS_ADMIN_ROUTES.assignment(roleId, userId)));
    expectOperation('GET', accessPath(ACCESS_ADMIN_ROUTES.events(roleId)));

    const capabilityKey = 'contract-capability';
    const metric = 'contract-metric';
    const featurePath = (path: string) => apiPath(path)
      .replace(capabilityKey, '{key}')
      .replace(metric, '{metric}');
    expectOperation('GET', apiPath(FEATURE_FLAG_ROUTES.capabilities));
    expectOperation('GET', featurePath(FEATURE_FLAG_ROUTES.evaluation(capabilityKey)));
    expectOperation('GET', apiPath(FEATURE_FLAG_ROUTES.entitlements));
    expectOperation('GET', featurePath(FEATURE_FLAG_ROUTES.limit(metric)));
    expectOperation('GET', apiPath(FEATURE_FLAG_ROUTES.flags));
    expectOperation('PUT', featurePath(FEATURE_FLAG_ROUTES.flag(capabilityKey)));
    expectOperation('POST', featurePath(FEATURE_FLAG_ROUTES.reset(capabilityKey)));
    expectOperation('GET', featurePath(FEATURE_FLAG_ROUTES.history(capabilityKey)));
  });
});
