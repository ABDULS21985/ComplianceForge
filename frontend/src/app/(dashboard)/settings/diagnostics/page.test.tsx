import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import DiagnosticsPage from './page';
import type { DiagnosticsSnapshot } from '@/types/diagnostics';

const { permissionState, snapshotRequest } = vi.hoisted(() => ({
  permissionState: {
    canConfigure: true,
    canRead: true,
    isError: false,
    isLoading: false,
    retry: vi.fn(),
  },
  snapshotRequest: vi.fn(),
}));

vi.mock('@/hooks/use-capability-permissions', () => ({
  useCapabilityPermissions: () => permissionState,
}));
vi.mock('@/lib/api', () => ({
  default: { diagnostics: { snapshot: snapshotRequest } },
}));

function diagnosticSnapshot(generatedAt = new Date().toISOString()): DiagnosticsSnapshot {
  return {
    organization_id: '7a2423cd-bdeb-472f-a60a-1b6cfec87aa8',
    generated_at: generatedAt,
    overall_status: 'warning',
    dependencies: [
      {
        key: 'database',
        name: 'Primary database',
        status: 'healthy',
        critical: true,
        latency_ms: 8,
        checked_at: generatedAt,
        message: 'Available.',
      },
      {
        key: 'object_storage',
        name: 'Object storage',
        status: 'warning',
        critical: false,
        latency_ms: 1_250,
        checked_at: generatedAt,
        message: 'Optional object operations are delayed.',
      },
    ],
    migration: {
      current_version: 52,
      supported_version: 53,
      pending_count: 1,
      dirty: false,
      status: 'warning',
    },
    queue: {
      pending: 4,
      leased: 2,
      dead: 0,
      expired_leases: 0,
      oldest_ready_age_seconds: 35,
      inbox_processing: 1,
      inbox_expired_leases: 0,
      status: 'healthy',
    },
    notifications: {
      due: 2,
      expired_leases: 0,
      terminal_failures: 0,
      oldest_due_age_seconds: 40,
      status: 'healthy',
    },
    connectors: {
      total: 3,
      healthy: 2,
      degraded: 1,
      unhealthy: 0,
      unknown: 0,
      recent_failed_syncs: 1,
      status: 'warning',
    },
    configuration: [
      {
        key: 'public_origin',
        category: 'security',
        status: 'warning',
        message: 'The public-origin safety check needs attention.',
        remediation: 'Ask a platform operator to review the documented public origin setting.',
      },
    ],
  };
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { gcTime: 0, retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <DiagnosticsPage />
    </QueryClientProvider>,
  );
}

describe('DiagnosticsPage', () => {
  beforeEach(() => {
    snapshotRequest.mockReset();
    permissionState.retry.mockReset();
    Object.assign(permissionState, {
      canConfigure: true,
      canRead: true,
      isError: false,
      isLoading: false,
    });
  });

  it('fails closed before requesting diagnostics without settings read permission', () => {
    permissionState.canRead = false;
    renderPage();

    expect(
      screen.getByRole('heading', { name: 'Administrator diagnostics unavailable' }),
    ).toBeInTheDocument();
    expect(snapshotRequest).not.toHaveBeenCalled();
  });

  it('fails closed and offers a retry when permission verification fails', () => {
    permissionState.canRead = false;
    permissionState.isError = true;
    renderPage();

    expect(
      screen.getByRole('heading', { name: 'Settings access could not be verified' }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(permissionState.retry).toHaveBeenCalledOnce();
    expect(snapshotRequest).not.toHaveBeenCalled();
  });

  it('renders non-colour status, scope, redaction, remediation, and manual refresh cues', async () => {
    snapshotRequest.mockResolvedValue({ data: diagnosticSnapshot() });
    renderPage();

    expect(
      await screen.findByRole('heading', { name: 'Administrator diagnostics' }),
    ).toBeInTheDocument();
    expect(screen.getByText(/counts are scoped only to your organization/i)).toBeInTheDocument();
    expect(screen.getByText(/raw dependency errors, endpoints, credentials/i)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Database migrations' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Event inbox' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Runtime dependencies' })).toBeInTheDocument();
    expect(screen.getAllByText('Warning').length).toBeGreaterThan(0);
    expect(screen.getByText(/Apply the 1 pending migration/i)).toBeInTheDocument();
    expect(screen.getByText(/Configuration values and secrets are never returned/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh snapshot' }));
    await waitFor(() => expect(snapshotRequest).toHaveBeenCalledTimes(2));
  });

  it('warns when cached operational data is stale', async () => {
    snapshotRequest.mockResolvedValue({ data: diagnosticSnapshot('2026-01-01T00:00:00Z') });
    renderPage();

    expect(await screen.findByText('Snapshot may be stale')).toBeInTheDocument();
    expect(screen.getByText(/Refresh before making an operational decision/i)).toBeInTheDocument();
  });

  it('shows a retryable service-safe error when no snapshot is available', async () => {
    snapshotRequest.mockRejectedValue({
      status: 503,
      message: 'Diagnostics are temporarily unavailable',
    });
    renderPage();

    expect(
      await screen.findByRole('heading', { name: 'Diagnostics temporarily unavailable' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Diagnostics are temporarily unavailable')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(snapshotRequest).toHaveBeenCalledTimes(2));
  });

  it('cancels the in-flight snapshot when the page stops observing it', async () => {
    let requestSignal: AbortSignal | undefined;
    snapshotRequest.mockImplementation((signal?: AbortSignal) => {
      requestSignal = signal;
      return new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () => {
          reject(new DOMException('Request cancelled', 'AbortError'));
        });
      });
    });
    const view = renderPage();
    expect(
      screen.getByRole('status', { name: 'Loading administrator diagnostics' }),
    ).toBeInTheDocument();
    await waitFor(() => expect(requestSignal).toBeDefined());

    view.unmount();
    expect(requestSignal?.aborted).toBe(true);
  });
});
