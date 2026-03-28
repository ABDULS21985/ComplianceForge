'use client';

import { useState } from 'react';
import Link from 'next/link';
import { z } from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Building2,
  AlertCircle,
  AlertTriangle,
  Plus,
  Check,
  X,
  ExternalLink,
  ChevronLeft,
  ChevronRight,
  DollarSign,
  FileWarning,
} from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { formatDate, formatCurrency, getRiskLevelColor } from '@/lib/utils';
import { COUNTRIES_EU_UK } from '@/lib/constants';
import {
  useVendors,
  useVendorStats,
  useCreateVendor,
} from '@/lib/api-hooks';
import type { Vendor, VendorStats } from '@/types';
import type { PaginatedResponse } from '@/lib/api';

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const onboardVendorSchema = z.object({
  name: z.string().min(1, 'Vendor name is required').max(200),
  legal_name: z.string().max(200).optional(),
  website: z.string().url('Must be a valid URL').optional().or(z.literal('')),
  country_code: z.string().min(2, 'Country is required'),
  contact_name: z.string().min(1, 'Contact name is required').max(100),
  contact_email: z.string().email('Must be a valid email'),
  risk_tier: z.enum(['critical', 'high', 'medium', 'low']),
  service_description: z.string().max(1000).optional(),
  data_processing: z.boolean().default(false),
  data_categories: z.array(z.string()).optional(),
  certifications: z.array(z.string()).default([]),
  owner_user_id: z.string().optional(),
});

type OnboardVendorValues = z.infer<typeof onboardVendorSchema>;

const CERTIFICATION_OPTIONS = [
  'ISO 27001',
  'SOC 2',
  'PCI DSS',
  'Cyber Essentials',
];

const DATA_CATEGORY_OPTIONS = [
  'Personal Data',
  'Special Category Data',
  'Financial Data',
  'Health Data',
  'Employee Data',
  'Customer Data',
  'Children\'s Data',
];

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatCard({
  title,
  value,
  subtitle,
  icon: Icon,
  className,
}: {
  title: string;
  value: string | number;
  subtitle?: string;
  icon: React.ElementType;
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardContent className="p-6">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <Icon className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="mt-2">
          <p className="text-3xl font-bold">{value}</p>
          {subtitle && (
            <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function StatCardSkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-3">
          <div className="h-4 w-24 rounded bg-muted" />
          <div className="h-8 w-16 rounded bg-muted" />
          <div className="h-3 w-32 rounded bg-muted" />
        </div>
      </CardContent>
    </Card>
  );
}

function TableSkeleton() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-5 w-40 rounded bg-muted" />
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex gap-4">
              <div className="h-4 w-1/5 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
              <div className="h-4 w-1/6 rounded bg-muted" />
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Onboard Vendor Sheet
// ---------------------------------------------------------------------------

function OnboardVendorSheet() {
  const [open, setOpen] = useState(false);
  const createVendor = useCreateVendor();

  const form = useForm<OnboardVendorValues>({
    resolver: zodResolver(onboardVendorSchema),
    defaultValues: {
      name: '',
      legal_name: '',
      website: '',
      country_code: '',
      contact_name: '',
      contact_email: '',
      risk_tier: 'medium',
      service_description: '',
      data_processing: false,
      data_categories: [],
      certifications: [],
      owner_user_id: '',
    },
  });

  const dataProcessing = form.watch('data_processing');

  const onSubmit = (values: OnboardVendorValues) => {
    createVendor.mutate(values, {
      onSuccess: () => {
        setOpen(false);
        form.reset();
      },
    });
  };

  const toggleCertification = (cert: string) => {
    const current = form.getValues('certifications');
    if (current.includes(cert)) {
      form.setValue('certifications', current.filter((c) => c !== cert));
    } else {
      form.setValue('certifications', [...current, cert]);
    }
  };

  const toggleDataCategory = (cat: string) => {
    const current = form.getValues('data_categories') ?? [];
    if (current.includes(cat)) {
      form.setValue('data_categories', current.filter((c) => c !== cat));
    } else {
      form.setValue('data_categories', [...current, cat]);
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Onboard Vendor
        </Button>
      </SheetTrigger>
      <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>Onboard New Vendor</SheetTitle>
          <SheetDescription>
            Register a new third-party vendor for risk management and compliance tracking.
          </SheetDescription>
        </SheetHeader>
        <form onSubmit={form.handleSubmit(onSubmit)} className="mt-6 space-y-5">
          {/* Name */}
          <div className="space-y-2">
            <Label htmlFor="name">Vendor Name *</Label>
            <Input id="name" {...form.register('name')} placeholder="Acme Corp" />
            {form.formState.errors.name && (
              <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
            )}
          </div>

          {/* Legal Name */}
          <div className="space-y-2">
            <Label htmlFor="legal_name">Legal Name</Label>
            <Input id="legal_name" {...form.register('legal_name')} placeholder="Acme Corporation Ltd" />
          </div>

          {/* Website */}
          <div className="space-y-2">
            <Label htmlFor="website">Website</Label>
            <Input id="website" {...form.register('website')} placeholder="https://example.com" />
            {form.formState.errors.website && (
              <p className="text-xs text-destructive">{form.formState.errors.website.message}</p>
            )}
          </div>

          {/* Country */}
          <div className="space-y-2">
            <Label>Country *</Label>
            <Select
              value={form.watch('country_code')}
              onValueChange={(v) => form.setValue('country_code', v)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select country" />
              </SelectTrigger>
              <SelectContent>
                {COUNTRIES_EU_UK.map((c) => (
                  <SelectItem key={c.code} value={c.code}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {form.formState.errors.country_code && (
              <p className="text-xs text-destructive">{form.formState.errors.country_code.message}</p>
            )}
          </div>

          {/* Contact Name */}
          <div className="space-y-2">
            <Label htmlFor="contact_name">Contact Name *</Label>
            <Input id="contact_name" {...form.register('contact_name')} placeholder="Jane Smith" />
            {form.formState.errors.contact_name && (
              <p className="text-xs text-destructive">{form.formState.errors.contact_name.message}</p>
            )}
          </div>

          {/* Contact Email */}
          <div className="space-y-2">
            <Label htmlFor="contact_email">Contact Email *</Label>
            <Input id="contact_email" type="email" {...form.register('contact_email')} placeholder="jane@example.com" />
            {form.formState.errors.contact_email && (
              <p className="text-xs text-destructive">{form.formState.errors.contact_email.message}</p>
            )}
          </div>

          {/* Risk Tier */}
          <div className="space-y-2">
            <Label>Risk Tier *</Label>
            <Select
              value={form.watch('risk_tier')}
              onValueChange={(v) => form.setValue('risk_tier', v as OnboardVendorValues['risk_tier'])}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select risk tier" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="critical">Critical</SelectItem>
                <SelectItem value="high">High</SelectItem>
                <SelectItem value="medium">Medium</SelectItem>
                <SelectItem value="low">Low</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Service Description */}
          <div className="space-y-2">
            <Label htmlFor="service_description">Service Description</Label>
            <Textarea
              id="service_description"
              {...form.register('service_description')}
              placeholder="Describe the services provided..."
              rows={3}
            />
          </div>

          <Separator />

          {/* Data Processing Switch */}
          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="data_processing">Data Processing</Label>
              <p className="text-xs text-muted-foreground">
                Does this vendor process personal data on your behalf?
              </p>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={dataProcessing}
              onClick={() => form.setValue('data_processing', !dataProcessing)}
              className={cn(
                'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors',
                dataProcessing ? 'bg-primary' : 'bg-muted'
              )}
            >
              <span
                className={cn(
                  'pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform',
                  dataProcessing ? 'translate-x-5' : 'translate-x-0'
                )}
              />
            </button>
          </div>

          {/* GDPR DPA Warning */}
          {dataProcessing && (
            <div className="rounded-lg border border-yellow-300 bg-yellow-50 p-4 dark:border-yellow-700 dark:bg-yellow-950/40">
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-yellow-600 dark:text-yellow-400" />
                <div>
                  <p className="text-sm font-medium text-yellow-800 dark:text-yellow-300">
                    Data Processing Agreement Required
                  </p>
                  <p className="mt-1 text-xs text-yellow-700 dark:text-yellow-400">
                    A Data Processing Agreement will be required per GDPR Article 28.
                    Ensure the DPA is executed before data processing begins.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Data Categories (shown when data_processing is on) */}
          {dataProcessing && (
            <div className="space-y-2">
              <Label>Data Categories</Label>
              <div className="flex flex-wrap gap-2">
                {DATA_CATEGORY_OPTIONS.map((cat) => {
                  const selected = (form.watch('data_categories') ?? []).includes(cat);
                  return (
                    <button
                      key={cat}
                      type="button"
                      onClick={() => toggleDataCategory(cat)}
                      className={cn(
                        'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
                        selected
                          ? 'border-primary bg-primary text-primary-foreground'
                          : 'border-border bg-background text-foreground hover:bg-muted'
                      )}
                    >
                      {cat}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          <Separator />

          {/* Certifications */}
          <div className="space-y-2">
            <Label>Certifications</Label>
            <div className="flex flex-wrap gap-2">
              {CERTIFICATION_OPTIONS.map((cert) => {
                const selected = form.watch('certifications').includes(cert);
                return (
                  <button
                    key={cert}
                    type="button"
                    onClick={() => toggleCertification(cert)}
                    className={cn(
                      'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
                      selected
                        ? 'border-primary bg-primary text-primary-foreground'
                        : 'border-border bg-background text-foreground hover:bg-muted'
                    )}
                  >
                    {cert}
                  </button>
                );
              })}
            </div>
          </div>

          {/* Owner */}
          <div className="space-y-2">
            <Label htmlFor="owner_user_id">Owner User ID</Label>
            <Input id="owner_user_id" {...form.register('owner_user_id')} placeholder="User ID" />
          </div>

          {/* Submit */}
          <div className="flex gap-3 pt-4">
            <Button type="submit" disabled={createVendor.isPending} className="flex-1">
              {createVendor.isPending ? 'Onboarding...' : 'Onboard Vendor'}
            </Button>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function VendorsPage() {
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const vendors = useVendors({ page, page_size: pageSize });
  const stats = useVendorStats();

  const vendorsData = vendors.data as PaginatedResponse<Vendor> | undefined;
  const statsData = stats.data as VendorStats | undefined;

  // Filter vendors missing DPA for alert banner
  const nonCompliantVendors = (vendorsData?.items ?? []).filter(
    (v) => v.data_processing && !v.dpa_in_place
  );

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Vendor Management</h1>
          <p className="text-muted-foreground">
            Manage third-party vendors, assess risk, and ensure GDPR compliance.
          </p>
        </div>
        <OnboardVendorSheet />
      </div>

      {/* GDPR DPA Alert Banner */}
      {nonCompliantVendors.length > 0 && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 dark:border-red-800 dark:bg-red-950/40">
          <div className="flex items-start gap-3">
            <AlertCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-red-600 dark:text-red-400" />
            <div className="flex-1">
              <h3 className="font-semibold text-red-800 dark:text-red-300">
                Missing Data Processing Agreements &mdash; GDPR Article 28 Violation Risk
              </h3>
              <p className="mt-1 text-sm text-red-700 dark:text-red-400">
                GDPR Article 28 requires a Data Processing Agreement (DPA) with every vendor that
                processes personal data on your behalf. The following vendors are non-compliant:
              </p>
              <div className="mt-3 flex flex-wrap gap-2">
                {nonCompliantVendors.map((v) => (
                  <Link key={v.id} href={`/vendors/${v.id}`}>
                    <Badge variant="destructive" className="cursor-pointer hover:opacity-80">
                      {v.name}
                    </Badge>
                  </Link>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {stats.isLoading ? (
          Array.from({ length: 5 }).map((_, i) => <StatCardSkeleton key={i} />)
        ) : stats.error ? (
          <Card className="col-span-full">
            <CardContent className="flex items-center gap-2 p-6 text-destructive">
              <AlertCircle className="h-5 w-5" />
              <span>Failed to load vendor statistics.</span>
            </CardContent>
          </Card>
        ) : statsData ? (
          <>
            <StatCard
              title="Total Vendors"
              value={statsData.total}
              subtitle="All registered vendors"
              icon={Building2}
            />
            <StatCard
              title="Critical Risk"
              value={statsData.critical_risk}
              subtitle="Require close monitoring"
              icon={AlertCircle}
              className="border-l-4 border-l-red-500"
            />
            <StatCard
              title="High Risk"
              value={statsData.high_risk}
              subtitle="Elevated risk level"
              icon={AlertTriangle}
              className="border-l-4 border-l-orange-500"
            />
            <StatCard
              title="Missing DPA"
              value={statsData.missing_dpa}
              subtitle="GDPR Article 28 risk"
              icon={FileWarning}
              className="border-l-4 border-l-red-600"
            />
            <StatCard
              title="Total Contract Value"
              value={formatCurrency(statsData.total_contract_value_eur)}
              subtitle="Active contracts"
              icon={DollarSign}
            />
          </>
        ) : null}
      </div>

      {/* Data Table */}
      {vendors.isLoading ? (
        <TableSkeleton />
      ) : vendors.error ? (
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load vendors. Please try again.</span>
          </CardContent>
        </Card>
      ) : !vendorsData?.items?.length ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <Building2 className="h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">No vendors yet</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              Get started by onboarding your first vendor.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">
              Vendor Register ({vendorsData.total} total)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Name</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Country</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Risk Tier</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Data Processing</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">DPA Status</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Certifications</th>
                    <th className="pb-3 pr-4 font-medium text-muted-foreground">Next Assessment</th>
                    <th className="pb-3 font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {vendorsData.items.map((vendor) => {
                    const missingDpa = vendor.data_processing && !vendor.dpa_in_place;
                    const countryName = COUNTRIES_EU_UK.find(
                      (c) => c.code === vendor.country_code
                    )?.name ?? vendor.country_code ?? '—';

                    return (
                      <tr
                        key={vendor.id}
                        className={cn(
                          'border-b last:border-0 hover:bg-muted/50',
                          missingDpa && 'border-l-4 border-l-red-500'
                        )}
                      >
                        <td className="py-3 pr-4">
                          <Link
                            href={`/vendors/${vendor.id}`}
                            className="font-medium hover:underline"
                          >
                            {vendor.name}
                          </Link>
                        </td>
                        <td className="py-3 pr-4 text-muted-foreground">{countryName}</td>
                        <td className="py-3 pr-4">
                          {vendor.risk_tier && (
                            <Badge className={cn('capitalize', getRiskLevelColor(vendor.risk_tier))}>
                              {vendor.risk_tier}
                            </Badge>
                          )}
                        </td>
                        <td className="py-3 pr-4">
                          {vendor.data_processing ? (
                            <span className="text-yellow-600 dark:text-yellow-400">Yes</span>
                          ) : (
                            <span className="text-muted-foreground">No</span>
                          )}
                        </td>
                        <td className="py-3 pr-4">
                          {vendor.data_processing ? (
                            vendor.dpa_in_place ? (
                              <div className="flex items-center gap-1 text-green-600 dark:text-green-400">
                                <Check className="h-4 w-4" />
                                <span>In Place</span>
                              </div>
                            ) : (
                              <div className="flex items-center gap-1 text-red-600 dark:text-red-400">
                                <X className="h-4 w-4" />
                                <span className="font-medium">Missing</span>
                              </div>
                            )
                          ) : (
                            <span className="text-muted-foreground">N/A</span>
                          )}
                        </td>
                        <td className="py-3 pr-4">
                          <div className="flex flex-wrap gap-1">
                            {vendor.certifications?.length > 0
                              ? vendor.certifications.map((cert) => (
                                  <Badge key={cert} variant="outline" className="text-xs">
                                    {cert}
                                  </Badge>
                                ))
                              : <span className="text-muted-foreground">None</span>}
                          </div>
                        </td>
                        <td className="py-3 pr-4 text-muted-foreground">
                          {formatDate(vendor.next_assessment_date)}
                        </td>
                        <td className="py-3">
                          <Link href={`/vendors/${vendor.id}`}>
                            <Button variant="ghost" size="sm">
                              <ExternalLink className="h-4 w-4" />
                            </Button>
                          </Link>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {vendorsData.total_pages > 1 && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Page {vendorsData.page} of {vendorsData.total_pages}
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => p + 1)}
                    disabled={page >= vendorsData.total_pages}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
