'use client';

import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  FileCheck2,
  Gauge,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
  UserRound,
} from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn, formatDate, getStatusColor } from '@/lib/utils';
import { evidenceStatusLabel, formatEvidenceError } from '@/lib/control-evidence';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ControlEvidencePanel } from '@/components/evidence/control-evidence-panel';
import Link from 'next/link';
import { Skeleton } from '@/components/ui/skeleton';
import { useControlPermissions } from '@/hooks/use-control-permissions';
import { useParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';

export default function ControlDetailPage() {
  const params = useParams<{ id: string }>();
  const controlId = params.id;
  const permission = useControlPermissions();
  const query = useQuery({
    queryKey: ['controls', controlId],
    queryFn: () => api.controls.get(controlId),
    enabled: Boolean(controlId && permission.canRead),
  });

  if (permission.isLoading) return <ControlPageSkeleton />;
  if (permission.isError) {
    return (
      <AccessState
        icon={RefreshCw}
        title="Control access could not be verified"
        description="The permission service is unavailable. Control and evidence data were not loaded."
      >
        <Button type="button" variant="outline" onClick={() => void permission.retry()}>
          Try again
        </Button>
      </AccessState>
    );
  }
  if (!permission.canRead) {
    return (
      <AccessState
        icon={LockKeyhole}
        title="Control evidence unavailable"
        description="Your role does not grant controls:read access."
      />
    );
  }
  if (query.isLoading) return <ControlPageSkeleton />;
  if (query.isError || !query.data) {
    return (
      <div className="space-y-4">
        <Button asChild variant="ghost" size="sm">
          <Link href="/evidence">
            <ArrowLeft aria-hidden="true" className="mr-2 h-4 w-4" />
            Evidence overview
          </Link>
        </Button>
        <Card>
          <CardContent className="space-y-4 py-10 text-center">
            <AlertCircle aria-hidden="true" className="mx-auto h-9 w-9 text-destructive" />
            <p role="alert">{formatEvidenceError(query.error, 'list')}</p>
            <Button type="button" variant="outline" onClick={() => void query.refetch()}>
              <RefreshCw aria-hidden="true" className="mr-2 h-4 w-4" />
              Try again
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const control = query.data;
  const implementation = control.implementation;
  return (
    <div className="space-y-6">
      <Button asChild variant="ghost" size="sm">
        <Link href="/evidence">
          <ArrowLeft aria-hidden="true" className="mr-2 h-4 w-4" />
          Evidence overview
        </Link>
      </Button>

      <header className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="font-mono">
            {control.code}
          </Badge>
          {control.priority && (
            <Badge variant="secondary">{evidenceStatusLabel(control.priority)} priority</Badge>
          )}
          {control.is_mandatory && <Badge>Mandatory</Badge>}
        </div>
        <div>
          <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">{control.title}</h1>
          <p className="mt-1 max-w-4xl text-muted-foreground">
            {control.description ||
              'No control description is available to your current field-access policy.'}
          </p>
        </div>
      </header>

      <div className="grid gap-5 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Control context</CardTitle>
            <CardDescription>
              Immutable catalogue guidance for evidence collection and review.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <section>
              <h2 className="text-sm font-medium">Guidance</h2>
              <p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">
                {control.guidance || 'No implementation guidance is available.'}
              </p>
            </section>
            <dl className="grid gap-4 border-t pt-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground">Category</dt>
                <dd className="mt-1 font-medium">{control.category || 'Not classified'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Control type</dt>
                <dd className="mt-1 font-medium">{control.control_type || 'Not classified'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Evidence requirements</dt>
                <dd className="mt-1 font-medium">
                  {control.evidence_requirements ? 'Configured' : 'Not specified'}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Framework identifier</dt>
                <dd className="mt-1 break-all font-mono text-xs">{control.framework_id}</dd>
              </div>
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Tenant implementation</CardTitle>
            <CardDescription>Current adoption and operating posture.</CardDescription>
          </CardHeader>
          <CardContent>
            {implementation ? (
              <dl className="space-y-4 text-sm">
                <div>
                  <dt className="flex items-center gap-2 text-muted-foreground">
                    <ShieldCheck aria-hidden="true" className="h-4 w-4" />
                    Implementation state
                  </dt>
                  <dd className="mt-1">
                    <Badge className={cn(getStatusColor(implementation.status))}>
                      {evidenceStatusLabel(implementation.status)}
                    </Badge>
                  </dd>
                </div>
                <div>
                  <dt className="flex items-center gap-2 text-muted-foreground">
                    <CheckCircle2 aria-hidden="true" className="h-4 w-4" />
                    Delivery status
                  </dt>
                  <dd className="mt-1 font-medium">
                    {evidenceStatusLabel(implementation.implementation_status)}
                  </dd>
                </div>
                <div>
                  <dt className="flex items-center gap-2 text-muted-foreground">
                    <Gauge aria-hidden="true" className="h-4 w-4" />
                    Maturity
                  </dt>
                  <dd className="mt-1 font-medium">{implementation.maturity_level} of 5</dd>
                </div>
                <div>
                  <dt className="flex items-center gap-2 text-muted-foreground">
                    <UserRound aria-hidden="true" className="h-4 w-4" />
                    Owner
                  </dt>
                  <dd className="mt-1 break-all">{implementation.owner_user_id || 'Unassigned'}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Last updated</dt>
                  <dd className="mt-1">{formatDate(implementation.updated_at)}</dd>
                </div>
              </dl>
            ) : (
              <div className="py-6 text-center">
                <FileCheck2 aria-hidden="true" className="mx-auto h-9 w-9 text-muted-foreground" />
                <p className="mt-3 font-medium">Not adopted</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  This tenant has no implementation record for the control.
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <ControlEvidencePanel
        canApprove={permission.canApprove}
        canExport={permission.canExport}
        canUpdate={permission.canUpdate}
        control={control}
      />
    </div>
  );
}

function AccessState({
  children,
  description,
  icon: Icon,
  title,
}: {
  children?: React.ReactNode;
  description: string;
  icon: typeof LockKeyhole;
  title: string;
}) {
  return (
    <Card className="mx-auto max-w-xl">
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <Icon aria-hidden="true" className="h-10 w-10 text-muted-foreground" />
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
        {children}
      </CardContent>
    </Card>
  );
}

function ControlPageSkeleton() {
  return (
    <div role="status" aria-label="Loading control evidence" className="space-y-5">
      <Skeleton className="h-9 w-44" />
      <Skeleton className="h-28" />
      <div className="grid gap-5 lg:grid-cols-3">
        <Skeleton className="h-72 lg:col-span-2" />
        <Skeleton className="h-72" />
      </div>
      <Skeleton className="h-80" />
    </div>
  );
}
