'use client';

import * as React from 'react';
import { CheckCircle2, Loader2, RefreshCw, UserRound, X } from 'lucide-react';
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import { directoryKeys, directoryUserInitials, directoryUserName } from '@/lib/directory';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { DirectoryUser } from '@/types/directory';
import type { DirectoryUserStatus } from '@/types/directory';
import { useQuery } from '@tanstack/react-query';

const MINIMUM_SEARCH_LENGTH = 2;
const SEARCH_DEBOUNCE_MS = 250;
const SEARCH_PAGE_SIZE = 20;

export function DirectoryUserPicker({
  autoFocus = false,
  description = 'Only active users are returned. Use the arrow keys to move through results and Enter to select.',
  disabled = false,
  onChange,
  searchLabel = 'Search active users',
  selectedLabel = 'Selected user',
  status = 'active',
  value,
}: {
  autoFocus?: boolean;
  description?: string;
  disabled?: boolean;
  onChange: (user: DirectoryUser | null) => void;
  searchLabel?: string;
  selectedLabel?: string;
  status?: DirectoryUserStatus;
  value: DirectoryUser | null;
}) {
  const helpId = React.useId();
  const [search, setSearch] = React.useState('');
  const [debouncedSearch, setDebouncedSearch] = React.useState('');

  React.useEffect(() => {
    const normalized = search.trim();
    const timer = window.setTimeout(() => setDebouncedSearch(normalized), SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [search]);

  const query = useQuery({
    queryKey: directoryKeys.userSearch(debouncedSearch, status),
    queryFn: ({ signal }) =>
      api.directory.listUsers(
        {
          search: debouncedSearch,
          status,
          sort_by: 'name',
          sort_dir: 'asc',
          page: 1,
          page_size: SEARCH_PAGE_SIZE,
        },
        signal,
      ),
    enabled: !disabled && !value && debouncedSearch.length >= MINIMUM_SEARCH_LENGTH,
    retry: false,
    staleTime: 30_000,
  });

  function selectUser(user: DirectoryUser) {
    setSearch('');
    setDebouncedSearch('');
    onChange(user);
  }

  function changeUser() {
    setSearch('');
    setDebouncedSearch('');
    onChange(null);
  }

  const userStatusLabel = status.replace(/_/g, ' ');
  const userStatusSentence = `${userStatusLabel.charAt(0).toUpperCase()}${userStatusLabel.slice(1)}`;

  if (value) {
    const name = directoryUserName(value);
    return (
      <div aria-live="polite" aria-label={selectedLabel} className="rounded-md border p-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <span
              aria-hidden="true"
              className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-primary"
            >
              {directoryUserInitials(value)}
            </span>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <p className="truncate font-medium">{name}</p>
                <Badge variant="outline" className="gap-1">
                  <CheckCircle2 aria-hidden="true" className="h-3 w-3" />
                  {userStatusLabel}
                </Badge>
              </div>
              <p className="truncate text-sm text-muted-foreground">{value.email}</p>
              {(value.job_title || value.department) && (
                <p className="truncate text-xs text-muted-foreground">
                  {[value.job_title, value.department].filter(Boolean).join(' · ')}
                </p>
              )}
            </div>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={changeUser}
            aria-label={`Change selected user ${name}`}
          >
            <X aria-hidden="true" className="mr-1 h-4 w-4" />
            Change
          </Button>
        </div>
      </div>
    );
  }

  const normalizedSearch = search.trim();
  const isDebouncing = normalizedSearch !== debouncedSearch;
  const users = !isDebouncing ? (query.data?.data ?? []) : [];
  return (
    <div className="space-y-2">
      <Command shouldFilter={false} loop className="rounded-md border" label={searchLabel}>
        <CommandInput
          aria-describedby={helpId}
          aria-label={searchLabel}
          autoFocus={autoFocus}
          disabled={disabled}
          placeholder="Search by name or email…"
          value={search}
          onValueChange={setSearch}
        />
        <CommandList className="min-h-24">
          {normalizedSearch.length < MINIMUM_SEARCH_LENGTH ? (
            <PickerStatus>
              Enter at least {MINIMUM_SEARCH_LENGTH} characters to search {userStatusLabel} users.
            </PickerStatus>
          ) : isDebouncing || query.isFetching ? (
            <PickerStatus>
              <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
              Searching {userStatusLabel} users…
            </PickerStatus>
          ) : query.isError ? (
            <div role="alert" className="space-y-2 px-4 py-4 text-sm text-destructive">
              <p>{userStatusSentence} users could not be loaded.</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void query.refetch()}
              >
                <RefreshCw aria-hidden="true" className="mr-1 h-4 w-4" />
                Try again
              </Button>
            </div>
          ) : users.length === 0 ? (
            <PickerStatus>No {userStatusLabel} users matched “{debouncedSearch}”.</PickerStatus>
          ) : (
            <CommandGroup
              heading={`${query.data?.pagination.total_items ?? users.length} ${userStatusLabel} user${(query.data?.pagination.total_items ?? users.length) === 1 ? '' : 's'} found`}
            >
              {users.map((user) => {
                const name = directoryUserName(user);
                return (
                  <CommandItem
                    key={user.id}
                    aria-label={`Select ${name}, ${user.email}`}
                    value={user.id}
                    onSelect={() => selectUser(user)}
                  >
                    <UserRound
                      aria-hidden="true"
                      className="mr-3 h-4 w-4 shrink-0 text-muted-foreground"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{name}</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {user.email}
                        {user.department ? ` · ${user.department}` : ''}
                      </span>
                    </span>
                  </CommandItem>
                );
              })}
            </CommandGroup>
          )}
        </CommandList>
      </Command>
      <p id={helpId} className="text-xs text-muted-foreground">
        {description}
      </p>
      {(query.data?.pagination.total_items ?? 0) > SEARCH_PAGE_SIZE && !isDebouncing && (
        <p className="text-xs text-muted-foreground">
          Showing the first {SEARCH_PAGE_SIZE} matches. Refine the search to find another user.
        </p>
      )}
    </div>
  );
}

function PickerStatus({ children }: { children: React.ReactNode }) {
  return (
    <div
      role="status"
      className="flex items-center justify-center gap-2 px-4 py-6 text-center text-sm text-muted-foreground"
    >
      {children}
    </div>
  );
}
