import type {
  ControlEvidence,
  ControlEvidenceSupersedeInput,
  ControlEvidenceUploadInput,
  EvidenceType,
} from '@/types/control-evidence';
import type { ApiError } from '@/lib/api';
import type { PermissionMap } from '@/types/access';
import type { User } from '@/types';

export const EVIDENCE_MAX_UPLOAD_BYTES = 25 * 1024 * 1024;
export const EVIDENCE_MAX_UPLOAD_MIB = 25;
export const EVIDENCE_METADATA_MAX_BYTES = 16 * 1024;
export const EVIDENCE_TITLE_MAX_LENGTH = 255;
export const EVIDENCE_DESCRIPTION_MAX_LENGTH = 4_000;
export const EVIDENCE_REVIEW_COMMENT_MAX_LENGTH = 4_000;
export const EVIDENCE_VERSION_REASON_MIN_LENGTH = 3;
export const EVIDENCE_VERSION_REASON_MAX_LENGTH = 1_000;
export const EVIDENCE_EXPIRING_SOON_DAYS = 30;

export const CONTROL_EVIDENCE_ROUTES = {
  controls: '/controls/',
  control: (controlId: string) => `/controls/${encodeURIComponent(controlId)}`,
  implementation: (controlId: string) =>
    `/controls/${encodeURIComponent(controlId)}/implementation`,
  evidence: (controlId: string) => `/controls/${encodeURIComponent(controlId)}/evidence`,
  download: (controlId: string, evidenceId: string) =>
    `/controls/${encodeURIComponent(controlId)}/evidence/${encodeURIComponent(evidenceId)}/download`,
  review: (controlId: string, evidenceId: string) =>
    `/controls/${encodeURIComponent(controlId)}/evidence/${encodeURIComponent(evidenceId)}/review`,
  history: (controlId: string, evidenceId: string) =>
    `/controls/${encodeURIComponent(controlId)}/evidence/${encodeURIComponent(evidenceId)}/history`,
  verifyIntegrity: (controlId: string, evidenceId: string) =>
    `/controls/${encodeURIComponent(controlId)}/evidence/${encodeURIComponent(evidenceId)}/verify-integrity`,
  supersede: (controlId: string, evidenceId: string) =>
    `/controls/${encodeURIComponent(controlId)}/evidence/${encodeURIComponent(evidenceId)}/supersede`,
} as const;

export interface EvidenceFileType {
  accept: readonly string[];
  extensions: readonly string[];
  label: string;
  mimeType: string;
}

export const EVIDENCE_FILE_TYPES: readonly EvidenceFileType[] = [
  { label: 'PDF', extensions: ['.pdf'], mimeType: 'application/pdf', accept: ['application/pdf'] },
  { label: 'PNG', extensions: ['.png'], mimeType: 'image/png', accept: ['image/png'] },
  { label: 'JPEG', extensions: ['.jpg', '.jpeg'], mimeType: 'image/jpeg', accept: ['image/jpeg'] },
  { label: 'Text', extensions: ['.txt', '.log'], mimeType: 'text/plain', accept: ['text/plain'] },
  {
    label: 'CSV',
    extensions: ['.csv'],
    mimeType: 'text/csv',
    accept: ['text/csv', 'application/vnd.ms-excel'],
  },
  {
    label: 'JSON',
    extensions: ['.json'],
    mimeType: 'application/json',
    accept: ['application/json'],
  },
  {
    label: 'Word',
    extensions: ['.docx'],
    mimeType: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    accept: ['application/vnd.openxmlformats-officedocument.wordprocessingml.document'],
  },
  {
    label: 'Excel',
    extensions: ['.xlsx'],
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    accept: ['application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'],
  },
  {
    label: 'PowerPoint',
    extensions: ['.pptx'],
    mimeType: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    accept: ['application/vnd.openxmlformats-officedocument.presentationml.presentation'],
  },
] as const;

export const EVIDENCE_ACCEPT = EVIDENCE_FILE_TYPES.flatMap((item) => item.extensions).join(',');
export const EVIDENCE_FILE_TYPE_LABEL = EVIDENCE_FILE_TYPES.map((item) => item.label).join(', ');

export const EVIDENCE_TYPES: readonly { label: string; value: EvidenceType }[] = [
  { value: 'document', label: 'Document' },
  { value: 'screenshot', label: 'Screenshot' },
  { value: 'log', label: 'Log' },
  { value: 'configuration', label: 'Configuration' },
  { value: 'report', label: 'Report' },
  { value: 'certificate', label: 'Certificate' },
  { value: 'interview_notes', label: 'Interview notes' },
  { value: 'test_result', label: 'Test result' },
  { value: 'policy', label: 'Policy' },
  { value: 'procedure', label: 'Procedure' },
  { value: 'training_record', label: 'Training record' },
] as const;

function fileExtension(filename: string): string {
  const index = filename.lastIndexOf('.');
  return index >= 0 ? filename.slice(index).toLowerCase() : '';
}

export function evidenceFileRule(file: Pick<File, 'name' | 'type'>): EvidenceFileType | undefined {
  const extension = fileExtension(file.name);
  return EVIDENCE_FILE_TYPES.find((item) => item.extensions.includes(extension));
}

export function evidenceFileValidationError(
  file: Pick<File, 'name' | 'size' | 'type'>,
): string | null {
  if (
    !file.name.trim() ||
    file.name.length > EVIDENCE_TITLE_MAX_LENGTH ||
    /[\\/\u0000-\u001f\u007f]/.test(file.name)
  ) {
    return 'Choose a file with a safe name of 255 characters or fewer.';
  }
  if (file.size === 0) return 'The evidence file is empty.';
  if (file.size > EVIDENCE_MAX_UPLOAD_BYTES) {
    return `The file exceeds the ${EVIDENCE_MAX_UPLOAD_MIB} MiB upload limit.`;
  }
  const rule = evidenceFileRule(file);
  if (!rule) return `Choose one of the supported formats: ${EVIDENCE_FILE_TYPE_LABEL}.`;
  if (file.type && !rule.accept.includes(file.type.toLowerCase())) {
    return `The file extension and reported file type do not match (${rule.label} expected).`;
  }
  return null;
}

/** Normalize an empty or browser-specific MIME declaration to the server's accepted type. */
export function normalizedEvidenceFile(file: File): File {
  const rule = evidenceFileRule(file);
  if (!rule || file.type === rule.mimeType) return file;
  return new File([file], file.name, { type: rule.mimeType, lastModified: file.lastModified });
}

export function parseEvidenceMetadata(value: string): Record<string, unknown> | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  if (new TextEncoder().encode(trimmed).byteLength > EVIDENCE_METADATA_MAX_BYTES) {
    throw new Error('Metadata must not exceed 16 KiB.');
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    throw new Error('Metadata must be valid JSON.');
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Metadata must be a JSON object.');
  }
  if (Object.prototype.hasOwnProperty.call(parsed, 'object_security')) {
    throw new Error('The object_security metadata field is reserved for the security service.');
  }
  return parsed as Record<string, unknown>;
}

export function validityError(validFrom?: string, validUntil?: string): string | null {
  if (!validFrom || !validUntil) return null;
  return validUntil < validFrom ? 'Valid until must be on or after valid from.' : null;
}

export function serializeEvidenceUpload(input: ControlEvidenceUploadInput): FormData {
  const body = new FormData();
  body.append('file', normalizedEvidenceFile(input.file));
  body.append('title', input.title.trim());
  body.append('evidence_type', input.evidence_type);
  if (input.description?.trim()) body.append('description', input.description.trim());
  if (input.valid_from) body.append('valid_from', input.valid_from);
  if (input.valid_until) body.append('valid_until', input.valid_until);
  if (input.metadata && Object.keys(input.metadata).length > 0) {
    body.append('metadata', JSON.stringify(input.metadata));
  }
  return body;
}

export function versionReasonError(value: string): string | null {
  const trimmed = value.trim();
  if (
    trimmed.length < EVIDENCE_VERSION_REASON_MIN_LENGTH ||
    trimmed.length > EVIDENCE_VERSION_REASON_MAX_LENGTH
  ) {
    return `Version reason must contain ${EVIDENCE_VERSION_REASON_MIN_LENGTH}-${EVIDENCE_VERSION_REASON_MAX_LENGTH} characters.`;
  }
  if (/\r|\n/.test(value)) return 'Version reason must be a single line.';
  return null;
}

export function serializeEvidenceSupersede(input: ControlEvidenceSupersedeInput): FormData {
  const reasonError = versionReasonError(input.version_reason);
  if (reasonError) throw new Error(reasonError);
  const body = serializeEvidenceUpload(input);
  body.append('version_reason', input.version_reason.trim());
  return body;
}

export type EvidenceFreshness = 'scheduled' | 'current' | 'expiring' | 'expired' | 'superseded';

export function evidenceFreshness(
  evidence: Pick<
    ControlEvidence,
    'expires_at' | 'is_current' | 'lifecycle_status' | 'review_status' | 'valid_from' | 'valid_until'
  >,
  now = new Date(),
): EvidenceFreshness {
  const timestamp = now.getTime();
  const validFrom = evidence.valid_from ? Date.parse(evidence.valid_from) : Number.NaN;
  const expiry = evidence.expires_at ?? evidence.valid_until;
  const validUntil = expiry ? Date.parse(expiry) : Number.NaN;
  if (
    evidence.lifecycle_status === 'expired' ||
    evidence.review_status === 'expired' ||
    (Number.isFinite(validUntil) && validUntil < timestamp)
  ) {
    return 'expired';
  }
  if (evidence.lifecycle_status === 'superseded' || !evidence.is_current) return 'superseded';
  if (Number.isFinite(validFrom) && validFrom > timestamp) return 'scheduled';
  if (
    Number.isFinite(validUntil) &&
    validUntil <= timestamp + EVIDENCE_EXPIRING_SOON_DAYS * 24 * 60 * 60 * 1_000
  ) {
    return 'expiring';
  }
  return 'current';
}

export function evidenceStatusLabel(status: string): string {
  return status.replace(/_/g, ' ').replace(/^\w/, (letter) => letter.toUpperCase());
}

export function safeEvidenceFilename(value: string | undefined): string {
  const cleaned = (value ?? 'evidence-download')
    .normalize('NFKC')
    .replace(/[\\/\u0000-\u001f\u007f\u202a-\u202e\u2066-\u2069]/g, '_')
    .replace(/^\.+/, '')
    .trim()
    .slice(0, 255);
  return cleaned || 'evidence-download';
}

export function evidenceDownloadHref(controlId: string, evidenceId: string): string {
  return `/api/bff${CONTROL_EVIDENCE_ROUTES.download(controlId, evidenceId)}`;
}

export function hasControlPermission(
  permissions: PermissionMap | undefined,
  user: User | null | undefined,
  action: 'read' | 'update' | 'export' | 'approve',
): boolean {
  return Boolean(user?.is_super_admin || permissions?.controls?.includes(action));
}

function errorDetail(error: unknown): Record<string, unknown> {
  if (!error || typeof error !== 'object') return {};
  const detail = (error as ApiError).detail;
  return detail && typeof detail === 'object' && !Array.isArray(detail)
    ? (detail as Record<string, unknown>)
    : {};
}

export function formatEvidenceError(
  error: unknown,
  operation: 'download' | 'history' | 'integrity' | 'list' | 'review' | 'supersede' | 'upload',
): string {
  const apiError = error as ApiError | undefined;
  const detail = errorDetail(error);
  const code = typeof detail.error_code === 'string' ? detail.error_code : '';
  if (apiError?.status === 413)
    return `The upload exceeds the ${EVIDENCE_MAX_UPLOAD_MIB} MiB limit.`;
  if (apiError?.status === 402 || code === 'ENTITLEMENT_LIMIT_EXCEEDED') {
    return 'Evidence storage quota has been reached. Ask an administrator to review the subscription capacity.';
  }
  if (code === 'ENTITLEMENT_REQUIRED')
    return 'Evidence management is not included in this subscription.';
  if (apiError?.status === 409 && operation === 'integrity') {
    return 'Integrity verification failed. The stored object no longer matches its immutable checksum or size. Escalate this evidence immediately.';
  }
  if (apiError?.status === 409 && operation === 'supersede') {
    return 'This evidence is no longer the active version. Reload its history before creating a replacement.';
  }
  if (apiError?.status === 422 && (operation === 'upload' || operation === 'supersede')) {
    return 'The file was rejected by content validation or malware scanning. Choose a trusted supported file.';
  }
  if (apiError?.status === 503 && (operation === 'upload' || operation === 'supersede')) {
    return 'Security scanning is temporarily unavailable. The file was not accepted; retry later.';
  }
  if (apiError?.status === 503 && operation === 'download') {
    return 'Secure download is temporarily unavailable. The file was not exposed; retry later.';
  }
  if (apiError?.status === 503 && (operation === 'history' || operation === 'integrity')) {
    return 'Evidence lifecycle verification is temporarily unavailable. No integrity conclusion was made; retry later.';
  }
  const message = typeof detail.message === 'string' ? detail.message : apiError?.message;
  if (message) return message;
  return `Evidence ${operation} failed. Please try again.`;
}
