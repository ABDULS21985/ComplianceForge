'use client';

import { ArrowDown, ArrowUp, ArrowUpDown, ChevronLeft, ChevronRight, Search } from 'lucide-react';
import React, { useCallback, useEffect, useState } from 'react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { EmptyState } from '@/components/data/empty-state';
import { Input } from '@/components/ui/input';
import type { Pagination } from '@/types';
import { TableSkeleton } from '@/components/data/loading-skeleton';

// ---- Types ----
export interface ColumnDef<T> {
  key: string;
  label: string;
  sortable?: boolean;
  className?: string;
  render?: (row: T) => React.ReactNode;
}

export interface FilterConfig {
  key: string;
  label: string;
  options: { label: string; value: string }[];
}

interface DataTableProps<T> {
  accessibleLabel?: string;
  columns: ColumnDef<T>[];
  data: T[];
  pagination?: Pagination;
  onPageChange?: (page: number) => void;
  onSortChange?: (field: string, direction: 'asc' | 'desc') => void;
  onSearch?: (query: string) => void;
  searchPlaceholder?: string;
  isLoading?: boolean;
  emptyMessage?: string;
  emptyDescription?: string;
  onRowClick?: (row: T) => void;
  filters?: FilterConfig[];
  onFilterChange?: (key: string, value: string) => void;
  className?: string;
}

export function DataTable<T extends Record<string, unknown>>({
  accessibleLabel = 'Data table',
  columns,
  data,
  pagination,
  onPageChange,
  onSortChange,
  onSearch,
  searchPlaceholder = 'Search...',
  isLoading = false,
  emptyMessage = 'No results found',
  emptyDescription,
  onRowClick,
  filters,
  onFilterChange,
  className,
}: DataTableProps<T>) {
  const [searchValue, setSearchValue] = useState('');
  const [sortField, setSortField] = useState<string | null>(null);
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');

  // Debounced search
  useEffect(() => {
    if (!onSearch) return;
    const timer = setTimeout(() => {
      onSearch(searchValue);
    }, 500);
    return () => clearTimeout(timer);
  }, [searchValue, onSearch]);

  const handleSort = useCallback(
    (field: string) => {
      let direction: 'asc' | 'desc' = 'asc';
      if (sortField === field && sortDirection === 'asc') {
        direction = 'desc';
      }
      setSortField(field);
      setSortDirection(direction);
      onSortChange?.(field, direction);
    },
    [sortField, sortDirection, onSortChange]
  );

  const getSortIcon = (field: string) => {
    if (sortField !== field) return <ArrowUpDown aria-hidden="true" className="ml-1 h-3.5 w-3.5 text-muted-foreground/50" />;
    return sortDirection === 'asc' ? (
      <ArrowUp aria-hidden="true" className="ml-1 h-3.5 w-3.5" />
    ) : (
      <ArrowDown aria-hidden="true" className="ml-1 h-3.5 w-3.5" />
    );
  };

  // Pagination helpers
  const startItem = pagination
    ? (pagination.page - 1) * pagination.page_size + 1
    : 0;
  const endItem = pagination
    ? Math.min(pagination.page * pagination.page_size, pagination.total_items)
    : data.length;
  const rowLabel = (row: T, rowIndex: number) => {
    const identity = row.name ?? row.title ?? row.reference ?? row.id;
    return `Open ${typeof identity === 'string' && identity.trim() ? identity : `row ${rowIndex + 1}`}`;
  };

  return (
    <div className={cn('space-y-4', className)}>
      {/* Toolbar: search + filters */}
      {(onSearch || (filters && filters.length > 0)) && (
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          {onSearch && (
            <div className="relative max-w-sm flex-1">
              <Search aria-hidden="true" className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                aria-label={searchPlaceholder}
                placeholder={searchPlaceholder}
                value={searchValue}
                onChange={(e) => setSearchValue(e.target.value)}
                className="pl-9"
              />
            </div>
          )}
          {filters && filters.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {filters.map((filter) => (
                <Select
                  key={filter.key}
                  onValueChange={(value) =>
                    onFilterChange?.(filter.key, value === '__all__' ? '' : value)
                  }
                >
                  <SelectTrigger aria-label={filter.label} className="min-h-11 w-[150px]">
                    <SelectValue placeholder={filter.label} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">All</SelectItem>
                    {filter.options.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Table */}
      {isLoading ? (
        <TableSkeleton rows={5} cols={columns.length} />
      ) : data.length === 0 ? (
        <EmptyState title={emptyMessage} description={emptyDescription} />
      ) : (
        <div className="rounded-md border">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <caption className="sr-only">{accessibleLabel}</caption>
              <thead>
                <tr className="border-b bg-muted/50">
                  {columns.map((col) => (
                    <th
                      key={col.key}
                      scope="col"
                      aria-sort={
                        col.sortable && sortField === col.key
                          ? sortDirection === 'asc'
                            ? 'ascending'
                            : 'descending'
                          : undefined
                      }
                      className={cn(
                        'px-4 py-3 text-left font-medium text-muted-foreground',
                        col.className
                      )}
                    >
                      {col.sortable ? (
                        <button
                          type="button"
                          className="flex min-h-11 items-center rounded-md text-left hover:text-foreground"
                          onClick={() => handleSort(col.key)}
                        >
                          {col.label}
                          {getSortIcon(col.key)}
                        </button>
                      ) : col.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.map((row, rowIndex) => (
                  <tr
                    key={(row.id as string) ?? rowIndex}
                    className={cn(
                      'border-b transition-colors hover:bg-muted/50',
                      onRowClick && 'cursor-pointer'
                    )}
                    onClick={() => onRowClick?.(row)}
                  >
                    {columns.map((col, columnIndex) => (
                      <td key={col.key} className={cn('px-4 py-3', col.className)}>
                        {col.render
                          ? col.render(row)
                          : (row[col.key] as React.ReactNode) ?? '—'}
                        {columnIndex === 0 && onRowClick && (
                          <button
                            type="button"
                            className="sr-only min-h-11 rounded-md px-3 focus:not-sr-only"
                            onClick={(event) => {
                              event.stopPropagation();
                              onRowClick(row);
                            }}
                          >
                            {rowLabel(row, rowIndex)}
                          </button>
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Footer: pagination */}
      {pagination && pagination.total_items > 0 && (
        <div className="flex flex-col gap-3 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          <span>
            Showing {startItem}–{endItem} of {pagination.total_items} results
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={pagination.page <= 1}
              onClick={() => onPageChange?.(pagination.page - 1)}
            >
              <ChevronLeft aria-hidden="true" className="mr-1 h-4 w-4" />
              Previous
            </Button>
            <span className="px-2">
              Page {pagination.page} of {pagination.total_pages}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={pagination.page >= pagination.total_pages}
              onClick={() => onPageChange?.(pagination.page + 1)}
            >
              Next
              <ChevronRight aria-hidden="true" className="ml-1 h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
