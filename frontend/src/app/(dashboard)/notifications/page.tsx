'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Bell, Check, CheckCheck, ChevronLeft, ChevronRight, Loader2, Settings } from 'lucide-react';
import { format } from 'date-fns';
import Link from 'next/link';
import { toast } from 'sonner';

import api from '@/lib/api';
import { formatApiError, notificationPollInterval } from '@/lib/enterprise-settings';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

const PAGE_SIZE = 20;

export default function NotificationsPage() {
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const notificationsQuery = useQuery({
    queryKey: ['notifications', 'list', page, PAGE_SIZE],
    queryFn: () => api.notifications.list({ page, page_size: PAGE_SIZE }),
    staleTime: 15_000,
    refetchInterval: (query) =>
      notificationPollInterval(
        query.state.fetchFailureCount,
        typeof document !== 'undefined' && document.visibilityState === 'hidden',
        typeof navigator === 'undefined' || navigator.onLine
      ),
    refetchIntervalInBackground: false,
  });
  const unreadQuery = useQuery({
    queryKey: ['notifications', 'unread-count'],
    queryFn: () => api.notifications.unreadCount(),
    staleTime: 15_000,
  });

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['notifications'] });
  const markRead = useMutation({
    mutationFn: (id: string) => api.notifications.markAsRead(id),
    onSuccess: refresh,
    onError: (error) => toast.error(formatApiError(error, 'The notification could not be marked as read.')),
  });
  const markAll = useMutation({
    mutationFn: () => api.notifications.markAllAsRead(),
    onSuccess: (response) => {
      void refresh();
      toast.success(response.count === 1 ? '1 notification marked as read' : `${response.count} notifications marked as read`);
    },
    onError: (error) => toast.error(formatApiError(error, 'Notifications could not be marked as read.')),
  });

  const notifications = notificationsQuery.data?.data ?? [];
  const pagination = notificationsQuery.data?.pagination;
  const unreadCount = unreadQuery.data?.count ?? 0;

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-bold tracking-tight">Notifications</h1>
            {unreadQuery.isSuccess && unreadCount > 0 && (
              <Badge variant="secondary">{unreadCount} unread</Badge>
            )}
          </div>
          <p className="mt-1 text-muted-foreground">Your compliance and operational updates, newest first.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={unreadCount === 0 || markAll.isPending}
            onClick={() => markAll.mutate()}
          >
            {markAll.isPending ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" /> : <CheckCheck aria-hidden="true" className="mr-2 h-4 w-4" />}
            Mark all read
          </Button>
          <Button asChild type="button" variant="outline">
            <Link href="/settings/notifications">
              <Settings aria-hidden="true" className="mr-2 h-4 w-4" />
              Preferences
            </Link>
          </Button>
        </div>
      </div>

      {notificationsQuery.isLoading && (
        <div role="status" aria-label="Loading notifications" className="space-y-3">
          {Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-28 w-full" />)}
        </div>
      )}

      {notificationsQuery.isError && (
        <Card>
          <CardContent className="space-y-4 py-10 text-center">
            <p role="alert">{formatApiError(notificationsQuery.error, 'Notifications could not be loaded.')}</p>
            <Button type="button" variant="outline" onClick={() => void notificationsQuery.refetch()}>Try again</Button>
          </CardContent>
        </Card>
      )}

      {notificationsQuery.isSuccess && notifications.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-14 text-center">
            <Bell aria-hidden="true" className="h-9 w-9 text-muted-foreground" />
            <div>
              <p className="font-medium">No notifications</p>
              <p className="text-sm text-muted-foreground">New in-app notifications will appear here.</p>
            </div>
          </CardContent>
        </Card>
      )}

      {notifications.length > 0 && (
        <ol aria-label="Notification feed" className="space-y-3">
          {notifications.map((notification) => {
            const unread = notification.read_at === null;
            const createdAt = new Date(notification.created_at);
            return (
              <li key={notification.id}>
                <Card className={cn(unread && 'border-primary/40 bg-primary/[0.025]')}>
                  <CardContent className="flex gap-4 py-5">
                    <span aria-hidden="true" className={cn('mt-2 h-2.5 w-2.5 shrink-0 rounded-full', unread ? 'bg-primary' : 'bg-muted')} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-col justify-between gap-2 sm:flex-row sm:items-start">
                        <div>
                          <h2 className={cn('text-base', unread && 'font-semibold')}>
                            {notification.subject || 'Notification'}
                            {unread && <span className="sr-only">, unread</span>}
                          </h2>
                          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                            <Badge variant="outline" className="font-normal">{notification.event_type}</Badge>
                            {Number.isNaN(createdAt.valueOf()) ? notification.created_at : format(createdAt, 'PPp')}
                          </div>
                        </div>
                        {unread && (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            disabled={markRead.isPending && markRead.variables === notification.id}
                            onClick={() => markRead.mutate(notification.id)}
                          >
                            {markRead.isPending && markRead.variables === notification.id
                              ? <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
                              : <Check aria-hidden="true" className="mr-2 h-4 w-4" />}
                            Mark read
                          </Button>
                        )}
                      </div>
                      {notification.body && <p className="mt-3 whitespace-pre-wrap text-sm text-muted-foreground">{notification.body}</p>}
                    </div>
                  </CardContent>
                </Card>
              </li>
            );
          })}
        </ol>
      )}

      {pagination && pagination.total_pages > 1 && (
        <nav aria-label="Notification pages" className="flex items-center justify-between gap-4">
          <p className="text-sm text-muted-foreground">
            Page {pagination.page} of {pagination.total_pages} · {pagination.total_items} notifications
          </p>
          <div className="flex gap-2">
            <Button type="button" variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>
              <ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" /> Previous
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={page >= pagination.total_pages} onClick={() => setPage((current) => current + 1)}>
              Next <ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" />
            </Button>
          </div>
        </nav>
      )}
    </div>
  );
}
