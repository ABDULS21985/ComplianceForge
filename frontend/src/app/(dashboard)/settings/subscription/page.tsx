'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Subscription {
  id: string;
  plan_name: string;
  tier: string;
  billing_cycle: 'monthly' | 'annual';
  next_billing_date: string;
  status: 'active' | 'trialing' | 'past_due' | 'cancelled';
}

interface Usage {
  users: { current: number; max: number };
  frameworks: { current: number; max: number };
  risks: { current: number; max: number };
  vendors: { current: number; max: number };
  storage_gb: { current: number; max: number };
}

interface Plan {
  id: string;
  name: string;
  tier: string;
  price_monthly: number;
  price_annual: number;
  features: Record<string, string | number | boolean>;
  limits: {
    users: number;
    frameworks: number;
    risks: number;
    vendors: number;
    storage_gb: number;
  };
}

const FEATURE_ROWS = [
  { key: 'users', label: 'Users' },
  { key: 'frameworks', label: 'Frameworks' },
  { key: 'risks', label: 'Risk Items' },
  { key: 'vendors', label: 'Vendors' },
  { key: 'storage_gb', label: 'Storage (GB)' },
  { key: 'sso', label: 'SSO' },
  { key: 'api_access', label: 'API Access' },
  { key: 'custom_reports', label: 'Custom Reports' },
  { key: 'monitoring', label: 'Continuous Monitoring' },
  { key: 'priority_support', label: 'Priority Support' },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function SubscriptionPage() {
  const queryClient = useQueryClient();
  const [cancelDialog, setCancelDialog] = useState(false);
  const [cancelReason, setCancelReason] = useState('');
  const [confirmDowngrade, setConfirmDowngrade] = useState<string | null>(null);

  // Queries
  const { data: subscription, isLoading, error } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription.get(),
  });

  const { data: usage } = useQuery({
    queryKey: ['subscription-usage'],
    queryFn: () => api.subscription.usage(),
  });

  const { data: plans } = useQuery({
    queryKey: ['subscription-plans'],
    queryFn: () => api.subscription.listPlans(),
  });

  // Mutations
  const changePlan = useMutation({
    mutationFn: (data: any) => api.subscription.changePlan(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscription'] });
      queryClient.invalidateQueries({ queryKey: ['subscription-usage'] });
      setConfirmDowngrade(null);
    },
  });

  const cancelSubscription = useMutation({
    mutationFn: (data: any) => api.subscription.cancel(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscription'] });
      setCancelDialog(false);
      setCancelReason('');
    },
  });

  const sub: Subscription | null = subscription ?? null;
  const usageData: Usage | null = usage ?? null;
  const planList: Plan[] = plans?.items ?? plans ?? [];

  // Tier order for upgrade/downgrade detection
  const tierOrder: Record<string, number> = { starter: 0, professional: 1, enterprise: 2 };
  const currentTierRank = sub ? (tierOrder[sub.tier?.toLowerCase()] ?? -1) : -1;

  function tierBadgeColor(tier: string) {
    const map: Record<string, string> = {
      starter: 'bg-gray-100 text-gray-700',
      professional: 'bg-blue-100 text-blue-700',
      enterprise: 'bg-purple-100 text-purple-700',
    };
    return map[tier?.toLowerCase()] ?? map.starter;
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-2xl font-bold">Subscription</h1>
        <div className="h-32 rounded-lg bg-gray-100 animate-pulse" />
        <div className="grid grid-cols-5 gap-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-20 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">Subscription</h1>
        <div className="bg-red-50 text-red-700 rounded-lg p-4">Failed to load subscription details.</div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-8">
      <h1 className="text-2xl font-bold">Subscription Management</h1>

      {/* Current Plan Card */}
      {sub && (
        <div className="border rounded-lg p-6 bg-white">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-3 mb-1">
                <h2 className="text-xl font-bold">{sub.plan_name}</h2>
                <span className={`text-xs font-bold px-2.5 py-1 rounded-full uppercase ${tierBadgeColor(sub.tier)}`}>
                  {sub.tier}
                </span>
              </div>
              <p className="text-sm text-gray-500">
                Billed {sub.billing_cycle} &middot; Next billing:{' '}
                {new Date(sub.next_billing_date).toLocaleDateString()}
              </p>
            </div>
            <div className="flex gap-2">
              <span className={`text-xs font-medium px-2 py-1 rounded-full ${
                sub.status === 'active' ? 'bg-green-100 text-green-700' :
                sub.status === 'trialing' ? 'bg-blue-100 text-blue-700' :
                sub.status === 'past_due' ? 'bg-amber-100 text-amber-700' :
                'bg-red-100 text-red-700'
              }`}>
                {sub.status.replace(/_/g, ' ').toUpperCase()}
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Usage Meters */}
      {usageData && (
        <div>
          <h3 className="text-lg font-semibold mb-3">Usage</h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
            {([
              { key: 'users', label: 'Users' },
              { key: 'frameworks', label: 'Frameworks' },
              { key: 'risks', label: 'Risks' },
              { key: 'vendors', label: 'Vendors' },
              { key: 'storage_gb', label: 'Storage (GB)' },
            ] as const).map(({ key, label }) => {
              const meter = usageData[key as keyof Usage];
              if (!meter) return null;
              const pct = meter.max > 0 ? Math.min((meter.current / meter.max) * 100, 100) : 0;
              const isHigh = pct >= 80;
              return (
                <div key={key} className="border rounded-lg p-4 bg-white">
                  <p className="text-sm font-medium text-gray-600">{label}</p>
                  <p className="text-lg font-bold mt-1">
                    {meter.current} <span className="text-sm font-normal text-gray-400">/ {meter.max === -1 ? 'Unlimited' : meter.max}</span>
                  </p>
                  <div className="mt-2 h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div
                      className={`h-full rounded-full transition-all ${isHigh ? 'bg-amber-500' : 'bg-blue-500'}`}
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Plan Comparison */}
      {planList.length > 0 && (
        <div>
          <h3 className="text-lg font-semibold mb-3">Plans</h3>
          <div className="border rounded-lg overflow-hidden bg-white">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-3 font-medium">Feature</th>
                  {planList.map((plan) => (
                    <th key={plan.id} className="text-center px-4 py-3 font-medium">
                      <span>{plan.name}</span>
                      {sub && plan.tier?.toLowerCase() === sub.tier?.toLowerCase() && (
                        <span className="ml-1 text-xs text-blue-600">(Current)</span>
                      )}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {FEATURE_ROWS.map((row) => (
                  <tr key={row.key} className="border-b last:border-0">
                    <td className="px-4 py-2 font-medium text-gray-700">{row.label}</td>
                    {planList.map((plan) => {
                      const val = plan.limits?.[row.key as keyof Plan['limits']] ?? plan.features?.[row.key];
                      return (
                        <td key={plan.id} className="px-4 py-2 text-center">
                          {typeof val === 'boolean' ? (
                            val ? (
                              <span className="text-green-600 font-bold">Yes</span>
                            ) : (
                              <span className="text-gray-300">--</span>
                            )
                          ) : val === -1 ? (
                            'Unlimited'
                          ) : (
                            String(val ?? '--')
                          )}
                        </td>
                      );
                    })}
                  </tr>
                ))}
                {/* Price row */}
                <tr className="border-t-2">
                  <td className="px-4 py-3 font-bold">Monthly Price</td>
                  {planList.map((plan) => (
                    <td key={plan.id} className="px-4 py-3 text-center font-bold text-lg">
                      {plan.price_monthly === 0 ? 'Free' : `$${plan.price_monthly}/mo`}
                    </td>
                  ))}
                </tr>
                {/* Action row */}
                <tr>
                  <td className="px-4 py-3" />
                  {planList.map((plan) => {
                    const planRank = tierOrder[plan.tier?.toLowerCase()] ?? -1;
                    const isCurrent = sub && plan.tier?.toLowerCase() === sub.tier?.toLowerCase();
                    const isUpgrade = planRank > currentTierRank;
                    const isDowngrade = planRank < currentTierRank;

                    return (
                      <td key={plan.id} className="px-4 py-3 text-center">
                        {isCurrent ? (
                          <span className="text-sm text-gray-400 font-medium">Current Plan</span>
                        ) : isUpgrade ? (
                          <button
                            onClick={() => changePlan.mutate({ plan_id: plan.id })}
                            disabled={changePlan.isPending}
                            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                          >
                            Upgrade
                          </button>
                        ) : isDowngrade ? (
                          <button
                            onClick={() => setConfirmDowngrade(plan.id)}
                            className="px-4 py-2 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
                          >
                            Downgrade
                          </button>
                        ) : null}
                      </td>
                    );
                  })}
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Billing History Placeholder */}
      <div>
        <h3 className="text-lg font-semibold mb-3">Billing History</h3>
        <div className="border rounded-lg p-8 bg-white text-center text-gray-400">
          Billing history will appear here once invoices are generated.
        </div>
      </div>

      {/* Cancel Subscription */}
      <div className="border-t pt-6">
        <button
          onClick={() => setCancelDialog(true)}
          className="text-sm text-red-600 hover:text-red-800 font-medium"
        >
          Cancel Subscription
        </button>
      </div>

      {/* Downgrade Confirmation */}
      {confirmDowngrade && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-lg shadow-xl max-w-md w-full p-6 space-y-4">
            <h2 className="text-lg font-bold">Confirm Downgrade</h2>
            <p className="text-sm text-gray-600">
              Downgrading may reduce your limits. Features or data exceeding the new plan limits will become read-only.
              Are you sure you want to proceed?
            </p>
            <div className="flex gap-2 justify-end">
              <button onClick={() => setConfirmDowngrade(null)} className="px-4 py-2 text-sm rounded border border-gray-300 hover:bg-gray-50">
                Cancel
              </button>
              <button
                onClick={() => changePlan.mutate({ plan_id: confirmDowngrade })}
                disabled={changePlan.isPending}
                className="px-4 py-2 text-sm font-medium rounded bg-red-600 text-white hover:bg-red-700 disabled:opacity-50"
              >
                {changePlan.isPending ? 'Processing...' : 'Confirm Downgrade'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Cancel Subscription Dialog */}
      {cancelDialog && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-lg shadow-xl max-w-md w-full p-6 space-y-4">
            <h2 className="text-lg font-bold text-red-600">Cancel Subscription</h2>
            <p className="text-sm text-gray-600">
              We are sorry to see you go. Your account will remain active until the end of the current billing period.
              Please let us know why you are cancelling.
            </p>
            <div>
              <label className="block text-sm font-medium mb-1">Reason</label>
              <select
                value={cancelReason}
                onChange={(e) => setCancelReason(e.target.value)}
                className="w-full border rounded px-3 py-2 text-sm"
              >
                <option value="">Select a reason</option>
                <option value="too_expensive">Too expensive</option>
                <option value="missing_features">Missing features</option>
                <option value="switching_provider">Switching to another provider</option>
                <option value="no_longer_needed">No longer needed</option>
                <option value="other">Other</option>
              </select>
            </div>
            <div className="flex gap-2 justify-end">
              <button onClick={() => setCancelDialog(false)} className="px-4 py-2 text-sm rounded border border-gray-300 hover:bg-gray-50">
                Keep Subscription
              </button>
              <button
                onClick={() => cancelSubscription.mutate({ reason: cancelReason })}
                disabled={!cancelReason || cancelSubscription.isPending}
                className="px-4 py-2 text-sm font-medium rounded bg-red-600 text-white hover:bg-red-700 disabled:opacity-50"
              >
                {cancelSubscription.isPending ? 'Cancelling...' : 'Cancel Subscription'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
