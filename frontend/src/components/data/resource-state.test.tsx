import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import {
  ResourceBoundary,
  ResourceState,
  StaleDataNotice,
} from '@/components/data/resource-state';
import { axe } from 'jest-axe';
import userEvent from '@testing-library/user-event';

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', {
    configurable: true,
    value,
  });
}

beforeEach(() => setOnline(true));

describe('resource state system', () => {
  it('exposes a named busy state while keeping skeletons decorative', async () => {
    const { container } = render(
      <ResourceState
        kind="loading"
        loadingLayout="cards"
        title="Loading policy register"
      />,
    );

    const status = screen.getByRole('status', {
      name: 'Loading policy register',
    });
    expect(status).toHaveAttribute('aria-busy', 'true');
    expect(status.querySelectorAll('[aria-hidden="true"]')).not.toHaveLength(0);
    expect((await axe(container)).violations).toEqual([]);
  });

  it('renders a safe actionable alert and invokes retry once', async () => {
    const retry = vi.fn();
    const user = userEvent.setup();
    const { container } = render(
      <ResourceState
        kind="error"
        title="Risks could not be loaded"
        description="The service did not return the risk list."
        onRetry={retry}
      />,
    );

    expect(screen.getByRole('alert')).toHaveAccessibleName(
      'Risks could not be loaded',
    );
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(retry).toHaveBeenCalledOnce();
    expect((await axe(container)).violations).toEqual([]);
  });

  it('distinguishes an offline transport failure from a service error', () => {
    setOnline(false);
    render(
      <ResourceBoundary isError onRetry={vi.fn()}>
        <p>Protected content</p>
      </ResourceBoundary>,
    );

    expect(screen.getByRole('alert')).toHaveAccessibleName('You are offline');
    expect(screen.queryByText('Protected content')).not.toBeInTheDocument();
  });

  it('uses explicit icon and text for forbidden and empty states', () => {
    const { rerender } = render(
      <ResourceState
        kind="forbidden"
        title="Audit management unavailable"
      />,
    );
    expect(screen.getByRole('status')).toHaveAccessibleName(
      'Audit management unavailable',
    );
    expect(screen.getByRole('status').querySelector('svg')).toHaveAttribute(
      'aria-hidden',
      'true',
    );

    rerender(<ResourceState kind="empty" title="No vendors found" />);
    expect(screen.getByRole('status')).toHaveAccessibleName('No vendors found');
  });

  it('announces stale cached data with a machine-readable timestamp', async () => {
    const refresh = vi.fn();
    const user = userEvent.setup();
    const timestamp = Date.parse('2026-09-14T09:30:00.000Z');
    render(
      <StaleDataNotice lastUpdatedAt={timestamp} onRefresh={refresh} />,
    );

    expect(screen.getByRole('status')).toHaveTextContent(
      'Showing saved data while the service reconnects',
    );
    expect(screen.getByText(/Last updated/).querySelector('time')).toHaveAttribute(
      'datetime',
      '2026-09-14T09:30:00.000Z',
    );
    await user.click(screen.getByRole('button', { name: 'Refresh now' }));
    expect(refresh).toHaveBeenCalledOnce();
  });
});
