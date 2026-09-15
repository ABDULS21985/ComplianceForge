export const SUPPORT_BUNDLE_ROUTE = '/settings/diagnostics/support-bundle';
export const SUPPORT_BUNDLE_SCOPE = 'health_and_posture';
export const SUPPORT_BUNDLE_REDACTION_PROFILE = 'health_posture_allowlist_v1';
export const SUPPORT_BUNDLE_MAX_BYTES = 64 * 1024;
export const SUPPORT_BUNDLE_MAX_REQUEST_BYTES = 1024;
export const SUPPORT_BUNDLE_SHA_HEADER = 'x-support-bundle-sha256';
export const SUPPORT_BUNDLE_MEMBERS = [
  'configuration.json', 'health.json', 'README.txt', 'manifest.json',
] as const;
export const SUPPORT_BUNDLE_EXCLUSIONS = [
  'credentials', 'dependency_endpoints', 'origins_and_paths', 'raw_logs',
  'event_payloads', 'business_records', 'user_identifiers', 'domain_record_identifiers',
] as const;
