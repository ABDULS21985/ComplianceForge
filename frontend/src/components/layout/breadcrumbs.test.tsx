import {
  Breadcrumbs,
  buildBreadcrumbItems,
} from '@/components/layout/breadcrumbs';
import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

const navigationState = vi.hoisted(() => ({
  pathname: '/risks/00000000-0000-4000-8000-000000000001',
}));

vi.mock('next/navigation', () => ({
  usePathname: () => navigationState.pathname,
}));

describe('breadcrumbs', () => {
  it('builds domain-aware labels for record paths', () => {
    expect(
      buildBreadcrumbItems(
        '/risks/00000000-0000-4000-8000-000000000001'
      )
    ).toEqual([
      { href: '/risks', label: 'Risk Register' },
      {
        href: '/risks/00000000-0000-4000-8000-000000000001',
        label: 'Risk details',
      },
    ]);
  });

  it('announces the current entity and keeps parent context linked', () => {
    render(
      <Breadcrumbs
        dynamicLabels={{
          '00000000-0000-4000-8000-000000000001': 'Supplier outage',
        }}
      />
    );

    expect(screen.getByRole('navigation', { name: 'Breadcrumb' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute(
      'href',
      '/dashboard'
    );
    expect(screen.getByRole('link', { name: 'Risk Register' })).toHaveAttribute(
      'href',
      '/risks'
    );
    expect(screen.getByText('Supplier outage')).toHaveAttribute(
      'aria-current',
      'page'
    );
  });
});
