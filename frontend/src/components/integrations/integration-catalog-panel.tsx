'use client';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Cloud, KeyRound, MessageSquare, Search, ShieldCheck, TicketCheck, Webhook } from 'lucide-react';
import { formatApiError, INTEGRATION_CATALOG, type IntegrationCatalogEntry } from '@/lib/enterprise-settings';
import { useMemo, useState } from 'react';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { IntegrationEditorDialog } from '@/components/integrations/integration-editor-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useQuery } from '@tanstack/react-query';

function CategoryIcon({ category }: { category: IntegrationCatalogEntry['category'] }) {
  const Icon = category === 'Identity' ? KeyRound : category === 'Cloud' ? Cloud : category === 'Security' ? ShieldCheck : category === 'IT service management' ? TicketCheck : category === 'Messaging' ? MessageSquare : Webhook;
  return <Icon aria-hidden="true" className="h-5 w-5" />;
}

export function IntegrationCatalogPanel({ canConfigure }: { canConfigure: boolean }) {
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<IntegrationCatalogEntry | null>(null);
  const integrationsQuery = useQuery({
    queryKey: ['integrations'],
    queryFn: () => api.integrations.list(),
    staleTime: 30_000,
  });
  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return query
      ? INTEGRATION_CATALOG.filter((entry) => `${entry.name} ${entry.category} ${entry.description} ${entry.type}`.toLowerCase().includes(query))
      : INTEGRATION_CATALOG;
  }, [search]);

  return (
    <section aria-labelledby="integration-catalog-heading" className="space-y-5">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div><h2 id="integration-catalog-heading" className="text-xl font-semibold">Connector catalog</h2><p className="text-sm text-muted-foreground">All integration types accepted by the API, grouped by domain.</p></div>
        <div className="relative w-full sm:max-w-xs"><Search aria-hidden="true" className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" /><Input aria-label="Search connector catalog" value={search} onChange={(event) => setSearch(event.target.value)} className="pl-9" placeholder="Search connectors" /></div>
      </div>

      {integrationsQuery.isLoading && <div role="status" aria-label="Loading connector catalog" className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 6 }, (_, index) => <Skeleton key={index} className="h-52" />)}</div>}
      {integrationsQuery.isError && <Card><CardContent className="space-y-3 py-8 text-center"><p role="alert">{formatApiError(integrationsQuery.error, 'Configured integrations could not be loaded.')}</p><Button type="button" variant="outline" onClick={() => void integrationsQuery.refetch()}>Try again</Button></CardContent></Card>}
      {integrationsQuery.isSuccess && filtered.length === 0 && <Card><CardContent className="py-10 text-center text-sm text-muted-foreground">No connectors match “{search}”.</CardContent></Card>}

      {integrationsQuery.isSuccess && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {filtered.map((entry) => {
            const configuredCount = integrationsQuery.data.data.filter((integration) => integration.integration_type === entry.type).length;
            return (
              <Card key={entry.type} className="flex flex-col">
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between gap-3"><span className="rounded-md bg-muted p-2 text-muted-foreground"><CategoryIcon category={entry.category} /></span>{configuredCount > 0 && <Badge variant="secondary">{configuredCount} configured</Badge>}</div>
                  <CardTitle className="pt-2 text-lg">{entry.name}</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-1 flex-col gap-4"><div className="flex-1"><Badge variant="outline" className="mb-2 font-normal">{entry.category}</Badge><p className="text-sm text-muted-foreground">{entry.description}</p></div>{canConfigure && <Button type="button" variant="outline" onClick={() => setSelected(entry)}>Configure</Button>}</CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {selected && <IntegrationEditorDialog key={selected.type} catalogEntry={selected} open onOpenChange={(open) => !open && setSelected(null)} />}
    </section>
  );
}
