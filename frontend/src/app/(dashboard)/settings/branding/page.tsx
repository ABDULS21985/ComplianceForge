'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BrandingConfig {
  logo_url?: string;
  logo_dark_url?: string;
  favicon_url?: string;
  primary_color: string;
  secondary_color: string;
  accent_color: string;
  sidebar_color: string;
  font_family: string;
  sidebar_style: 'full' | 'compact' | 'icons_only';
  corner_radius: 'none' | 'small' | 'medium' | 'large';
  density: 'comfortable' | 'compact' | 'spacious';
  show_powered_by: boolean;
  custom_css: string;
  custom_domain?: string;
  domain_status?: 'none' | 'pending_verification' | 'verified' | 'active';
  dns_records?: DnsRecord[];
}

interface DnsRecord {
  type: string;
  name: string;
  value: string;
  verified: boolean;
}

const DEFAULT_CONFIG: BrandingConfig = {
  primary_color: '#4F46E5',
  secondary_color: '#7C3AED',
  accent_color: '#06B6D4',
  sidebar_color: '#1F2937',
  font_family: 'Inter',
  sidebar_style: 'full',
  corner_radius: 'medium',
  density: 'comfortable',
  show_powered_by: true,
  custom_css: '',
};

const FONT_OPTIONS = [
  'Inter', 'Roboto', 'Open Sans', 'Lato', 'Poppins', 'Nunito', 'Source Sans Pro', 'Montserrat', 'Raleway', 'IBM Plex Sans',
];

const RADIUS_OPTIONS: { value: string; label: string; preview: string }[] = [
  { value: 'none', label: 'None', preview: 'rounded-none' },
  { value: 'small', label: 'Small', preview: 'rounded' },
  { value: 'medium', label: 'Medium', preview: 'rounded-lg' },
  { value: 'large', label: 'Large', preview: 'rounded-xl' },
];

const DENSITY_OPTIONS = [
  { value: 'compact', label: 'Compact' },
  { value: 'comfortable', label: 'Comfortable' },
  { value: 'spacious', label: 'Spacious' },
];

const SIDEBAR_STYLES = [
  { value: 'full', label: 'Full Width', description: 'Expanded sidebar with labels' },
  { value: 'compact', label: 'Compact', description: 'Narrower sidebar' },
  { value: 'icons_only', label: 'Icons Only', description: 'Minimal icon sidebar' },
];

type DomainStep = 'enter' | 'dns' | 'verify' | 'active';

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function BrandingSettingsPage() {
  const [config, setConfig] = useState<BrandingConfig>(DEFAULT_CONFIG);
  const [savedConfig, setSavedConfig] = useState<BrandingConfig>(DEFAULT_CONFIG);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [showPreview, setShowPreview] = useState(false);
  const [domainStep, setDomainStep] = useState<DomainStep>('enter');
  const [domainInput, setDomainInput] = useState('');
  const [verifying, setVerifying] = useState(false);
  const [uploadingLogo, setUploadingLogo] = useState<string | null>(null);

  const logoRef = useRef<HTMLInputElement>(null);
  const logoDarkRef = useRef<HTMLInputElement>(null);
  const faviconRef = useRef<HTMLInputElement>(null);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = (await api.branding.get()) as BrandingConfig;
      const merged = { ...DEFAULT_CONFIG, ...data };
      setConfig(merged);
      setSavedConfig(merged);
      setDomainInput(merged.custom_domain ?? '');
      if (merged.domain_status === 'active') setDomainStep('active');
      else if (merged.domain_status === 'verified') setDomainStep('active');
      else if (merged.domain_status === 'pending_verification') setDomainStep('dns');
      else setDomainStep('enter');
    } catch {
      setError('Failed to load branding settings.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchConfig(); }, [fetchConfig]);

  const updateField = <K extends keyof BrandingConfig>(key: K, value: BrandingConfig[K]) => {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSuccess(false);
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    setSuccess(false);
    try {
      await api.branding.update(config);
      setSavedConfig(config);
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
    } catch {
      setError('Failed to save branding settings.');
    } finally {
      setSaving(false);
    }
  };

  const handleReset = () => {
    setConfig(DEFAULT_CONFIG);
    setSuccess(false);
  };

  // Logo upload
  const handleFileUpload = async (field: 'logo_url' | 'logo_dark_url' | 'favicon_url', file: File) => {
    setUploadingLogo(field);
    try {
      const formData = new FormData();
      formData.append('file', file);
      formData.append('type', field);
      const result = (await api.branding.uploadLogo(formData)) as { url: string };
      updateField(field, result.url);
    } catch {
      setError(`Failed to upload ${field.replace('_url', '').replace('_', ' ')}.`);
    } finally {
      setUploadingLogo(null);
    }
  };

  const handleDrop = (field: 'logo_url' | 'logo_dark_url' | 'favicon_url') => (e: React.DragEvent) => {
    e.preventDefault();
    const file = e.dataTransfer.files[0];
    if (file && file.type.startsWith('image/')) handleFileUpload(field, file);
  };

  // Domain wizard
  const startDomainSetup = async () => {
    if (!domainInput.trim()) return;
    try {
      const result = (await api.branding.setupDomain({ domain: domainInput.trim() })) as {
        dns_records: DnsRecord[];
      };
      updateField('custom_domain', domainInput.trim());
      updateField('dns_records', result.dns_records);
      updateField('domain_status', 'pending_verification');
      setDomainStep('dns');
    } catch {
      setError('Failed to initiate domain setup.');
    }
  };

  const verifyDomain = async () => {
    setVerifying(true);
    try {
      const result = (await api.branding.verifyDomain()) as { verified: boolean; status: string };
      if (result.verified) {
        updateField('domain_status', 'active');
        setDomainStep('active');
      } else {
        setError('DNS records not yet propagated. Please try again later.');
      }
    } catch {
      setError('Domain verification failed. Please check DNS records.');
    } finally {
      setVerifying(false);
    }
  };

  const hasChanges = JSON.stringify(config) !== JSON.stringify(savedConfig);

  // Loading state
  if (loading) {
    return (
      <div className="p-6 space-y-6 animate-pulse">
        <div className="h-8 bg-gray-200 rounded w-48" />
        <div className="h-64 bg-gray-100 rounded-xl" />
        <div className="h-48 bg-gray-100 rounded-xl" />
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Branding Settings</h1>
          <p className="text-sm text-gray-500 mt-1">Customize the look and feel of your GRC platform</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowPreview(!showPreview)}
            className="px-4 py-2 text-sm font-medium bg-white border border-gray-300 rounded-lg hover:bg-gray-50 text-gray-700"
          >
            {showPreview ? 'Hide Preview' : 'Preview'}
          </button>
          <button
            onClick={handleReset}
            className="px-4 py-2 text-sm font-medium bg-white border border-gray-300 rounded-lg hover:bg-gray-50 text-gray-700"
          >
            Reset to Default
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !hasChanges}
            className="px-4 py-2 text-sm font-medium bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      </div>

      {/* Status messages */}
      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 p-3 rounded-lg text-sm">
          {error}
          <button onClick={() => setError(null)} className="ml-2 font-medium underline">Dismiss</button>
        </div>
      )}
      {success && (
        <div className="bg-green-50 border border-green-200 text-green-700 p-3 rounded-lg text-sm">
          Branding settings saved successfully.
        </div>
      )}

      <div className="flex flex-col xl:flex-row gap-6">
        {/* Settings */}
        <div className="flex-1 space-y-6">
          {/* Logo Upload */}
          <section className="bg-white border border-gray-200 rounded-xl p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Logo &amp; Favicon</h2>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              {([
                { field: 'logo_url' as const, label: 'Primary Logo', ref: logoRef },
                { field: 'logo_dark_url' as const, label: 'Dark Mode Logo', ref: logoDarkRef },
                { field: 'favicon_url' as const, label: 'Favicon', ref: faviconRef },
              ]).map(({ field, label, ref }) => (
                <div key={field}>
                  <label className="text-xs font-medium text-gray-600 mb-1.5 block">{label}</label>
                  <div
                    onDragOver={(e) => e.preventDefault()}
                    onDrop={handleDrop(field)}
                    onClick={() => ref.current?.click()}
                    className="border-2 border-dashed border-gray-300 rounded-lg p-4 text-center cursor-pointer hover:border-indigo-400 hover:bg-indigo-50/30 transition-colors min-h-[100px] flex flex-col items-center justify-center"
                  >
                    {config[field] ? (
                      <img src={config[field]} alt={label} className="max-h-12 max-w-full object-contain" />
                    ) : (
                      <svg className="w-8 h-8 text-gray-300 mb-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                      </svg>
                    )}
                    {uploadingLogo === field ? (
                      <span className="text-xs text-indigo-600 mt-1">Uploading...</span>
                    ) : (
                      <span className="text-xs text-gray-400 mt-1">Drop or click</span>
                    )}
                  </div>
                  <input
                    ref={ref}
                    type="file"
                    accept="image/*"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) handleFileUpload(field, file);
                    }}
                  />
                </div>
              ))}
            </div>
          </section>

          {/* Colors */}
          <section className="bg-white border border-gray-200 rounded-xl p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Colors</h2>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              {([
                { field: 'primary_color' as const, label: 'Primary' },
                { field: 'secondary_color' as const, label: 'Secondary' },
                { field: 'accent_color' as const, label: 'Accent' },
                { field: 'sidebar_color' as const, label: 'Sidebar' },
              ]).map(({ field, label }) => (
                <div key={field}>
                  <label className="text-xs font-medium text-gray-600 mb-1.5 block">{label}</label>
                  <div className="flex items-center gap-2">
                    <input
                      type="color"
                      value={config[field]}
                      onChange={(e) => updateField(field, e.target.value)}
                      className="w-10 h-10 rounded-lg border border-gray-300 cursor-pointer p-0.5"
                    />
                    <input
                      type="text"
                      value={config[field]}
                      onChange={(e) => updateField(field, e.target.value)}
                      className="flex-1 text-sm border border-gray-300 rounded-lg px-2 py-1.5 font-mono"
                      maxLength={7}
                    />
                  </div>
                </div>
              ))}
            </div>
          </section>

          {/* Typography & Layout */}
          <section className="bg-white border border-gray-200 rounded-xl p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Typography &amp; Layout</h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {/* Font */}
              <div>
                <label className="text-xs font-medium text-gray-600 mb-1.5 block">Font Family</label>
                <select
                  value={config.font_family}
                  onChange={(e) => updateField('font_family', e.target.value)}
                  className="w-full text-sm border border-gray-300 rounded-lg px-3 py-2"
                >
                  {FONT_OPTIONS.map((f) => (
                    <option key={f} value={f}>{f}</option>
                  ))}
                </select>
              </div>

              {/* Corner Radius */}
              <div>
                <label className="text-xs font-medium text-gray-600 mb-1.5 block">Corner Radius</label>
                <div className="flex gap-2">
                  {RADIUS_OPTIONS.map((opt) => (
                    <button
                      key={opt.value}
                      onClick={() => updateField('corner_radius', opt.value as any)}
                      className={`flex-1 py-2 text-xs font-medium border rounded-lg transition-colors ${
                        config.corner_radius === opt.value
                          ? 'bg-indigo-50 border-indigo-300 text-indigo-700'
                          : 'border-gray-300 text-gray-600 hover:bg-gray-50'
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Sidebar Style */}
              <div>
                <label className="text-xs font-medium text-gray-600 mb-1.5 block">Sidebar Style</label>
                <div className="space-y-1.5">
                  {SIDEBAR_STYLES.map((s) => (
                    <label
                      key={s.value}
                      className={`flex items-center gap-2 p-2 rounded-lg border cursor-pointer transition-colors ${
                        config.sidebar_style === s.value
                          ? 'bg-indigo-50 border-indigo-300'
                          : 'border-gray-200 hover:border-gray-300'
                      }`}
                    >
                      <input
                        type="radio"
                        name="sidebar_style"
                        checked={config.sidebar_style === s.value}
                        onChange={() => updateField('sidebar_style', s.value as any)}
                        className="w-3.5 h-3.5 text-indigo-600"
                      />
                      <div>
                        <div className="text-xs font-medium text-gray-900">{s.label}</div>
                        <div className="text-[10px] text-gray-500">{s.description}</div>
                      </div>
                    </label>
                  ))}
                </div>
              </div>

              {/* Density */}
              <div>
                <label className="text-xs font-medium text-gray-600 mb-1.5 block">UI Density</label>
                <div className="flex gap-2">
                  {DENSITY_OPTIONS.map((d) => (
                    <button
                      key={d.value}
                      onClick={() => updateField('density', d.value as any)}
                      className={`flex-1 py-2 text-xs font-medium border rounded-lg transition-colors ${
                        config.density === d.value
                          ? 'bg-indigo-50 border-indigo-300 text-indigo-700'
                          : 'border-gray-300 text-gray-600 hover:bg-gray-50'
                      }`}
                    >
                      {d.label}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Powered By toggle */}
            <div className="mt-4 flex items-center justify-between p-3 bg-gray-50 rounded-lg">
              <div>
                <div className="text-sm font-medium text-gray-900">Show &quot;Powered by ComplianceForge&quot;</div>
                <div className="text-xs text-gray-500">Display attribution in the footer</div>
              </div>
              <button
                onClick={() => updateField('show_powered_by', !config.show_powered_by)}
                className={`relative w-11 h-6 rounded-full transition-colors ${
                  config.show_powered_by ? 'bg-indigo-600' : 'bg-gray-300'
                }`}
              >
                <span
                  className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full shadow transition-transform ${
                    config.show_powered_by ? 'translate-x-5' : ''
                  }`}
                />
              </button>
            </div>
          </section>

          {/* Custom Domain */}
          <section className="bg-white border border-gray-200 rounded-xl p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-4">Custom Domain</h2>

            {domainStep === 'enter' && (
              <div>
                <p className="text-sm text-gray-600 mb-3">Set up a custom domain for your GRC platform.</p>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={domainInput}
                    onChange={(e) => setDomainInput(e.target.value)}
                    placeholder="grc.yourcompany.com"
                    className="flex-1 text-sm border border-gray-300 rounded-lg px-3 py-2"
                  />
                  <button
                    onClick={startDomainSetup}
                    disabled={!domainInput.trim()}
                    className="px-4 py-2 text-sm font-medium bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50"
                  >
                    Set Up Domain
                  </button>
                </div>
              </div>
            )}

            {domainStep === 'dns' && (
              <div>
                <p className="text-sm text-gray-600 mb-3">
                  Add these DNS records to your domain registrar for <span className="font-semibold">{config.custom_domain}</span>:
                </p>
                <div className="border border-gray-200 rounded-lg overflow-hidden mb-4">
                  <table className="w-full text-sm">
                    <thead className="bg-gray-50">
                      <tr>
                        <th className="px-3 py-2 text-left text-xs font-semibold text-gray-600">Type</th>
                        <th className="px-3 py-2 text-left text-xs font-semibold text-gray-600">Name</th>
                        <th className="px-3 py-2 text-left text-xs font-semibold text-gray-600">Value</th>
                        <th className="px-3 py-2 text-left text-xs font-semibold text-gray-600">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(config.dns_records ?? []).map((rec, i) => (
                        <tr key={i} className="border-t border-gray-100">
                          <td className="px-3 py-2 font-mono text-xs">{rec.type}</td>
                          <td className="px-3 py-2 font-mono text-xs">{rec.name}</td>
                          <td className="px-3 py-2 font-mono text-xs break-all">{rec.value}</td>
                          <td className="px-3 py-2">
                            {rec.verified ? (
                              <span className="text-xs text-green-600 font-medium">Verified</span>
                            ) : (
                              <span className="text-xs text-yellow-600 font-medium">Pending</span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={verifyDomain}
                    disabled={verifying}
                    className="px-4 py-2 text-sm font-medium bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50"
                  >
                    {verifying ? 'Verifying...' : 'Verify DNS'}
                  </button>
                  <button
                    onClick={() => { setDomainStep('enter'); setDomainInput(''); }}
                    className="px-4 py-2 text-sm font-medium bg-white border border-gray-300 rounded-lg hover:bg-gray-50 text-gray-700"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {domainStep === 'active' && (
              <div className="flex items-center gap-3 p-3 bg-green-50 border border-green-200 rounded-lg">
                <svg className="w-5 h-5 text-green-600 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <div className="flex-1">
                  <div className="text-sm font-medium text-green-800">Custom domain active</div>
                  <div className="text-xs text-green-600">{config.custom_domain}</div>
                </div>
                <button
                  onClick={() => setDomainStep('enter')}
                  className="text-xs text-green-700 hover:text-green-800 font-medium underline"
                >
                  Change
                </button>
              </div>
            )}
          </section>

          {/* Custom CSS */}
          <section className="bg-white border border-gray-200 rounded-xl p-6">
            <h2 className="text-base font-semibold text-gray-900 mb-2">Custom CSS</h2>
            <p className="text-xs text-gray-500 mb-3">Add custom styles to override platform defaults. Use with caution.</p>
            <textarea
              value={config.custom_css}
              onChange={(e) => updateField('custom_css', e.target.value)}
              placeholder="/* Custom CSS overrides */&#10;.sidebar { }&#10;.header { }"
              rows={8}
              className="w-full text-sm font-mono border border-gray-300 rounded-lg px-3 py-2 bg-gray-50 focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
            />
          </section>
        </div>

        {/* Live Preview Panel */}
        {showPreview && (
          <div className="w-full xl:w-80 flex-shrink-0">
            <div className="sticky top-6">
              <div className="bg-white border border-gray-200 rounded-xl overflow-hidden">
                <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
                  <h3 className="text-sm font-semibold text-gray-700">Live Preview</h3>
                </div>
                <div className="p-4">
                  {/* Mini sidebar preview */}
                  <div className="flex rounded-lg overflow-hidden border border-gray-200 h-48">
                    <div
                      className="flex flex-col gap-1.5 p-2"
                      style={{
                        backgroundColor: config.sidebar_color,
                        width: config.sidebar_style === 'icons_only' ? '36px' : config.sidebar_style === 'compact' ? '48px' : '64px',
                      }}
                    >
                      {config.logo_url ? (
                        <img src={config.logo_url} alt="" className="w-full h-4 object-contain mb-1" />
                      ) : (
                        <div className="w-full h-4 rounded bg-white/20 mb-1" />
                      )}
                      {[...Array(5)].map((_, i) => (
                        <div
                          key={i}
                          className="rounded"
                          style={{
                            height: '6px',
                            backgroundColor: i === 0 ? config.primary_color : 'rgba(255,255,255,0.2)',
                          }}
                        />
                      ))}
                    </div>
                    <div className="flex-1 p-2 bg-gray-50" style={{ fontFamily: config.font_family }}>
                      <div
                        className="h-5 mb-2"
                        style={{
                          backgroundColor: config.primary_color,
                          borderRadius: config.corner_radius === 'none' ? '0' : config.corner_radius === 'small' ? '4px' : config.corner_radius === 'medium' ? '8px' : '12px',
                        }}
                      />
                      <div className="space-y-1.5">
                        <div className="h-3 bg-gray-200 rounded w-3/4" />
                        <div className="h-3 bg-gray-200 rounded w-1/2" />
                        <div className="flex gap-1">
                          <div
                            className="h-6 flex-1"
                            style={{
                              backgroundColor: config.primary_color,
                              borderRadius: config.corner_radius === 'none' ? '0' : config.corner_radius === 'small' ? '4px' : config.corner_radius === 'medium' ? '8px' : '12px',
                            }}
                          />
                          <div
                            className="h-6 flex-1"
                            style={{
                              backgroundColor: config.secondary_color,
                              borderRadius: config.corner_radius === 'none' ? '0' : config.corner_radius === 'small' ? '4px' : config.corner_radius === 'medium' ? '8px' : '12px',
                            }}
                          />
                        </div>
                        <div
                          className="h-2 w-8"
                          style={{ backgroundColor: config.accent_color, borderRadius: '4px' }}
                        />
                      </div>
                      {config.show_powered_by && (
                        <div className="mt-auto pt-2">
                          <div className="text-[6px] text-gray-400">Powered by ComplianceForge</div>
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Color swatches */}
                  <div className="mt-4 grid grid-cols-4 gap-2">
                    {[
                      { label: 'Primary', color: config.primary_color },
                      { label: 'Secondary', color: config.secondary_color },
                      { label: 'Accent', color: config.accent_color },
                      { label: 'Sidebar', color: config.sidebar_color },
                    ].map((s) => (
                      <div key={s.label} className="text-center">
                        <div className="w-full h-6 rounded-md border border-gray-200" style={{ backgroundColor: s.color }} />
                        <span className="text-[9px] text-gray-500 mt-0.5 block">{s.label}</span>
                      </div>
                    ))}
                  </div>

                  {/* Font preview */}
                  <div className="mt-4 p-2 bg-gray-50 rounded-lg">
                    <p className="text-xs text-gray-500 mb-1">Font: {config.font_family}</p>
                    <p className="text-sm font-semibold text-gray-900" style={{ fontFamily: config.font_family }}>
                      The quick brown fox
                    </p>
                    <p className="text-xs text-gray-600" style={{ fontFamily: config.font_family }}>
                      jumps over the lazy dog
                    </p>
                  </div>

                  {/* Settings summary */}
                  <div className="mt-4 space-y-1 text-[10px] text-gray-500">
                    <div className="flex justify-between"><span>Sidebar:</span><span className="capitalize">{config.sidebar_style.replace('_', ' ')}</span></div>
                    <div className="flex justify-between"><span>Corners:</span><span className="capitalize">{config.corner_radius}</span></div>
                    <div className="flex justify-between"><span>Density:</span><span className="capitalize">{config.density}</span></div>
                    <div className="flex justify-between"><span>Powered by:</span><span>{config.show_powered_by ? 'Shown' : 'Hidden'}</span></div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
