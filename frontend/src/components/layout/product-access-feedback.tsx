'use client';

import * as React from 'react';
import { AlertTriangle, CreditCard, RefreshCw, ToggleLeft } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  isEntitlementFailure,
  isEvaluationUnavailable,
  PRODUCT_ACCESS_EVENT,
  type ProductAccessFailure,
} from '@/lib/product-access';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

export function ProductAccessFeedback() {
  const [failure, setFailure] = React.useState<ProductAccessFailure | null>(null);
  React.useEffect(() => {
    function receive(event: Event) {
      setFailure((event as CustomEvent<ProductAccessFailure>).detail);
    }
    window.addEventListener(PRODUCT_ACCESS_EVENT, receive);
    return () => window.removeEventListener(PRODUCT_ACCESS_EVENT, receive);
  }, []);

  if (!failure) return null;
  const entitlement = isEntitlementFailure(failure);
  const unavailable = isEvaluationUnavailable(failure);
  const Icon = entitlement ? CreditCard : unavailable ? RefreshCw : ToggleLeft;
  const title = entitlement
    ? failure.code === 'ENTITLEMENT_LIMIT_EXCEEDED' ? 'Subscription capacity reached' : 'Upgrade required'
    : unavailable ? 'Availability check unavailable' : 'Capability disabled';
  const explanation = entitlement
    ? failure.code === 'ENTITLEMENT_LIMIT_EXCEEDED'
      ? 'Your plan’s current resource allowance has been reached. Review usage and plan options before trying again.'
      : 'This capability is not included in the organization’s current subscription.'
    : unavailable
      ? 'The platform failed closed because it could not verify feature or subscription availability. No restricted action was performed.'
      : 'An administrator, rollout rule, prerequisite, or platform safety switch currently disables this capability.';

  return (
    <Dialog open onOpenChange={(open) => !open && setFailure(null)}>
      <DialogContent>
        <DialogHeader>
          <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-full bg-amber-500/10">
            <Icon aria-hidden="true" className="h-5 w-5 text-amber-700" />
          </div>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{explanation}</DialogDescription>
        </DialogHeader>
        <div className="rounded-md bg-muted p-3 text-sm">
          <p>{failure.message}</p>
          {failure.requestId && <p className="mt-2 break-all font-mono text-xs text-muted-foreground">Support reference: {failure.requestId}</p>}
        </div>
        {unavailable && (
          <p className="flex gap-2 text-sm text-muted-foreground"><AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />Try the action again after a short wait. If it persists, give support the reference above.</p>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setFailure(null)}>Dismiss</Button>
          {entitlement && <Button asChild><Link href="/settings/subscription" onClick={() => setFailure(null)}>Review plan and usage</Link></Button>}
          {!entitlement && !unavailable && <Button asChild><Link href="/settings/capabilities" onClick={() => setFailure(null)}>View capability status</Link></Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
