'use client';

import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { Shield, AlertCircle, Inbox } from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import api from '@/lib/api';
import type { ComplianceFramework } from '@/types';
import type { PaginatedResponse } from '@/lib/api';

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

function useFrameworks() {
  return useQuery<PaginatedResponse<ComplianceFramework>>({
    queryKey: ['frameworks'],
    queryFn: () =>
      api.frameworks.list({
        page: 1,
        page_size: 100,
      }) as Promise<PaginatedResponse<ComplianceFramework>>,
  });
}

// ---------------------------------------------------------------------------
// Skeleton card
// ---------------------------------------------------------------------------

function FrameworkCardSkeleton() {
  return (
    <Card>
      <div className="h-1.5 animate-pulse rounded-t-lg bg-muted" />
      <CardContent className="p-6">
        <div className="animate-pulse space-y-3">
          <div className="h-5 w-3/4 rounded bg-muted" />
          <div className="h-4 w-1/2 rounded bg-muted" />
          <div className="flex items-center gap-2">
            <div className="h-5 w-16 rounded-full bg-muted" />
            <div className="h-5 w-20 rounded-full bg-muted" />
          </div>
          <div className="h-4 w-1/3 rounded bg-muted" />
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Framework Card
// ---------------------------------------------------------------------------

function FrameworkCard({
  framework,
}: {
  framework: ComplianceFramework;
}) {
  const borderColor = framework.color_hex ?? '#6B7280';

  return (
    <Link href={`/frameworks/${framework.id}`}>
      <Card className="transition-shadow hover:shadow-md cursor-pointer h-full">
        <div
          className="h-1.5 rounded-t-lg"
          style={{ backgroundColor: borderColor }}
        />
        <CardContent className="p-6">
          <div className="space-y-3">
            <div>
              <h3 className="font-semibold text-lg leading-tight">
                {framework.name}
              </h3>
              <p className="text-sm text-muted-foreground mt-1">
                {framework.version}
                {framework.issuing_body
                  ? ` \u00b7 ${framework.issuing_body}`
                  : ''}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {framework.category && (
                <Badge variant="secondary">{framework.category}</Badge>
              )}
              <Badge variant="outline">
                <Shield className="mr-1 h-3 w-3" />
                {framework.total_controls} controls
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function FrameworksPage() {
  const { data, isLoading, error } = useFrameworks();

  const frameworks = data?.items ?? [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Frameworks</h1>
        <p className="text-muted-foreground">
          Compliance frameworks adopted by your organisation.
        </p>
      </div>

      {/* Error state */}
      {error && (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load frameworks. Please try again.</span>
          </CardContent>
        </Card>
      )}

      {/* Loading state */}
      {isLoading && (
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 9 }).map((_, i) => (
            <FrameworkCardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && frameworks.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-16 text-center">
            <Inbox className="h-12 w-12 text-muted-foreground" />
            <h3 className="mt-4 text-lg font-semibold">
              No frameworks configured
            </h3>
            <p className="mt-1 text-sm text-muted-foreground">
              Contact your administrator to adopt compliance frameworks.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Framework grid */}
      {!isLoading && !error && frameworks.length > 0 && (
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
          {frameworks.map((fw) => (
            <FrameworkCard key={fw.id} framework={fw} />
          ))}
        </div>
      )}
    </div>
  );
}
