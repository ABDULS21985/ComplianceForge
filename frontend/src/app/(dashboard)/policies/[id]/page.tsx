'use client';

import { useMemo, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import {
  ArrowLeft,
  BookOpen,
  CheckCircle2,
  Clock,
  Eye,
  FileText,
  Loader2,
  Send,
  Shield,
  Upload,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import {
  usePolicy,
  usePublishPolicy,
  useAttestPolicy,
  useUpdatePolicy,
  usePolicyAttestationStats,
} from '@/lib/api-hooks';
import {
  cn,
  formatDate,
  formatDateTime,
  getStatusColor,
} from '@/lib/utils';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PolicyData {
  id: string;
  policy_ref: string;
  title: string;
  status: string;
  classification: string;
  summary?: string;
  content_html?: string;
  version_label?: string;
  version?: number;
  owner_user_id: string;
  owner?: { id: string; first_name: string; last_name: string };
  owner_name?: string;
  approver_user_id?: string;
  approver?: { id: string; first_name: string; last_name: string };
  category_name?: string;
  category_id?: string;
  is_mandatory?: boolean;
  requires_attestation?: boolean;
  review_frequency_months?: number;
  next_review_date?: string;
  published_at?: string;
  created_at?: string;
  updated_at?: string;
  tags?: string[];
  versions?: PolicyVersion[];
  attestations?: Attestation[];
  exceptions?: PolicyException[];
}

interface PolicyVersion {
  id: string;
  version_label: string;
  change_type: string;
  change_description?: string;
  published_at?: string;
  created_by_name?: string;
}

interface Attestation {
  id: string;
  user_id: string;
  user_name?: string;
  user_email?: string;
  status: string;
  attested_at?: string;
  comment?: string;
}

interface PolicyException {
  id: string;
  title: string;
  reason?: string;
  status: string;
  requested_by_name?: string;
  approved_by_name?: string;
  expiry_date?: string;
  created_at?: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getClassificationColor(classification: string): string {
  switch (classification) {
    case 'public': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'internal': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'confidential': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    case 'restricted': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

function getChangeTypeBadge(type: string): string {
  switch (type) {
    case 'major': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'minor': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'patch': return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

// Simple sanitizer -- strips script tags. In production you would use DOMPurify.
function sanitizeHTML(html: string): string {
  return html
    .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
    .replace(/on\w+\s*=\s*["'][^"']*["']/gi, '')
    .replace(/javascript\s*:/gi, '');
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function PolicyDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;

  const { data, isLoading, error } = usePolicy(id);
  const publishPolicy = usePublishPolicy();
  const attestPolicy = useAttestPolicy();
  const updatePolicy = useUpdatePolicy();

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-muted-foreground">
        <FileText className="h-12 w-12 mb-4 text-destructive" />
        <p className="text-lg font-medium">Policy not found</p>
        <p className="text-sm mt-1">The requested policy could not be loaded.</p>
        <Button variant="outline" className="mt-4" onClick={() => router.push('/policies')}>
          <ArrowLeft className="mr-2 h-4 w-4" /> Back to Policies
        </Button>
      </div>
    );
  }

  const policy = data as PolicyData;
  const versions = policy.versions ?? [];
  const attestations = policy.attestations ?? [];
  const exceptions = policy.exceptions ?? [];

  const ownerName = policy.owner
    ? `${policy.owner.first_name} ${policy.owner.last_name}`
    : policy.owner_name ?? '---';

  // Status-based action buttons
  const statusActions = () => {
    switch (policy.status) {
      case 'draft':
        return (
          <Button
            onClick={() =>
              updatePolicy.mutate({ id: policy.id, data: { status: 'under_review' } })
            }
            disabled={updatePolicy.isPending}
          >
            {updatePolicy.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Send className="mr-2 h-4 w-4" />
            )}
            Submit for Review
          </Button>
        );
      case 'approved':
        return (
          <Button
            onClick={() => publishPolicy.mutate(policy.id)}
            disabled={publishPolicy.isPending}
          >
            {publishPolicy.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Upload className="mr-2 h-4 w-4" />
            )}
            Publish
          </Button>
        );
      case 'published':
        return (
          <Button
            onClick={() =>
              attestPolicy.mutate({ id: policy.id, data: { attested: true } })
            }
            disabled={attestPolicy.isPending}
          >
            {attestPolicy.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <CheckCircle2 className="mr-2 h-4 w-4" />
            )}
            Acknowledge
          </Button>
        );
      default:
        return null;
    }
  };

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link
        href="/policies"
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="mr-1 h-4 w-4" /> Back to Policies
      </Link>

      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-3 flex-wrap">
            <span className="font-mono text-sm text-muted-foreground">{policy.policy_ref}</span>
            <Badge className={cn(getStatusColor(policy.status), 'text-xs')}>
              {policy.status.replace(/_/g, ' ')}
            </Badge>
            <Badge className={cn(getClassificationColor(policy.classification), 'text-xs')}>
              {policy.classification}
            </Badge>
            {policy.is_mandatory && (
              <Badge variant="outline" className="text-xs">Mandatory</Badge>
            )}
          </div>
          <h1 className="text-2xl font-bold tracking-tight">{policy.title}</h1>
          {policy.summary && (
            <p className="text-sm text-muted-foreground max-w-2xl">{policy.summary}</p>
          )}
        </div>
        <div className="flex items-center gap-2">
          {statusActions()}
        </div>
      </div>

      {/* Meta info bar */}
      <div className="flex flex-wrap gap-6 text-sm text-muted-foreground border rounded-lg p-4 bg-muted/20">
        <div>
          <span className="font-medium text-foreground">Version: </span>
          {policy.version_label ?? `v${policy.version ?? 1}.0`}
        </div>
        <div>
          <span className="font-medium text-foreground">Owner: </span>
          {ownerName}
        </div>
        <div>
          <span className="font-medium text-foreground">Category: </span>
          {policy.category_name ?? policy.category_id ?? '---'}
        </div>
        <div>
          <span className="font-medium text-foreground">Review Cycle: </span>
          {policy.review_frequency_months ? `${policy.review_frequency_months} months` : '---'}
        </div>
        <div>
          <span className="font-medium text-foreground">Next Review: </span>
          {formatDate(policy.next_review_date)}
        </div>
        <div>
          <span className="font-medium text-foreground">Published: </span>
          {formatDate(policy.published_at)}
        </div>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="content">
        <TabsList>
          <TabsTrigger value="content" className="flex items-center gap-1">
            <BookOpen className="h-3.5 w-3.5" /> Content
          </TabsTrigger>
          <TabsTrigger value="versions" className="flex items-center gap-1">
            <Clock className="h-3.5 w-3.5" /> Versions ({versions.length})
          </TabsTrigger>
          <TabsTrigger value="attestations" className="flex items-center gap-1">
            <CheckCircle2 className="h-3.5 w-3.5" /> Attestations ({attestations.length})
          </TabsTrigger>
          <TabsTrigger value="exceptions" className="flex items-center gap-1">
            <Shield className="h-3.5 w-3.5" /> Exceptions ({exceptions.length})
          </TabsTrigger>
        </TabsList>

        {/* Content */}
        <TabsContent value="content">
          <Card>
            <CardContent className="p-6">
              {policy.content_html ? (
                <div
                  className="prose prose-sm dark:prose-invert max-w-none"
                  dangerouslySetInnerHTML={{
                    __html: sanitizeHTML(policy.content_html),
                  }}
                />
              ) : (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <FileText className="h-8 w-8 mb-2 opacity-40" />
                  <p className="font-medium">No content available</p>
                  <p className="text-sm mt-1">The policy content has not been written yet.</p>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Versions */}
        <TabsContent value="versions">
          <Card>
            <CardContent className="p-0">
              {versions.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <Clock className="h-8 w-8 mb-2 opacity-40" />
                  <p className="font-medium">No version history</p>
                  <p className="text-sm mt-1">Version history will appear after the first publish.</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">Version</th>
                        <th className="px-4 py-3 text-left font-medium">Change Type</th>
                        <th className="px-4 py-3 text-left font-medium">Published At</th>
                        <th className="px-4 py-3 text-left font-medium">Description</th>
                        <th className="px-4 py-3 text-left font-medium">Author</th>
                      </tr>
                    </thead>
                    <tbody>
                      {versions.map((v) => (
                        <tr key={v.id} className="border-b last:border-b-0">
                          <td className="px-4 py-3 font-mono font-medium">{v.version_label}</td>
                          <td className="px-4 py-3">
                            <Badge className={cn(getChangeTypeBadge(v.change_type), 'text-xs')}>
                              {v.change_type}
                            </Badge>
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">
                            {formatDateTime(v.published_at)}
                          </td>
                          <td className="px-4 py-3 max-w-xs truncate">
                            {v.change_description ?? '---'}
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">
                            {v.created_by_name ?? '---'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Attestations */}
        <TabsContent value="attestations">
          <Card>
            <CardContent className="p-0">
              {attestations.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <CheckCircle2 className="h-8 w-8 mb-2 opacity-40" />
                  <p className="font-medium">No attestation records</p>
                  <p className="text-sm mt-1">
                    {policy.requires_attestation
                      ? 'Users have not yet attested to this policy.'
                      : 'This policy does not require attestation.'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">User</th>
                        <th className="px-4 py-3 text-left font-medium">Status</th>
                        <th className="px-4 py-3 text-left font-medium">Attested At</th>
                        <th className="px-4 py-3 text-left font-medium">Comment</th>
                      </tr>
                    </thead>
                    <tbody>
                      {attestations.map((a) => (
                        <tr key={a.id} className="border-b last:border-b-0">
                          <td className="px-4 py-3">
                            <div>
                              <span className="font-medium">{a.user_name ?? '---'}</span>
                              {a.user_email && (
                                <span className="block text-xs text-muted-foreground">{a.user_email}</span>
                              )}
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <Badge className={cn(getStatusColor(a.status), 'text-xs')}>
                              {a.status.replace(/_/g, ' ')}
                            </Badge>
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">
                            {formatDateTime(a.attested_at)}
                          </td>
                          <td className="px-4 py-3 text-muted-foreground max-w-xs truncate">
                            {a.comment ?? '---'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Exceptions */}
        <TabsContent value="exceptions">
          <Card>
            <CardContent className="p-0">
              {exceptions.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                  <Shield className="h-8 w-8 mb-2 opacity-40" />
                  <p className="font-medium">No exceptions</p>
                  <p className="text-sm mt-1">No policy exceptions have been requested.</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">Title</th>
                        <th className="px-4 py-3 text-left font-medium">Status</th>
                        <th className="px-4 py-3 text-left font-medium">Requested By</th>
                        <th className="px-4 py-3 text-left font-medium">Approved By</th>
                        <th className="px-4 py-3 text-left font-medium">Expiry Date</th>
                        <th className="px-4 py-3 text-left font-medium">Reason</th>
                      </tr>
                    </thead>
                    <tbody>
                      {exceptions.map((ex) => {
                        const expired =
                          ex.expiry_date && new Date(ex.expiry_date) < new Date();
                        return (
                          <tr key={ex.id} className="border-b last:border-b-0">
                            <td className="px-4 py-3 font-medium">{ex.title}</td>
                            <td className="px-4 py-3">
                              <Badge className={cn(getStatusColor(ex.status), 'text-xs')}>
                                {ex.status.replace(/_/g, ' ')}
                              </Badge>
                            </td>
                            <td className="px-4 py-3 text-muted-foreground">
                              {ex.requested_by_name ?? '---'}
                            </td>
                            <td className="px-4 py-3 text-muted-foreground">
                              {ex.approved_by_name ?? '---'}
                            </td>
                            <td className="px-4 py-3">
                              <span className={cn(expired && 'text-red-500 font-medium')}>
                                {formatDate(ex.expiry_date)}
                              </span>
                              {expired && (
                                <Badge className="ml-1 bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400 text-[10px]">
                                  Expired
                                </Badge>
                              )}
                            </td>
                            <td className="px-4 py-3 text-muted-foreground max-w-xs truncate">
                              {ex.reason ?? '---'}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
