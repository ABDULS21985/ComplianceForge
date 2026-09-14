import type { ApiError } from '@/lib/api';

export const PRODUCT_ACCESS_EVENT = 'complianceforge:product-access-error';

export type ProductAccessErrorCode =
  | 'ENTITLEMENT_REQUIRED'
  | 'ENTITLEMENT_LIMIT_EXCEEDED'
  | 'FEATURE_DISABLED'
  | 'FEATURE_EVALUATION_UNAVAILABLE'
  | 'ENTITLEMENT_EVALUATION_UNAVAILABLE';

export interface ProductAccessFailure {
  code: ProductAccessErrorCode;
  message: string;
  path: string;
  requestId?: string;
  status: number;
}

const PRODUCT_ACCESS_CODES = new Set<ProductAccessErrorCode>([
  'ENTITLEMENT_REQUIRED',
  'ENTITLEMENT_LIMIT_EXCEEDED',
  'FEATURE_DISABLED',
  'FEATURE_EVALUATION_UNAVAILABLE',
  'ENTITLEMENT_EVALUATION_UNAVAILABLE',
]);

export function productAccessFailure(error: ApiError, path: string): ProductAccessFailure | null {
  const detail = error.detail && typeof error.detail === 'object'
    ? error.detail as Record<string, unknown>
    : {};
  const code = detail.error_code;
  if (typeof code !== 'string' || !PRODUCT_ACCESS_CODES.has(code as ProductAccessErrorCode)) return null;
  const failure: ProductAccessFailure = {
    code: code as ProductAccessErrorCode,
    message: typeof detail.message === 'string' && detail.message ? detail.message : error.message,
    path,
    status: error.status,
  };
  if (typeof detail.request_id === 'string' && detail.request_id) failure.requestId = detail.request_id;
  return failure;
}

export function publishProductAccessFailure(failure: ProductAccessFailure): void {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new CustomEvent<ProductAccessFailure>(PRODUCT_ACCESS_EVENT, { detail: failure }));
}

export function isEntitlementFailure(failure: ProductAccessFailure): boolean {
  return failure.code === 'ENTITLEMENT_REQUIRED' || failure.code === 'ENTITLEMENT_LIMIT_EXCEEDED';
}

export function isEvaluationUnavailable(failure: ProductAccessFailure): boolean {
  return failure.code === 'FEATURE_EVALUATION_UNAVAILABLE' || failure.code === 'ENTITLEMENT_EVALUATION_UNAVAILABLE';
}
