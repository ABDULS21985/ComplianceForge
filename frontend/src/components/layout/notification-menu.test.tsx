import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  normalizeNotifications,
  NotificationMenu,
} from '@/components/layout/notification-menu';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';

const notificationApi = vi.hoisted(() => ({
  list: vi.fn(),
  markAllAsRead: vi.fn(),
  markAsRead: vi.fn(),
  unreadCount: vi.fn(),
}));

vi.mock('@/lib/api', () => ({
  default: { notifications: notificationApi },
}));

function TestProviders({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  notificationApi.unreadCount.mockResolvedValue({ count: 3 });
  notificationApi.list.mockResolvedValue({
    data: [
      {
        id: 'notification-1',
        event_type: 'evidence.review_due',
        subject: 'Evidence review due',
        body: 'Access review evidence expires this week.',
        status: 'sent',
        created_at: '2026-09-14T08:00:00Z',
        read_at: null,
      },
    ],
  });
  notificationApi.markAsRead.mockResolvedValue(undefined);
  notificationApi.markAllAsRead.mockResolvedValue(undefined);
});

describe('notification menu', () => {
  it('announces only the unread count returned by the API', async () => {
    render(<NotificationMenu />, { wrapper: TestProviders });

    expect(
      await screen.findByRole('button', {
        name: 'Notifications, 3 unread',
      })
    ).toBeVisible();
    expect(screen.getByText('3')).toHaveAttribute('aria-hidden', 'true');
  });

  it('normalizes the notification list envelope and preserves unread state', () => {
    const [notification] = normalizeNotifications({
      data: [
        {
          id: 'notification-1',
          subject: 'Evidence review due',
          body: 'Access review evidence expires this week.',
          event_type: 'evidence.review_due',
          created_at: '2026-09-14T08:00:00Z',
          read_at: null,
        },
      ],
    });

    expect(notification).toMatchObject({
      id: 'notification-1',
      subject: 'Evidence review due',
      read_at: null,
    });
  });
});
