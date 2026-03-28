'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Integration {
  id: string;
  type: string;
  name: string;
  status: 'active' | 'degraded' | 'error' | 'inactive';
  last_sync_at: string | null;
  config: Record<string, any>;
}

interface APIKey {
  id: string;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
}

const INTEGRATION_TYPES = [
  { type: 'sso_saml', name: 'SSO (SAML)', icon: '🔐' },
  { type: 'sso_oidc', name: 'SSO (OIDC)', icon: '🔑' },
  { type: 'aws', name: 'AWS', icon: '☁️' },
  { type: 'azure', name: 'Azure', icon: '🌐' },
  { type: 'gcp', name: 'Google Cloud', icon: '🔷' },
  { type: 'splunk', name: 'Splunk', icon: '📊' },
  { type: 'elastic', name: 'Elastic', icon: '🔍' },
  { type: 'servicenow', name: 'ServiceNow', icon: '🎫' },
  { type: 'jira', name: 'Jira', icon: '📋' },
  { type: 'webhook', name: 'Webhook', icon: '🔗' },
] as const;

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function IntegrationsPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'marketplace' | 'active' | 'api-keys' | 'sso'>('marketplace');
  const [configDialog, setConfigDialog] = useState<{ open: boolean; type: string; name: string } | null>(null);
  const [configFields, setConfigFields] = useState<Record<string, string>>({});
  const [newKeyDialog, setNewKeyDialog] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [ssoProtocol, setSsoProtocol] = useState<'saml' | 'oidc'>('saml');
  const [ssoConfig, setSsoConfig] = useState<Record<string, string>>({});

  // Queries
  const { data: integrations, isLoading, error } = useQuery({
    queryKey: ['integrations'],
    queryFn: () => api.integrations.list(),
  });

  const { data: apiKeys } = useQuery({
    queryKey: ['api-keys'],
    queryFn: () => api.integrations.listAPIKeys(),
    enabled: activeTab === 'api-keys',
  });

  const { data: ssoData } = useQuery({
    queryKey: ['sso-config'],
    queryFn: () => api.integrations.getSSOConfig(),
    enabled: activeTab === 'sso',
  });

  // Mutations
  const createIntegration = useMutation({
    mutationFn: (data: any) => api.integrations.create(data),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['integrations'] }); setConfigDialog(null); setConfigFields({}); },
  });

  const syncIntegration = useMutation({
    mutationFn: (id: string) => api.integrations.sync(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['integrations'] }),
  });

  const testIntegration = useMutation({
    mutationFn: (id: string) => api.integrations.test(id),
  });

  const createAPIKeyMutation = useMutation({
    mutationFn: (data: any) => api.integrations.createAPIKey(data),
    onSuccess: (data: any) => {
      setCreatedKey(data?.key ?? data?.api_key ?? 'Key created');
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
      setNewKeyName('');
    },
  });

  const revokeAPIKeyMutation = useMutation({
    mutationFn: (id: string) => api.integrations.revokeAPIKey(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['api-keys'] }),
  });

  const updateSSOMutation = useMutation({
    mutationFn: (data: any) => api.integrations.updateSSOConfig(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sso-config'] }),
  });

  const integrationList: Integration[] = integrations?.items ?? integrations ?? [];
  const activeIntegrations = integrationList.filter((i) => i.status !== 'inactive');
  const apiKeyList: APIKey[] = apiKeys?.items ?? apiKeys ?? [];

  // Status helpers
  function statusBadge(status: string) {
    const map: Record<string, string> = {
      active: 'bg-green-100 text-green-700',
      degraded: 'bg-amber-100 text-amber-700',
      error: 'bg-red-100 text-red-700',
      inactive: 'bg-gray-100 text-gray-500',
    };
    return map[status] ?? map.inactive;
  }

  function statusDot(status: string) {
    const map: Record<string, string> = {
      active: 'bg-green-500',
      degraded: 'bg-amber-500',
      error: 'bg-red-500',
      inactive: 'bg-gray-400',
    };
    return map[status] ?? map.inactive;
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <h1 className="text-2xl font-bold">Integration Hub</h1>
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
          {Array.from({ length: 10 }).map((_, i) => (
            <div key={i} className="h-40 rounded-lg bg-gray-100 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold mb-4">Integration Hub</h1>
        <div className="bg-red-50 text-red-700 rounded-lg p-4">Failed to load integrations.</div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Integration Hub</h1>

      {/* Tabs */}
      <div className="flex gap-1 border-b">
        {(['marketplace', 'active', 'api-keys', 'sso'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
              activeTab === tab
                ? 'border-blue-600 text-blue-600'
                : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            {tab === 'marketplace' ? 'Marketplace' : tab === 'active' ? 'Active' : tab === 'api-keys' ? 'API Keys' : 'SSO Config'}
          </button>
        ))}
      </div>

      {/* Marketplace Tab */}
      {activeTab === 'marketplace' && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
          {INTEGRATION_TYPES.map((it) => {
            const existing = integrationList.find((i) => i.type === it.type);
            return (
              <div key={it.type} className="border rounded-lg p-4 bg-white flex flex-col items-center text-center gap-2">
                <span className="text-3xl">{it.icon}</span>
                <p className="font-medium text-sm">{it.name}</p>
                {existing ? (
                  <div className="flex items-center gap-1.5">
                    <span className={`w-2 h-2 rounded-full ${statusDot(existing.status)}`} />
                    <span className="text-xs capitalize">{existing.status}</span>
                  </div>
                ) : (
                  <span className="text-xs text-gray-400">Not configured</span>
                )}
                {existing?.last_sync_at && (
                  <span className="text-xs text-gray-400">
                    Last sync: {new Date(existing.last_sync_at).toLocaleDateString()}
                  </span>
                )}
                <button
                  onClick={() => {
                    setConfigDialog({ open: true, type: it.type, name: it.name });
                    setConfigFields({});
                  }}
                  className="mt-auto px-3 py-1.5 text-xs font-medium rounded border border-gray-300 hover:bg-gray-50 w-full"
                >
                  Configure
                </button>
              </div>
            );
          })}
        </div>
      )}

      {/* Active Integrations Tab */}
      {activeTab === 'active' && (
        <div className="space-y-3">
          {activeIntegrations.length === 0 && (
            <p className="text-center text-gray-500 py-12">No active integrations. Configure one from the Marketplace tab.</p>
          )}
          {activeIntegrations.map((item) => (
            <div key={item.id} className="border rounded-lg p-4 bg-white flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className={`w-3 h-3 rounded-full ${statusDot(item.status)}`} />
                <div>
                  <p className="font-medium">{item.name}</p>
                  <p className="text-xs text-gray-500 capitalize">{item.type.replace(/_/g, ' ')}</p>
                </div>
              </div>
              <div className="flex items-center gap-3">
                <span className={`text-xs px-2 py-1 rounded-full font-medium ${statusBadge(item.status)}`}>
                  {item.status}
                </span>
                {item.last_sync_at && (
                  <span className="text-xs text-gray-400">
                    Synced {new Date(item.last_sync_at).toLocaleString()}
                  </span>
                )}
                <button
                  onClick={() => syncIntegration.mutate(item.id)}
                  disabled={syncIntegration.isPending}
                  className="px-3 py-1 text-xs font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                >
                  Sync
                </button>
                <button
                  onClick={() => testIntegration.mutate(item.id)}
                  disabled={testIntegration.isPending}
                  className="px-3 py-1 text-xs font-medium rounded border border-gray-300 hover:bg-gray-50"
                >
                  Test
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* API Keys Tab */}
      {activeTab === 'api-keys' && (
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <p className="text-sm text-gray-500">Manage API keys for programmatic access.</p>
            <button
              onClick={() => { setNewKeyDialog(true); setCreatedKey(null); }}
              className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
            >
              Create API Key
            </button>
          </div>

          {/* Created key display */}
          {createdKey && (
            <div className="bg-green-50 border border-green-200 rounded-lg p-4">
              <p className="text-sm font-medium text-green-800 mb-1">API Key created successfully. Copy it now — it will not be shown again.</p>
              <code className="block bg-white border rounded p-2 text-sm font-mono break-all">{createdKey}</code>
            </div>
          )}

          {/* New key dialog */}
          {newKeyDialog && !createdKey && (
            <div className="border rounded-lg p-4 bg-gray-50 space-y-3">
              <label className="block text-sm font-medium">Key Name</label>
              <input
                type="text"
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
                className="w-full border rounded px-3 py-2 text-sm"
                placeholder="e.g. CI/CD Pipeline"
              />
              <div className="flex gap-2">
                <button
                  onClick={() => createAPIKeyMutation.mutate({ name: newKeyName })}
                  disabled={!newKeyName.trim() || createAPIKeyMutation.isPending}
                  className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                >
                  {createAPIKeyMutation.isPending ? 'Creating...' : 'Create'}
                </button>
                <button
                  onClick={() => setNewKeyDialog(false)}
                  className="px-4 py-2 text-sm font-medium rounded border border-gray-300 hover:bg-gray-50"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          {/* Key list */}
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-2 font-medium">Name</th>
                  <th className="text-left px-4 py-2 font-medium">Prefix</th>
                  <th className="text-left px-4 py-2 font-medium">Created</th>
                  <th className="text-left px-4 py-2 font-medium">Last Used</th>
                  <th className="text-left px-4 py-2 font-medium">Expires</th>
                  <th className="text-right px-4 py-2 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {apiKeyList.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-4 py-8 text-center text-gray-400">No API keys created yet.</td>
                  </tr>
                )}
                {apiKeyList.map((key) => (
                  <tr key={key.id} className="border-b last:border-0">
                    <td className="px-4 py-2 font-medium">{key.name}</td>
                    <td className="px-4 py-2 font-mono text-gray-500">{key.prefix}...</td>
                    <td className="px-4 py-2 text-gray-500">{new Date(key.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-2 text-gray-500">{key.last_used_at ? new Date(key.last_used_at).toLocaleDateString() : 'Never'}</td>
                    <td className="px-4 py-2 text-gray-500">{key.expires_at ? new Date(key.expires_at).toLocaleDateString() : 'Never'}</td>
                    <td className="px-4 py-2 text-right">
                      <button
                        onClick={() => {
                          if (confirm('Revoke this API key? This action cannot be undone.')) {
                            revokeAPIKeyMutation.mutate(key.id);
                          }
                        }}
                        className="text-red-600 hover:text-red-800 text-xs font-medium"
                      >
                        Revoke
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* SSO Configuration Tab */}
      {activeTab === 'sso' && (
        <div className="space-y-6 max-w-2xl">
          <div className="flex gap-2">
            <button
              onClick={() => setSsoProtocol('saml')}
              className={`px-4 py-2 text-sm font-medium rounded ${ssoProtocol === 'saml' ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-700'}`}
            >
              SAML 2.0
            </button>
            <button
              onClick={() => setSsoProtocol('oidc')}
              className={`px-4 py-2 text-sm font-medium rounded ${ssoProtocol === 'oidc' ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-700'}`}
            >
              OpenID Connect
            </button>
          </div>

          {ssoProtocol === 'saml' && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">Entity ID (Issuer)</label>
                <input
                  type="text"
                  value={ssoConfig.entity_id ?? ssoData?.entity_id ?? ''}
                  onChange={(e) => setSsoConfig({ ...ssoConfig, entity_id: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">SSO URL</label>
                <input
                  type="text"
                  value={ssoConfig.sso_url ?? ssoData?.sso_url ?? ''}
                  onChange={(e) => setSsoConfig({ ...ssoConfig, sso_url: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Certificate (PEM)</label>
                <textarea
                  value={ssoConfig.certificate ?? ssoData?.certificate ?? ''}
                  onChange={(e) => setSsoConfig({ ...ssoConfig, certificate: e.target.value })}
                  rows={4}
                  className="w-full border rounded px-3 py-2 text-sm font-mono"
                />
              </div>
            </div>
          )}

          {ssoProtocol === 'oidc' && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">Discovery URL</label>
                <input
                  type="text"
                  value={ssoConfig.discovery_url ?? ssoData?.discovery_url ?? ''}
                  onChange={(e) => setSsoConfig({ ...ssoConfig, discovery_url: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Client ID</label>
                <input
                  type="text"
                  value={ssoConfig.client_id ?? ssoData?.client_id ?? ''}
                  onChange={(e) => setSsoConfig({ ...ssoConfig, client_id: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Client Secret</label>
                <input
                  type="password"
                  value={ssoConfig.client_secret ?? ''}
                  onChange={(e) => setSsoConfig({ ...ssoConfig, client_secret: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                  placeholder="Enter to update"
                />
              </div>
            </div>
          )}

          <button
            onClick={() => updateSSOMutation.mutate({ protocol: ssoProtocol, ...ssoConfig })}
            disabled={updateSSOMutation.isPending}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {updateSSOMutation.isPending ? 'Saving...' : 'Save SSO Configuration'}
          </button>
        </div>
      )}

      {/* Configure Dialog (overlay) */}
      {configDialog?.open && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-lg shadow-xl max-w-lg w-full p-6 space-y-4">
            <h2 className="text-lg font-bold">Configure {configDialog.name}</h2>
            <div className="space-y-3">
              <div>
                <label className="block text-sm font-medium mb-1">Display Name</label>
                <input
                  type="text"
                  value={configFields.name ?? ''}
                  onChange={(e) => setConfigFields({ ...configFields, name: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                  placeholder={configDialog.name}
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Endpoint / URL</label>
                <input
                  type="text"
                  value={configFields.endpoint ?? ''}
                  onChange={(e) => setConfigFields({ ...configFields, endpoint: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">API Key / Token</label>
                <input
                  type="password"
                  value={configFields.api_key ?? ''}
                  onChange={(e) => setConfigFields({ ...configFields, api_key: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
            </div>
            <div className="flex gap-2 justify-end pt-2">
              <button
                onClick={() => setConfigDialog(null)}
                className="px-4 py-2 text-sm font-medium rounded border border-gray-300 hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={() =>
                  createIntegration.mutate({
                    type: configDialog.type,
                    name: configFields.name || configDialog.name,
                    config: { endpoint: configFields.endpoint, api_key: configFields.api_key },
                  })
                }
                disabled={createIntegration.isPending}
                className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {createIntegration.isPending ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
