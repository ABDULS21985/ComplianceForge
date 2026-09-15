'use client';

import { Card, CardContent } from '@/components/ui/card';
import {
  CreditCard,
  DatabaseZap,
  Fingerprint,
  LockKeyhole,
  RefreshCw,
  Scale,
  ScrollText,
  ToggleLeft,
} from 'lucide-react';
import {
  DATA_GOVERNANCE_CAPABILITY,
  dataGovernanceKeys,
  formatGovernanceError,
} from '@/lib/data-governance';
import {
  GovernancePageSkeleton,
  ReadOnlyNotice,
} from '@/components/data-governance/governance-states';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import api from '@/lib/api';
import { Button } from '@/components/ui/button';
import { featureEvaluationExplanation } from '@/lib/feature-flags';
import { GovernanceHistoryPanel } from '@/components/data-governance/history-panel';
import { LegalHoldsPanel } from '@/components/data-governance/legal-holds-panel';
import Link from 'next/link';
import { PolicyPanel } from '@/components/data-governance/policy-panel';
import { RecordsPanel } from '@/components/data-governance/records-panel';
import { SchedulesPanel } from '@/components/data-governance/schedules-panel';
import { useCapabilityPermissions } from '@/hooks/use-capability-permissions';
import { useQuery } from '@tanstack/react-query';

export default function DataGovernancePage() {
  const permission = useCapabilityPermissions();
  const capabilityQuery = useQuery({
    queryKey: dataGovernanceKeys.capability,
    queryFn: () => api.featureFlags.evaluate(DATA_GOVERNANCE_CAPABILITY),
    enabled: permission.canRead,
    retry: false,
    staleTime: 30_000,
  });

  if (permission.isLoading || (permission.canRead && capabilityQuery.isLoading))
    return <GovernancePageSkeleton />;
  if (permission.isError)
    return (
      <AccessState
        icon={RefreshCw}
        title="Settings access could not be verified"
        description="The permission service is temporarily unavailable. No governance data was loaded."
      >
        <Button type="button" variant="outline" onClick={() => void permission.retry()}>
          Try again
        </Button>
      </AccessState>
    );
  if (!permission.canRead)
    return (
      <AccessState
        icon={LockKeyhole}
        title="Data lifecycle governance unavailable"
        description="Your role does not grant organization settings access."
      />
    );
  if (capabilityQuery.isError)
    return (
      <AccessState
        icon={RefreshCw}
        title="Capability evaluation unavailable"
        description={formatGovernanceError(
          capabilityQuery.error,
          'The platform could not safely verify data-lifecycle availability. No restricted data was loaded.',
        )}
      >
        <Button type="button" variant="outline" onClick={() => void capabilityQuery.refetch()}>
          Try again
        </Button>
      </AccessState>
    );
  if (!capabilityQuery.data?.enabled) {
    const subscriptionDenied = capabilityQuery.data?.evaluation_reason === 'subscription_denied';
    return (
      <AccessState
        icon={subscriptionDenied ? CreditCard : ToggleLeft}
        title={
          subscriptionDenied
            ? 'Data lifecycle upgrade required'
            : 'Data lifecycle capability disabled'
        }
        description={
          capabilityQuery.data
            ? featureEvaluationExplanation(capabilityQuery.data)
            : 'This capability is not currently available.'
        }
      >
        <Button asChild>
          <Link href={subscriptionDenied ? '/settings/subscription' : '/settings/capabilities'}>
            {subscriptionDenied ? 'Review plan and usage' : 'View capability status'}
          </Link>
        </Button>
      </AccessState>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Data lifecycle governance</h1>
        <p className="mt-1 max-w-4xl text-muted-foreground">
          Control residency, retention, disposition, legal preservation, and independently
          verifiable change history across governed records.
        </p>
      </div>
      {!permission.canConfigure && <ReadOnlyNotice />}
      <Tabs defaultValue="policy" className="space-y-5">
        <TabsList
          aria-label="Data lifecycle governance sections"
          className="h-auto max-w-full flex-wrap justify-start"
        >
          <TabsTrigger value="policy" className="gap-2">
            <DatabaseZap aria-hidden="true" className="h-4 w-4" />
            Policy & residency
          </TabsTrigger>
          <TabsTrigger value="schedules" className="gap-2">
            <ScrollText aria-hidden="true" className="h-4 w-4" />
            Retention schedules
          </TabsTrigger>
          <TabsTrigger value="records" className="gap-2">
            <Scale aria-hidden="true" className="h-4 w-4" />
            Records & disposition
          </TabsTrigger>
          <TabsTrigger value="holds" className="gap-2">
            <LockKeyhole aria-hidden="true" className="h-4 w-4" />
            Legal holds
          </TabsTrigger>
          <TabsTrigger value="history" className="gap-2">
            <Fingerprint aria-hidden="true" className="h-4 w-4" />
            History & verification
          </TabsTrigger>
        </TabsList>
        <TabsContent value="policy">
          <PolicyPanel canConfigure={permission.canConfigure} />
        </TabsContent>
        <TabsContent value="schedules">
          <SchedulesPanel canConfigure={permission.canConfigure} />
        </TabsContent>
        <TabsContent value="records">
          <RecordsPanel canConfigure={permission.canConfigure} />
        </TabsContent>
        <TabsContent value="holds">
          <LegalHoldsPanel canConfigure={permission.canConfigure} />
        </TabsContent>
        <TabsContent value="history">
          <GovernanceHistoryPanel />
        </TabsContent>
      </Tabs>
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
