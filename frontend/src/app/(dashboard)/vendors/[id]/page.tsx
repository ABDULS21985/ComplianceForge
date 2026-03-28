'use client';

import { useParams } from 'next/navigation';
import Link from 'next/link';
import {
  ArrowLeft,
  Building2,
  AlertCircle,
  AlertTriangle,
  Globe,
  Mail,
  User,
  Calendar,
  Shield,
  FileText,
  ExternalLink,
  Edit,
} from 'lucide-react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';

import { cn } from '@/lib/utils';
import { formatDate, formatCurrency, getRiskLevelColor, getStatusColor } from '@/lib/utils';
import { COUNTRIES_EU_UK } from '@/lib/constants';
import { useVendor } from '@/lib/api-hooks';
import type { Vendor } from '@/types';

// ---------------------------------------------------------------------------
// Skeleton
// ---------------------------------------------------------------------------

function VendorDetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="animate-pulse space-y-4">
        <div className="h-8 w-64 rounded bg-muted" />
        <div className="h-4 w-40 rounded bg-muted" />
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardContent className="p-6">
              <div className="animate-pulse space-y-3">
                <div className="h-5 w-32 rounded bg-muted" />
                <div className="h-4 w-full rounded bg-muted" />
                <div className="h-4 w-3/4 rounded bg-muted" />
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Info Row Helper
// ---------------------------------------------------------------------------

function InfoRow({ label, value, icon: Icon }: { label: string; value: React.ReactNode; icon?: React.ElementType }) {
  return (
    <div className="flex items-start gap-3 py-2">
      {Icon && <Icon className="mt-0.5 h-4 w-4 flex-shrink-0 text-muted-foreground" />}
      <div className="flex-1">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p className="mt-0.5 text-sm">{value || '—'}</p>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function VendorDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const vendorQuery = useVendor(id);
  const vendor = vendorQuery.data as Vendor | undefined;

  if (vendorQuery.isLoading) {
    return <VendorDetailSkeleton />;
  }

  if (vendorQuery.error) {
    return (
      <div className="space-y-6">
        <Link href="/vendors">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Vendors
          </Button>
        </Link>
        <Card>
          <CardContent className="flex items-center gap-2 p-6 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <span>Failed to load vendor details. Please try again.</span>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!vendor) {
    return (
      <div className="space-y-6">
        <Link href="/vendors">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Vendors
          </Button>
        </Link>
        <Card>
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <Building2 className="h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">Vendor not found</h3>
          </CardContent>
        </Card>
      </div>
    );
  }

  const countryName = COUNTRIES_EU_UK.find((c) => c.code === vendor.country_code)?.name ?? vendor.country_code;
  const missingDpa = vendor.data_processing && !vendor.dpa_in_place;

  return (
    <div className="space-y-6">
      {/* Navigation */}
      <Link href="/vendors">
        <Button variant="ghost" size="sm">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Vendors
        </Button>
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-bold tracking-tight">{vendor.name}</h1>
            {vendor.risk_tier && (
              <Badge className={cn('capitalize', getRiskLevelColor(vendor.risk_tier))}>
                {vendor.risk_tier} risk
              </Badge>
            )}
            <Badge className={getStatusColor(vendor.status)}>
              {vendor.status.replace(/_/g, ' ')}
            </Badge>
          </div>
          {countryName && (
            <p className="flex items-center gap-1 text-muted-foreground">
              <Globe className="h-4 w-4" />
              {countryName}
            </p>
          )}
        </div>
        <Link href={`/vendors/${vendor.id}/edit`}>
          <Button variant="outline">
            <Edit className="mr-2 h-4 w-4" />
            Edit Vendor
          </Button>
        </Link>
      </div>

      {/* DPA Warning */}
      {missingDpa && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 dark:border-red-800 dark:bg-red-950/40">
          <div className="flex items-start gap-3">
            <AlertCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-red-600 dark:text-red-400" />
            <div>
              <h3 className="font-semibold text-red-800 dark:text-red-300">
                Data Processing Agreement Required
              </h3>
              <p className="mt-1 text-sm text-red-700 dark:text-red-400">
                This vendor processes personal data but does not have a DPA in place.
                GDPR Article 28 requires a written contract (DPA) with every data processor.
                This must be resolved immediately to avoid regulatory non-compliance.
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Detail Sections */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Overview */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Overview</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1">
            <InfoRow label="Vendor Reference" value={vendor.vendor_ref} icon={FileText} />
            <InfoRow label="Legal Name" value={vendor.legal_name} icon={Building2} />
            <InfoRow
              label="Website"
              value={
                vendor.website ? (
                  <a
                    href={vendor.website}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-1 text-primary hover:underline"
                  >
                    {vendor.website}
                    <ExternalLink className="h-3 w-3" />
                  </a>
                ) : (
                  '—'
                )
              }
              icon={Globe}
            />
            <InfoRow label="Contact Name" value={vendor.contact_name} icon={User} />
            <InfoRow label="Contact Email" value={vendor.contact_email} icon={Mail} />
            <InfoRow label="Service Description" value={vendor.service_description} />
            <InfoRow
              label="Owner"
              value={
                vendor.owner
                  ? `${vendor.owner.first_name} ${vendor.owner.last_name}`
                  : vendor.owner_user_id ?? '—'
              }
              icon={User}
            />
          </CardContent>
        </Card>

        {/* Risk Assessment */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Risk Assessment</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1">
            <InfoRow
              label="Risk Tier"
              value={
                vendor.risk_tier ? (
                  <Badge className={cn('capitalize', getRiskLevelColor(vendor.risk_tier))}>
                    {vendor.risk_tier}
                  </Badge>
                ) : (
                  '—'
                )
              }
              icon={AlertTriangle}
            />
            <InfoRow label="Risk Score" value={vendor.risk_score ?? '—'} icon={Shield} />
            <InfoRow label="Assessment Frequency" value={vendor.assessment_frequency} icon={Calendar} />
            <InfoRow label="Last Assessment" value={formatDate(vendor.last_assessment_date)} icon={Calendar} />
            <InfoRow label="Next Assessment" value={formatDate(vendor.next_assessment_date)} icon={Calendar} />
          </CardContent>
        </Card>

        {/* Certifications */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Certifications</CardTitle>
          </CardHeader>
          <CardContent>
            {vendor.certifications?.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {vendor.certifications.map((cert) => (
                  <Badge key={cert} variant="outline" className="px-3 py-1">
                    <Shield className="mr-1 h-3 w-3" />
                    {cert}
                  </Badge>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                No certifications recorded. Consider requesting SOC 2 or ISO 27001 documentation.
              </p>
            )}
          </CardContent>
        </Card>

        {/* Data Processing */}
        <Card className={missingDpa ? 'border-red-300 dark:border-red-800' : ''}>
          <CardHeader>
            <CardTitle className="text-lg">Data Processing</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1">
            <InfoRow
              label="Processes Personal Data"
              value={
                vendor.data_processing ? (
                  <span className="font-medium text-yellow-600 dark:text-yellow-400">Yes</span>
                ) : (
                  'No'
                )
              }
            />
            {vendor.data_processing && (
              <>
                <InfoRow
                  label="DPA Status"
                  value={
                    vendor.dpa_in_place ? (
                      <span className="font-medium text-green-600 dark:text-green-400">In Place</span>
                    ) : (
                      <span className="font-medium text-red-600 dark:text-red-400">Missing</span>
                    )
                  }
                />
                {vendor.dpa_signed_date && (
                  <InfoRow label="DPA Signed Date" value={formatDate(vendor.dpa_signed_date)} icon={Calendar} />
                )}
                {vendor.data_categories && vendor.data_categories.length > 0 && (
                  <div className="py-2">
                    <p className="text-xs font-medium text-muted-foreground">Data Categories</p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {vendor.data_categories.map((cat) => (
                        <Badge key={cat} variant="secondary" className="text-xs">
                          {cat}
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* Contract Info */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-lg">Contract Information</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <div>
                <p className="text-xs font-medium text-muted-foreground">Contract Start</p>
                <p className="mt-1 text-sm">{formatDate(vendor.contract_start_date)}</p>
              </div>
              <div>
                <p className="text-xs font-medium text-muted-foreground">Contract End</p>
                <p className="mt-1 text-sm">{formatDate(vendor.contract_end_date)}</p>
              </div>
              <div>
                <p className="text-xs font-medium text-muted-foreground">Contract Value</p>
                <p className="mt-1 text-sm font-semibold">
                  {formatCurrency(vendor.contract_value_eur)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
