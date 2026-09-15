export interface SupportBundleRequest {
  consent: true;
  scope: 'health_and_posture';
}

export interface SupportBundleFile {
  name: 'configuration.json' | 'health.json' | 'README.txt';
  bytes: number;
  sha256: string;
}

/** Internal ZIP metadata, not a JSON API response or a reusable sharing grant. */
export interface SupportBundleManifest {
  schema_version: 1;
  bundle_id: string;
  organization_id: string;
  generated_at: string;
  scope: 'health_and_posture';
  consent_recorded_at: string;
  redaction_profile: 'health_posture_allowlist_v1';
  configuration_fingerprint: string;
  automatically_transmitted: false;
  files: SupportBundleFile[];
  excluded: string[];
}

export interface VerifiedSupportBundle {
  blob: Blob;
  filename: string;
  sha256: string;
  manifest: SupportBundleManifest;
}
