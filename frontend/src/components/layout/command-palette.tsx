'use client';

import { ArrowRight, FileSearch, Loader2, Search } from 'lucide-react';
import {
  buildGlobalSearchRoute,
  getEntityRoute,
  getVisibleNavigationGroups,
  type NavigationContext,
  type NavigationItem,
  normalizeGlobalAutocompleteResults,
} from '@/lib/navigation';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog';
import { useEffect, useMemo, useState } from 'react';
import api from '@/lib/api';
import { NAVIGATION_ICONS } from '@/components/layout/navigation-icons';
import { useNavigationStore } from '@/store/navigation-store';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';

interface CommandPaletteProps {
  context: NavigationContext;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}

export function CommandPalette({
  context,
  onOpenChange,
  open,
}: CommandPaletteProps) {
  const router = useRouter();
  const recordRecent = useNavigationStore((state) => state.recordRecent);
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const groups = useMemo(() => getVisibleNavigationGroups(context), [context]);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), 200);
    return () => window.clearTimeout(timer);
  }, [query]);

  const autocompleteQuery = useQuery({
    queryKey: ['global-search', 'autocomplete', debouncedQuery],
    queryFn: () => api.search.autocomplete(debouncedQuery),
    enabled: open && debouncedQuery.length >= 2,
    staleTime: 30_000,
    retry: 1,
  });
  const records = normalizeGlobalAutocompleteResults(autocompleteQuery.data);

  const navigate = (href: string, item?: NavigationItem) => {
    if (item) recordRecent(item.id);
    setQuery('');
    setDebouncedQuery('');
    onOpenChange(false);
    router.push(href);
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setQuery('');
      setDebouncedQuery('');
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <DialogTitle className="sr-only">Search and navigate</DialogTitle>
        <DialogDescription className="sr-only">
          Search available pages and compliance records. Use the arrow keys to
          move through results and Enter to open one.
        </DialogDescription>
        <Command label="Search pages and compliance records">
          <CommandInput
            aria-label="Search pages and compliance records"
            autoFocus
            placeholder="Search pages, risks, policies, vendors…"
            value={query}
            onValueChange={setQuery}
          />
          <CommandList>
            <CommandEmpty>No matching page or record found.</CommandEmpty>

            {groups.map((group) => (
              <CommandGroup key={group.id} heading={group.label}>
                {group.items.map((item) => {
                  const Icon = NAVIGATION_ICONS[item.icon];
                  return (
                    <CommandItem
                      key={item.id}
                      value={`${item.label} ${item.href} ${item.keywords.join(' ')}`}
                      onSelect={() => navigate(item.href, item)}
                    >
                      <Icon aria-hidden="true" className="mr-3 h-4 w-4" />
                      <span className="flex-1">{item.label}</span>
                      <span className="hidden text-xs text-muted-foreground sm:inline">
                        {item.href}
                      </span>
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            ))}

            {debouncedQuery.length >= 2 && <CommandSeparator />}

            {autocompleteQuery.isLoading && (
              <div
                role="status"
                className="flex items-center gap-2 px-4 py-3 text-sm text-muted-foreground"
              >
                <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
                Searching compliance records…
              </div>
            )}

            {autocompleteQuery.isError && (
              <p role="status" className="px-4 py-3 text-sm text-muted-foreground">
                Record search is unavailable. Page navigation still works.
              </p>
            )}

            {records.length > 0 && (
              <CommandGroup heading="Compliance records">
                {records.map((record) => (
                  <CommandItem
                    key={`${record.entity_type}-${record.entity_id}`}
                    value={`${record.title} ${record.subtitle ?? ''} ${record.entity_type}`}
                    onSelect={() => navigate(getEntityRoute(record))}
                  >
                    <FileSearch aria-hidden="true" className="mr-3 h-4 w-4" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">
                        {record.title}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {record.subtitle ?? record.entity_type.replaceAll('_', ' ')}
                      </span>
                    </span>
                    <ArrowRight aria-hidden="true" className="ml-3 h-4 w-4" />
                  </CommandItem>
                ))}
              </CommandGroup>
            )}

            {query.trim() && (
              <CommandGroup heading="More">
                <CommandItem
                  value={`search all ${query}`}
                  onSelect={() => navigate(buildGlobalSearchRoute(query.trim()))}
                >
                  <Search aria-hidden="true" className="mr-3 h-4 w-4" />
                  <span className="flex-1 truncate">
                    Search all records for “{query.trim()}”
                  </span>
                  <ArrowRight aria-hidden="true" className="ml-3 h-4 w-4" />
                </CommandItem>
              </CommandGroup>
            )}
          </CommandList>
          <div className="flex items-center justify-between border-t px-3 py-2 text-[11px] text-muted-foreground">
            <span>↑↓ navigate · Enter open</span>
            <span>Esc close</span>
          </div>
        </Command>
      </DialogContent>
    </Dialog>
  );
}
