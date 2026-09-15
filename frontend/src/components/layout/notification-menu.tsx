'use client';

import { Bell, CheckCheck, Loader2 } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { formatApiError, notificationPollInterval } from '@/lib/enterprise-settings';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { formatDistanceToNow } from 'date-fns';
import Link from 'next/link';
import type { NotificationRecord } from '@/types/enterprise-settings';
import { toast } from 'sonner';
import { useState } from 'react';

export function normalizeNotifications(value: unknown): Array<Pick<NotificationRecord, 'body' | 'created_at' | 'event_type' | 'id' | 'read_at' | 'subject'>> {
  const response =
    value && typeof value === 'object'
      ? (value as Record<string, unknown>)
      : {};
  const rawItems = Array.isArray(value)
    ? value
    : Array.isArray(response.data)
      ? response.data
      : [];

  return rawItems
    .filter(
      (item): item is Record<string, unknown> =>
        Boolean(item) && typeof item === 'object'
    )
    .map((item) => ({
      body: typeof item.body === 'string' ? item.body : '',
      created_at:
        typeof item.created_at === 'string' ? item.created_at : '',
      event_type:
        typeof item.event_type === 'string' ? item.event_type : 'notification',
      id: String(item.id ?? ''),
      read_at: typeof item.read_at === 'string' ? item.read_at : null,
      subject:
        typeof item.subject === 'string' ? item.subject : 'Notification',
    }))
    .filter((notification) => notification.id);
}

function getUnreadCount(value: unknown): number {
  if (!value || typeof value !== 'object') return 0;
  const count = Number((value as Record<string, unknown>).count ?? 0);
  return Number.isFinite(count) && count > 0 ? count : 0;
}

export function NotificationMenu() {
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const unreadQuery = useQuery({
    queryKey: ['notifications', 'unread-count'],
    queryFn: () => api.notifications.unreadCount(),
    refetchInterval: (query) =>
      notificationPollInterval(
        query.state.fetchFailureCount,
        typeof document !== 'undefined' && document.visibilityState === 'hidden',
        typeof navigator === 'undefined' || navigator.onLine
      ),
    refetchIntervalInBackground: false,
    staleTime: 30_000,
    retry: 2,
  });
  const notificationsQuery = useQuery({
    queryKey: ['notifications', 'list', 'topbar'],
    queryFn: () => api.notifications.list({ page: 1, page_size: 6 }),
    enabled: open,
    staleTime: 15_000,
    retry: 1,
  });
  const notifications = normalizeNotifications(notificationsQuery.data);
  const unreadCount = getUnreadCount(unreadQuery.data);

  const refreshNotifications = () =>
    queryClient.invalidateQueries({ queryKey: ['notifications'] });
  const markReadMutation = useMutation({
    mutationFn: (id: string) => api.notifications.markAsRead(id),
    onSuccess: refreshNotifications,
    onError: (error) => toast.error(formatApiError(error, 'The notification could not be marked as read.')),
  });
  const markAllMutation = useMutation({
    mutationFn: () => api.notifications.markAllAsRead(),
    onSuccess: refreshNotifications,
    onError: (error) => toast.error(formatApiError(error, 'Notifications could not be marked as read.')),
  });

  const triggerLabel = unreadQuery.isSuccess
    ? unreadCount > 0
      ? `Notifications, ${unreadCount} unread`
      : 'Notifications, none unread'
    : 'Notifications';

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="relative"
          aria-label={triggerLabel}
        >
          <Bell aria-hidden="true" className="h-5 w-5" />
          {unreadQuery.isSuccess && unreadCount > 0 && (
            <Badge
              variant="destructive"
              aria-hidden="true"
              className="absolute -right-1 -top-1 h-5 min-w-5 px-1 text-[10px]"
            >
              {unreadCount > 99 ? '99+' : unreadCount}
            </Badge>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[min(92vw,24rem)] p-0">
        <div className="flex items-center justify-between gap-3 px-3 py-2">
          <DropdownMenuLabel className="p-0">Notifications</DropdownMenuLabel>
          {unreadQuery.isSuccess && unreadCount > 0 && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-8 gap-1.5 text-xs"
              disabled={markAllMutation.isPending}
              onClick={() => markAllMutation.mutate()}
            >
              {markAllMutation.isPending ? (
                <Loader2 aria-hidden="true" className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCheck aria-hidden="true" className="h-3.5 w-3.5" />
              )}
              Mark all read
            </Button>
          )}
        </div>
        <DropdownMenuSeparator className="m-0" />

        {notificationsQuery.isLoading && (
          <div
            role="status"
            className="flex items-center justify-center gap-2 px-4 py-8 text-sm text-muted-foreground"
          >
            <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
            Loading notifications…
          </div>
        )}

        {notificationsQuery.isError && (
          <p role="alert" className="px-4 py-6 text-center text-sm text-muted-foreground">
            Notifications could not be loaded.
          </p>
        )}

        {notificationsQuery.isSuccess && notifications.length === 0 && (
          <p className="px-4 py-8 text-center text-sm text-muted-foreground">
            You have no notifications.
          </p>
        )}

        {notifications.length > 0 && (
          <div className="max-h-[min(60vh,26rem)] overflow-y-auto p-1">
            {notifications.map((notification) => {
              const unread = !notification.read_at;
              return (
                <DropdownMenuItem
                  key={notification.id}
                  onSelect={() => {
                    if (unread) markReadMutation.mutate(notification.id);
                  }}
                  className="items-start gap-3 px-3 py-3"
                >
                  <span
                    aria-hidden="true"
                    className={cn(
                      'mt-1.5 h-2 w-2 shrink-0 rounded-full',
                      unread ? 'bg-primary' : 'bg-transparent'
                    )}
                  />
                  <span className="min-w-0 flex-1">
                    <span className={cn('block text-sm', unread && 'font-semibold')}>
                      {notification.subject}
                      {unread && <span className="sr-only">, unread</span>}
                    </span>
                    {notification.body && (
                      <span className="mt-0.5 block line-clamp-2 text-xs text-muted-foreground">
                        {notification.body}
                      </span>
                    )}
                    {notification.created_at && (
                      <span className="mt-1 block text-[11px] text-muted-foreground">
                        {formatDistanceToNow(new Date(notification.created_at), {
                          addSuffix: true,
                        })}
                      </span>
                    )}
                  </span>
                </DropdownMenuItem>
              );
            })}
          </div>
        )}

        <DropdownMenuSeparator className="m-0" />
        <div className="grid grid-cols-2 p-1">
          <DropdownMenuItem asChild className="justify-center">
            <Link href="/notifications">View all</Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild className="justify-center">
            <Link href="/settings/notifications">Preferences</Link>
          </DropdownMenuItem>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
