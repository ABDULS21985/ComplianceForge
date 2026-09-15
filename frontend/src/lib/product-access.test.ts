// @vitest-environment jsdom

import { describe, expect, it, vi } from 'vitest';
import {
  isEntitlementFailure,
  isEvaluationUnavailable,
  PRODUCT_ACCESS_EVENT,
  productAccessFailure,
  publishProductAccessFailure,
} from '@/lib/product-access';

describe('global product access failures', () => {
  it('classifies entitlement, disabled, and unavailable codes without guessing from status', () => {
    const entitlement = productAccessFailure({ status: 402, message: 'Upgrade', detail: { error_code: 'ENTITLEMENT_REQUIRED', request_id: 'req-1' } }, '/reports');
    const disabled = productAccessFailure({ status: 403, message: 'Disabled', detail: { error_code: 'FEATURE_DISABLED' } }, '/reports');
    const unavailable = productAccessFailure({ status: 503, message: 'Unavailable', detail: { error_code: 'FEATURE_EVALUATION_UNAVAILABLE' } }, '/reports');
    expect(entitlement).toMatchObject({ code: 'ENTITLEMENT_REQUIRED', requestId: 'req-1', path: '/reports' });
    expect(entitlement && isEntitlementFailure(entitlement)).toBe(true);
    expect(disabled && isEntitlementFailure(disabled)).toBe(false);
    expect(unavailable && isEvaluationUnavailable(unavailable)).toBe(true);
    expect(productAccessFailure({ status: 403, message: 'Forbidden', detail: { error_code: 'permission_denied' } }, '/reports')).toBeNull();
  });

  it('publishes a typed browser event for the global feedback surface', () => {
    const listener = vi.fn();
    window.addEventListener(PRODUCT_ACCESS_EVENT, listener);
    publishProductAccessFailure({ code: 'FEATURE_DISABLED', message: 'Disabled', path: '/sso', status: 403 });
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener(PRODUCT_ACCESS_EVENT, listener);
  });
});
