// @vitest-environment jsdom

import { act, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { ProductAccessFeedback } from '@/components/layout/product-access-feedback';
import { publishProductAccessFailure } from '@/lib/product-access';
import userEvent from '@testing-library/user-event';

describe('ProductAccessFeedback', () => {
  it('offers plan guidance for entitlement failures and clears the announcement', async () => {
    const user = userEvent.setup();
    render(<ProductAccessFeedback />);

    act(() => {
      publishProductAccessFailure({
        code: 'ENTITLEMENT_LIMIT_EXCEEDED',
        message: 'The user allowance is exhausted.',
        path: '/settings/users',
        requestId: 'req-capacity',
        status: 402,
      });
    });

    expect(screen.getByRole('dialog', { name: 'Subscription capacity reached' })).toBeInTheDocument();
    expect(screen.getByText('Support reference: req-capacity')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review plan and usage' })).toHaveAttribute('href', '/settings/subscription');

    await user.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('distinguishes disabled capabilities from fail-closed evaluation outages', () => {
    const { rerender } = render(<ProductAccessFeedback />);

    act(() => {
      publishProductAccessFailure({
        code: 'FEATURE_DISABLED',
        message: 'Advanced exports are not currently available.',
        path: '/reports/export',
        status: 403,
      });
    });
    expect(screen.getByRole('dialog', { name: 'Capability disabled' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View capability status' })).toHaveAttribute('href', '/settings/capabilities');

    rerender(<ProductAccessFeedback />);
    act(() => {
      publishProductAccessFailure({
        code: 'FEATURE_EVALUATION_UNAVAILABLE',
        message: 'Evaluation service is unavailable.',
        path: '/reports/export',
        requestId: 'req-evaluation',
        status: 503,
      });
    });
    expect(screen.getByRole('dialog', { name: 'Availability check unavailable' })).toBeInTheDocument();
    expect(screen.getByText(/failed closed/i)).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Review plan and usage' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'View capability status' })).not.toBeInTheDocument();
  });
});
