'use client';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { formatApiError, isUuid, parseCommaList, parseJSONObject } from '@/lib/enterprise-settings';
import { type FormEvent, useState } from 'react';
import { KeyRound, Loader2, LockKeyhole, ShieldCheck } from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type {
  SSOConfiguration,
  SSOProtocol,
  UpdateSSOConfigurationInput,
} from '@/types/enterprise-settings';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

interface SSODraft {
  protocol: SSOProtocol;
  enabled: boolean;
  enforced: boolean;
  samlEntityId: string;
  samlSsoUrl: string;
  samlSloUrl: string;
  samlCertificate: string;
  samlNameIdFormat: string;
  samlAttributeMapping: string;
  oidcIssuerUrl: string;
  oidcClientId: string;
  oidcClientSecret: string;
  oidcScopes: string;
  oidcClaimMapping: string;
  secretConfigured: boolean;
  autoProvision: boolean;
  jitProvisioning: boolean;
  defaultRoleId: string;
  allowedDomains: string;
  groupToRoleMapping: string;
}

function toDraft(config: SSOConfiguration): SSODraft {
  return {
    protocol: config.protocol || 'saml2',
    enabled: config.is_enabled,
    enforced: config.is_enforced,
    samlEntityId: config.saml_entity_id ?? '',
    samlSsoUrl: config.saml_sso_url ?? '',
    samlSloUrl: config.saml_slo_url ?? '',
    samlCertificate: config.saml_certificate ?? '',
    samlNameIdFormat: config.saml_name_id_format ?? '',
    samlAttributeMapping: JSON.stringify(config.saml_attribute_mapping ?? {}, null, 2),
    oidcIssuerUrl: config.oidc_issuer_url ?? '',
    oidcClientId: config.oidc_client_id ?? '',
    oidcClientSecret: '',
    oidcScopes: (config.oidc_scopes?.length
      ? config.oidc_scopes
      : ['openid', 'profile', 'email']
    ).join(', '),
    oidcClaimMapping: JSON.stringify(config.oidc_claim_mapping ?? {}, null, 2),
    secretConfigured: config.oidc_client_secret_configured,
    autoProvision: config.auto_provision_users,
    jitProvisioning: config.jit_provisioning,
    defaultRoleId: config.default_role_id ?? '',
    allowedDomains: (config.allowed_domains ?? []).join(', '),
    groupToRoleMapping: JSON.stringify(config.group_to_role_mapping ?? {}, null, 2),
  };
}

function secureURL(value: string): boolean {
  try {
    const url = new URL(value);
    return (
      url.protocol === 'https:' ||
      (url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname))
    );
  } catch {
    return false;
  }
}

export function SSOPanel({ canConfigure }: { canConfigure: boolean }) {
  const queryClient = useQueryClient();
  const [draftOverride, setDraftOverride] = useState<SSODraft | null>(null);
  const [validationError, setValidationError] = useState('');
  const configQuery = useQuery({
    queryKey: ['integrations', 'sso'],
    queryFn: () => api.integrations.getSSOConfig(),
    staleTime: 30_000,
  });

  const serverDraft = configQuery.data?.data ? toDraft(configQuery.data.data) : null;
  const draft = draftOverride ?? serverDraft;

  function updateDraft(update: (current: SSODraft) => SSODraft) {
    setDraftOverride((current) => {
      const source = current ?? serverDraft;
      return source ? update(source) : current;
    });
  }

  const updateMutation = useMutation({
    mutationFn: (input: UpdateSSOConfigurationInput) => api.integrations.updateSSOConfig(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['integrations', 'sso'] });
      updateDraft((current) => ({
        ...current,
        oidcClientSecret: '',
        secretConfigured:
          current.protocol === 'oidc'
            ? current.secretConfigured || Boolean(current.oidcClientSecret)
            : false,
      }));
      toast.success('SSO configuration saved');
    },
    onError: (error) => toast.error(formatApiError(error, 'SSO configuration could not be saved.')),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft) return;
    setValidationError('');
    if (draft.enforced && !draft.enabled)
      return setValidationError('Enforced SSO must also be enabled.');
    if (draft.enabled && draft.protocol === 'saml2') {
      if (!draft.samlEntityId.trim() || !draft.samlSsoUrl.trim() || !draft.samlCertificate.trim())
        return setValidationError(
          'Enabled SAML requires an entity ID, SSO URL, and PEM certificate.',
        );
      if (
        !secureURL(draft.samlSsoUrl.trim()) ||
        (draft.samlSloUrl.trim() && !secureURL(draft.samlSloUrl.trim()))
      )
        return setValidationError(
          'SAML endpoints must use HTTPS (loopback HTTP is allowed for development).',
        );
      if (!draft.samlCertificate.includes('-----BEGIN CERTIFICATE-----'))
        return setValidationError('SAML certificate must be PEM encoded.');
    }
    const scopes = parseCommaList(draft.oidcScopes);
    if (draft.protocol === 'oidc' && !scopes.includes('openid'))
      return setValidationError('OIDC scopes must include openid.');
    if (draft.enabled && draft.protocol === 'oidc') {
      if (
        !draft.oidcIssuerUrl.trim() ||
        !draft.oidcClientId.trim() ||
        (!draft.secretConfigured && !draft.oidcClientSecret.trim())
      )
        return setValidationError(
          'Enabled OIDC requires an issuer URL, client ID, and client secret.',
        );
      if (!secureURL(draft.oidcIssuerUrl.trim()))
        return setValidationError(
          'OIDC issuer URL must use HTTPS (loopback HTTP is allowed for development).',
        );
    }
    if (draft.defaultRoleId.trim() && !isUuid(draft.defaultRoleId.trim()))
      return setValidationError('Default role ID must be a valid UUID.');

    let samlAttributes: Record<string, unknown>;
    let oidcClaims: Record<string, unknown>;
    let groupRoles: Record<string, unknown>;
    try {
      samlAttributes = parseJSONObject(draft.samlAttributeMapping, 'SAML attribute mapping');
      oidcClaims = parseJSONObject(draft.oidcClaimMapping, 'OIDC claim mapping');
      groupRoles = parseJSONObject(draft.groupToRoleMapping, 'Group-to-role mapping');
    } catch (error) {
      return setValidationError(
        error instanceof Error ? error.message : 'Mappings must be valid JSON objects.',
      );
    }

    const input: UpdateSSOConfigurationInput = {
      protocol: draft.protocol,
      is_enabled: draft.enabled,
      is_enforced: draft.enforced,
      auto_provision_users: draft.autoProvision,
      jit_provisioning: draft.jitProvisioning,
      allowed_domains: parseCommaList(draft.allowedDomains).map((domain) => domain.toLowerCase()),
      group_to_role_mapping: groupRoles,
      default_role_id: draft.defaultRoleId.trim() || undefined,
    };
    if (draft.protocol === 'saml2') {
      input.saml_entity_id = draft.samlEntityId.trim() || undefined;
      input.saml_sso_url = draft.samlSsoUrl.trim() || undefined;
      input.saml_slo_url = draft.samlSloUrl.trim() || undefined;
      input.saml_certificate = draft.samlCertificate.trim() || undefined;
      input.saml_name_id_format = draft.samlNameIdFormat.trim() || undefined;
      input.saml_attribute_mapping = samlAttributes;
    } else {
      input.oidc_issuer_url = draft.oidcIssuerUrl.trim() || undefined;
      input.oidc_client_id = draft.oidcClientId.trim() || undefined;
      input.oidc_scopes = scopes;
      input.oidc_claim_mapping = oidcClaims;
      if (draft.oidcClientSecret.trim()) input.oidc_client_secret = draft.oidcClientSecret;
    }
    updateMutation.mutate(input);
  }

  if (configQuery.isError)
    return (
      <Card>
        <CardContent className="space-y-3 py-8 text-center">
          <p role="alert">
            {formatApiError(configQuery.error, 'SSO configuration could not be loaded.')}
          </p>
          <Button type="button" variant="outline" onClick={() => void configQuery.refetch()}>
            Try again
          </Button>
        </CardContent>
      </Card>
    );
  if (configQuery.isLoading || !draft)
    return (
      <div role="status" aria-label="Loading SSO configuration" className="space-y-4">
        <Skeleton className="h-28" />
        <Skeleton className="h-96" />
      </div>
    );

  return (
    <form onSubmit={submit} className="space-y-5">
      <div>
        <h2 className="text-xl font-semibold">Single sign-on</h2>
        <p className="text-sm text-muted-foreground">
          Configure one tenant identity protocol. Secret values are write-only.
        </p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <LockKeyhole aria-hidden="true" className="h-5 w-5" />
            Access policy
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="sso-protocol">Protocol</Label>
            <Select
              disabled={!canConfigure || updateMutation.isPending}
              value={draft.protocol}
              onValueChange={(protocol: SSOProtocol) =>
                updateDraft((current) => ({ ...current, protocol, enforced: false }))
              }
            >
              <SelectTrigger id="sso-protocol">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="saml2">SAML 2.0</SelectItem>
                <SelectItem value="oidc">OpenID Connect</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center justify-between gap-3 rounded-md border p-3">
            <div>
              <Label htmlFor="sso-enabled">Enabled</Label>
              <p className="text-xs text-muted-foreground">Allow SSO sign-in.</p>
            </div>
            <Switch
              id="sso-enabled"
              disabled={!canConfigure || updateMutation.isPending}
              checked={draft.enabled}
              onCheckedChange={(enabled) =>
                updateDraft((current) => ({
                  ...current,
                  enabled,
                  enforced: enabled ? current.enforced : false,
                }))
              }
            />
          </div>
          <div className="flex items-center justify-between gap-3 rounded-md border p-3">
            <div>
              <Label htmlFor="sso-enforced">Enforced</Label>
              <p className="text-xs text-muted-foreground">Require SSO for users.</p>
            </div>
            <Switch
              id="sso-enforced"
              disabled={!canConfigure || !draft.enabled || updateMutation.isPending}
              checked={draft.enforced}
              onCheckedChange={(enforced) => updateDraft((current) => ({ ...current, enforced }))}
            />
          </div>
        </CardContent>
      </Card>

      {draft.protocol === 'saml2' ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">SAML service provider</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="saml-entity">Entity ID</Label>
              <Input
                id="saml-entity"
                disabled={!canConfigure}
                value={draft.samlEntityId}
                onChange={(event) =>
                  updateDraft((current) => ({ ...current, samlEntityId: event.target.value }))
                }
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="saml-sso-url">SSO URL</Label>
                <Input
                  id="saml-sso-url"
                  type="url"
                  disabled={!canConfigure}
                  value={draft.samlSsoUrl}
                  onChange={(event) =>
                    updateDraft((current) => ({ ...current, samlSsoUrl: event.target.value }))
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="saml-slo-url">SLO URL (optional)</Label>
                <Input
                  id="saml-slo-url"
                  type="url"
                  disabled={!canConfigure}
                  value={draft.samlSloUrl}
                  onChange={(event) =>
                    updateDraft((current) => ({ ...current, samlSloUrl: event.target.value }))
                  }
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="saml-name-id">Name ID format (optional)</Label>
              <Input
                id="saml-name-id"
                disabled={!canConfigure}
                value={draft.samlNameIdFormat}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    samlNameIdFormat: event.target.value,
                  }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="saml-certificate">Signing certificate (PEM)</Label>
              <Textarea
                id="saml-certificate"
                rows={8}
                className="font-mono text-xs"
                disabled={!canConfigure}
                value={draft.samlCertificate}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    samlCertificate: event.target.value,
                  }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="saml-mapping">Attribute mapping JSON</Label>
              <Textarea
                id="saml-mapping"
                rows={5}
                className="font-mono text-xs"
                disabled={!canConfigure}
                value={draft.samlAttributeMapping}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    samlAttributeMapping: event.target.value,
                  }))
                }
              />
            </div>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <KeyRound aria-hidden="true" className="h-5 w-5" />
              OpenID Connect client
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="oidc-issuer">Issuer URL</Label>
              <Input
                id="oidc-issuer"
                type="url"
                disabled={!canConfigure}
                value={draft.oidcIssuerUrl}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    oidcIssuerUrl: event.target.value,
                  }))
                }
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="oidc-client">Client ID</Label>
                <Input
                  id="oidc-client"
                  disabled={!canConfigure}
                  value={draft.oidcClientId}
                  onChange={(event) =>
                    updateDraft((current) => ({
                      ...current,
                      oidcClientId: event.target.value,
                    }))
                  }
                />
              </div>
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="oidc-secret">New client secret</Label>
                  {draft.secretConfigured && <Badge variant="secondary">Secret configured</Badge>}
                </div>
                <Input
                  id="oidc-secret"
                  type="password"
                  autoComplete="new-password"
                  disabled={!canConfigure}
                  value={draft.oidcClientSecret}
                  onChange={(event) =>
                    updateDraft((current) => ({
                      ...current,
                      oidcClientSecret: event.target.value,
                    }))
                  }
                  placeholder={
                    draft.secretConfigured ? 'Leave blank to preserve' : 'Required before enabling'
                  }
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="oidc-scopes">Scopes</Label>
              <Input
                id="oidc-scopes"
                disabled={!canConfigure}
                value={draft.oidcScopes}
                onChange={(event) =>
                  updateDraft((current) => ({ ...current, oidcScopes: event.target.value }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="oidc-mapping">Claim mapping JSON</Label>
              <Textarea
                id="oidc-mapping"
                rows={5}
                className="font-mono text-xs"
                disabled={!canConfigure}
                value={draft.oidcClaimMapping}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    oidcClaimMapping: event.target.value,
                  }))
                }
              />
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <ShieldCheck aria-hidden="true" className="h-5 w-5" />
            Provisioning
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex items-center justify-between gap-3 rounded-md border p-3">
              <div>
                <Label htmlFor="auto-provision">Auto-provision users</Label>
                <p className="text-xs text-muted-foreground">Create eligible users on sign-in.</p>
              </div>
              <Switch
                id="auto-provision"
                disabled={!canConfigure}
                checked={draft.autoProvision}
                onCheckedChange={(autoProvision) =>
                  updateDraft((current) => ({ ...current, autoProvision }))
                }
              />
            </div>
            <div className="flex items-center justify-between gap-3 rounded-md border p-3">
              <div>
                <Label htmlFor="jit-provision">JIT provisioning</Label>
                <p className="text-xs text-muted-foreground">Provision at authentication time.</p>
              </div>
              <Switch
                id="jit-provision"
                disabled={!canConfigure}
                checked={draft.jitProvisioning}
                onCheckedChange={(jitProvisioning) =>
                  updateDraft((current) => ({ ...current, jitProvisioning }))
                }
              />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="default-role">Default role UUID (optional)</Label>
              <Input
                id="default-role"
                disabled={!canConfigure}
                value={draft.defaultRoleId}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    defaultRoleId: event.target.value,
                  }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="allowed-domains">Allowed email domains</Label>
              <Input
                id="allowed-domains"
                disabled={!canConfigure}
                value={draft.allowedDomains}
                onChange={(event) =>
                  updateDraft((current) => ({
                    ...current,
                    allowedDomains: event.target.value,
                  }))
                }
                placeholder="example.com, subsidiary.example"
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="group-role-mapping">Group-to-role mapping JSON</Label>
            <Textarea
              id="group-role-mapping"
              rows={5}
              className="font-mono text-xs"
              disabled={!canConfigure}
              value={draft.groupToRoleMapping}
              onChange={(event) =>
                updateDraft((current) => ({
                  ...current,
                  groupToRoleMapping: event.target.value,
                }))
              }
            />
          </div>
          <p className="text-xs text-muted-foreground">
            Blank optional scalar fields preserve existing values; the current API does not provide
            an explicit clear operation.
          </p>
        </CardContent>
      </Card>
      {validationError && (
        <p role="alert" className="text-sm text-destructive">
          {validationError}
        </p>
      )}
      {canConfigure && (
        <div className="flex justify-end">
          <Button type="submit" disabled={updateMutation.isPending}>
            {updateMutation.isPending && (
              <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />
            )}
            Save SSO configuration
          </Button>
        </div>
      )}
    </form>
  );
}
