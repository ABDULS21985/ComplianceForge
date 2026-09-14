import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';

import { NotificationPreferencesForm } from '@/components/notifications/notification-preferences-form';

const notificationApi = vi.hoisted(() => ({
  getPreferences: vi.fn(),
  updatePreferences: vi.fn(),
}));

vi.mock('@/lib/api', () => ({
  default: { notifications: notificationApi },
}));

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  notificationApi.getPreferences.mockResolvedValue({
    data: {
      user_id: 'user-1',
      organization_id: 'org-1',
      event_type: '*',
      email_enabled: true,
      in_app_enabled: true,
      slack_enabled: false,
      digest_frequency: 'daily',
      quiet_hours_start: '22:00',
      quiet_hours_end: '07:00',
      quiet_hours_timezone: 'Africa/Lagos',
    },
  });
  notificationApi.updatePreferences.mockImplementation(async (input) => ({
    data: {
      user_id: 'user-1', organization_id: 'org-1', event_type: '*',
      ...input,
    },
  }));
});

describe('notification preferences form', () => {
  it('reads the backend data envelope and saves only the mutable contract', async () => {
    const user = userEvent.setup();
    render(<NotificationPreferencesForm />, { wrapper: Providers });

    const email = await screen.findByRole('switch', { name: 'Email notifications' });
    expect(email).toBeChecked();
    expect(screen.getByLabelText('Starts')).toHaveValue('22:00');
    expect(screen.getByLabelText('Timezone')).toHaveValue('Africa/Lagos');

    await user.click(email);
    await user.click(screen.getByRole('button', { name: 'Save preferences' }));

    await waitFor(() => expect(notificationApi.updatePreferences).toHaveBeenCalledWith({
      email_enabled: false,
      in_app_enabled: true,
      slack_enabled: false,
      digest_frequency: 'daily',
      quiet_hours_start: '22:00',
      quiet_hours_end: '07:00',
      quiet_hours_timezone: 'Africa/Lagos',
    }));
  });

  it('rejects an incomplete quiet-hours pair before calling the API', async () => {
    notificationApi.getPreferences.mockResolvedValueOnce({
      data: {
        user_id: 'user-1', organization_id: 'org-1', event_type: '*',
        email_enabled: true, in_app_enabled: true, slack_enabled: false,
        digest_frequency: 'immediate', quiet_hours_start: null,
        quiet_hours_end: null, quiet_hours_timezone: null,
      },
    });
    const user = userEvent.setup();
    render(<NotificationPreferencesForm />, { wrapper: Providers });
    const start = await screen.findByLabelText('Starts');
    await user.type(start, '22:00');
    await user.click(screen.getByRole('button', { name: 'Save preferences' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('both a quiet-hours start and end');
    expect(notificationApi.updatePreferences).not.toHaveBeenCalled();
  });
});
