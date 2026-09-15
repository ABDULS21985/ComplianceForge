import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  OfflineBanner,
  RouteFocusManager,
} from '@/components/layout/app-status';
import { render, screen } from '@testing-library/react';
import { axe } from 'jest-axe';

const navigationState = vi.hoisted(() => ({ pathname: '/dashboard' }));

vi.mock('next/navigation', () => ({
  usePathname: () => navigationState.pathname,
}));

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', {
    configurable: true,
    value,
  });
}

beforeEach(() => {
  navigationState.pathname = '/dashboard';
  setOnline(true);
});

describe('application accessibility status', () => {
  it('moves focus to the new page heading and announces route completion', () => {
    const { rerender } = render(
      <>
        <RouteFocusManager />
        <main id="main-content">
          <h1>Dashboard</h1>
        </main>
      </>,
    );
    expect(document.title).toBe('Dashboard | ComplianceForge');

    navigationState.pathname = '/risks';
    rerender(
      <>
        <RouteFocusManager />
        <main id="main-content">
          <h1>Risk register</h1>
        </main>
      </>,
    );

    expect(screen.getByRole('heading', { name: 'Risk register' })).toHaveFocus();
    expect(screen.getByTestId('route-announcer')).toHaveTextContent(
      'Risk register loaded',
    );
    expect(document.title).toBe('Risk register | ComplianceForge');
  });

  it('announces offline mode with text and an icon, without colour dependence', async () => {
    setOnline(false);
    const { container } = render(<OfflineBanner />);

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('You are offline');
    expect(alert.querySelector('svg')).toHaveAttribute('aria-hidden', 'true');
    expect((await axe(container)).violations).toEqual([]);
    setOnline(true);
  });

  it('focuses a public-shell heading even when it sits outside the main landmark', () => {
    const { rerender } = render(
      <>
        <RouteFocusManager />
        <header><h1 data-route-heading>Initial portal</h1></header>
        <main>Initial content</main>
      </>,
    );

    navigationState.pathname = '/vendor-portal';
    rerender(
      <>
        <RouteFocusManager />
        <header><h1 data-route-heading>Vendor questionnaire</h1></header>
        <main>Questionnaire content</main>
      </>,
    );

    expect(
      screen.getByRole('heading', { name: 'Vendor questionnaire' }),
    ).toHaveFocus();
    expect(screen.getByTestId('route-announcer')).toHaveTextContent(
      'Vendor questionnaire loaded',
    );
  });

  it('falls back to the main landmark when a transient state has no heading', () => {
    const { rerender } = render(
      <>
        <RouteFocusManager />
        <main id="main-content">Initial state</main>
      </>,
    );

    navigationState.pathname = '/reports';
    rerender(
      <>
        <RouteFocusManager />
        <main id="main-content">Loading reports</main>
      </>,
    );

    expect(screen.getByRole('main')).toHaveFocus();
    expect(screen.getByRole('main')).toHaveAttribute('tabindex', '-1');
    expect(screen.getByTestId('route-announcer')).toHaveTextContent(
      'Reports loaded',
    );
  });
});
