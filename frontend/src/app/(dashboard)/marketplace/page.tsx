'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface MarketplacePackage {
  id: string;
  name: string;
  slug: string;
  publisher: string;
  publisher_slug: string;
  description: string;
  type: 'control_library' | 'policy_template' | 'risk_template' | 'framework_pack' | 'integration';
  category: string;
  region?: string;
  industry?: string;
  framework?: string;
  rating: number;
  review_count: number;
  download_count: number;
  price: number;
  price_label?: string;
  featured?: boolean;
  icon_url?: string;
  tags?: string[];
  installed?: boolean;
}

interface InstalledPackage {
  id: string;
  package_id: string;
  package_name: string;
  publisher: string;
  installed_at: string;
  version: string;
  type: string;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const TYPE_OPTIONS = [
  { value: '', label: 'All Types' },
  { value: 'control_library', label: 'Control Library' },
  { value: 'policy_template', label: 'Policy Template' },
  { value: 'risk_template', label: 'Risk Template' },
  { value: 'framework_pack', label: 'Framework Pack' },
  { value: 'integration', label: 'Integration' },
];

const CATEGORY_OPTIONS = [
  { value: '', label: 'All Categories' },
  { value: 'security', label: 'Security' },
  { value: 'privacy', label: 'Privacy' },
  { value: 'governance', label: 'Governance' },
  { value: 'risk', label: 'Risk Management' },
  { value: 'compliance', label: 'Compliance' },
  { value: 'audit', label: 'Audit' },
];

const REGION_OPTIONS = [
  { value: '', label: 'All Regions' },
  { value: 'global', label: 'Global' },
  { value: 'eu', label: 'European Union' },
  { value: 'uk', label: 'United Kingdom' },
  { value: 'us', label: 'United States' },
  { value: 'apac', label: 'Asia Pacific' },
];

const SORT_OPTIONS = [
  { value: 'popular', label: 'Most Popular' },
  { value: 'rating', label: 'Highest Rated' },
  { value: 'newest', label: 'Newest' },
  { value: 'price_asc', label: 'Price: Low to High' },
  { value: 'price_desc', label: 'Price: High to Low' },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function MarketplacePage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('');
  const [regionFilter, setRegionFilter] = useState('');
  const [sortBy, setSortBy] = useState('popular');
  const [showInstalled, setShowInstalled] = useState(false);
  const [installConfirm, setInstallConfirm] = useState<MarketplacePackage | null>(null);
  const [featuredIndex, setFeaturedIndex] = useState(0);

  // Fetch packages
  const { data: packagesData, isLoading } = useQuery({
    queryKey: ['marketplace-packages', search, typeFilter, categoryFilter, regionFilter, sortBy],
    queryFn: () =>
      api.marketplace.search({
        search: search || undefined,
        type: typeFilter || undefined,
        category: categoryFilter || undefined,
        region: regionFilter || undefined,
        sort: sortBy,
      }),
  });

  // Fetch featured
  const { data: featuredData } = useQuery({
    queryKey: ['marketplace-featured'],
    queryFn: () => api.marketplace.featured(),
  });

  // Fetch installed
  const { data: installedData } = useQuery({
    queryKey: ['marketplace-installed'],
    queryFn: () => api.marketplace.installed(),
  });

  // Install mutation
  const installMutation = useMutation({
    mutationFn: (data: { package_id: string }) => api.marketplace.install(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['marketplace-installed'] });
      queryClient.invalidateQueries({ queryKey: ['marketplace-packages'] });
      setInstallConfirm(null);
    },
  });

  // Uninstall mutation
  const uninstallMutation = useMutation({
    mutationFn: (id: string) => api.marketplace.uninstall(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['marketplace-installed'] });
      queryClient.invalidateQueries({ queryKey: ['marketplace-packages'] });
    },
  });

  const packages: MarketplacePackage[] = packagesData?.items ?? packagesData ?? [];
  const featured: MarketplacePackage[] = featuredData?.items ?? featuredData ?? [];
  const installed: InstalledPackage[] = installedData?.items ?? installedData ?? [];
  const installedIds = new Set(installed.map((i) => i.package_id));

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Control Library Marketplace</h1>
        <div className="flex gap-2">
          <button
            onClick={() => setShowInstalled(false)}
            className={`px-4 py-2 text-sm font-medium rounded ${!showInstalled ? 'bg-blue-600 text-white' : 'border border-gray-300 text-gray-700 hover:bg-gray-50'}`}
          >
            Browse
          </button>
          <button
            onClick={() => setShowInstalled(true)}
            className={`px-4 py-2 text-sm font-medium rounded ${showInstalled ? 'bg-blue-600 text-white' : 'border border-gray-300 text-gray-700 hover:bg-gray-50'}`}
          >
            Installed ({installed.length})
          </button>
        </div>
      </div>

      {/* Search Bar */}
      {!showInstalled && (
        <div className="relative">
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search packages, controls, templates..."
            className="w-full border rounded-lg px-4 py-3 text-sm pl-10 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
          />
          <svg className="absolute left-3 top-3.5 h-4 w-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>
      )}

      {showInstalled ? (
        /* Installed Packages Section */
        <div className="space-y-3">
          {installed.length === 0 && (
            <div className="text-center py-16 text-gray-500">
              <p className="text-lg font-medium">No packages installed</p>
              <p className="text-sm mt-1">Browse the marketplace to find and install packages</p>
            </div>
          )}
          {installed.map((pkg) => (
            <div key={pkg.id} className="border rounded-lg p-4 bg-white shadow-sm flex items-center justify-between">
              <div>
                <div className="flex items-center gap-2">
                  <p className="font-semibold text-gray-900">{pkg.package_name}</p>
                  <TypeBadge type={pkg.type} />
                </div>
                <p className="text-sm text-gray-500 mt-0.5">
                  {pkg.publisher} &middot; v{pkg.version} &middot; Installed {new Date(pkg.installed_at).toLocaleDateString()}
                </p>
              </div>
              <button
                onClick={() => uninstallMutation.mutate(pkg.id)}
                disabled={uninstallMutation.isPending}
                className="px-3 py-1.5 text-sm font-medium rounded border border-red-300 text-red-600 hover:bg-red-50"
              >
                Uninstall
              </button>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex gap-6">
          {/* Filter Sidebar */}
          <div className="w-56 shrink-0 space-y-4">
            <FilterSection label="Type" options={TYPE_OPTIONS} value={typeFilter} onChange={setTypeFilter} />
            <FilterSection label="Category" options={CATEGORY_OPTIONS} value={categoryFilter} onChange={setCategoryFilter} />
            <FilterSection label="Region" options={REGION_OPTIONS} value={regionFilter} onChange={setRegionFilter} />
            <div>
              <label className="block text-xs font-semibold text-gray-500 uppercase mb-2">Sort By</label>
              <select
                value={sortBy}
                onChange={(e) => setSortBy(e.target.value)}
                className="w-full border rounded px-3 py-2 text-sm"
              >
                {SORT_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Main Content */}
          <div className="flex-1 space-y-6">
            {/* Featured Carousel */}
            {featured.length > 0 && !search && (
              <div className="bg-gradient-to-r from-blue-600 to-purple-600 rounded-xl p-6 text-white relative overflow-hidden">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs font-semibold uppercase tracking-wider opacity-80">Featured</span>
                  <div className="flex gap-1">
                    <button
                      onClick={() => setFeaturedIndex(Math.max(0, featuredIndex - 1))}
                      disabled={featuredIndex === 0}
                      className="w-6 h-6 rounded-full bg-white/20 flex items-center justify-center text-xs hover:bg-white/30 disabled:opacity-30"
                    >
                      &lt;
                    </button>
                    <button
                      onClick={() => setFeaturedIndex(Math.min(featured.length - 1, featuredIndex + 1))}
                      disabled={featuredIndex >= featured.length - 1}
                      className="w-6 h-6 rounded-full bg-white/20 flex items-center justify-center text-xs hover:bg-white/30 disabled:opacity-30"
                    >
                      &gt;
                    </button>
                  </div>
                </div>
                {featured[featuredIndex] && (
                  <div>
                    <h3 className="text-xl font-bold">{featured[featuredIndex].name}</h3>
                    <p className="text-sm opacity-90 mt-1">{featured[featuredIndex].description}</p>
                    <div className="flex items-center gap-3 mt-3">
                      <span className="text-sm">{featured[featuredIndex].publisher}</span>
                      <Stars rating={featured[featuredIndex].rating} light />
                      <span className="text-xs opacity-75">{featured[featuredIndex].download_count?.toLocaleString()} downloads</span>
                    </div>
                    <button
                      onClick={() => setInstallConfirm(featured[featuredIndex])}
                      className="mt-3 px-4 py-2 text-sm font-medium rounded bg-white text-blue-700 hover:bg-blue-50"
                    >
                      Install
                    </button>
                  </div>
                )}
                <div className="flex gap-1 justify-center mt-4">
                  {featured.map((_, idx) => (
                    <button
                      key={idx}
                      onClick={() => setFeaturedIndex(idx)}
                      className={`w-2 h-2 rounded-full ${idx === featuredIndex ? 'bg-white' : 'bg-white/40'}`}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* Package Grid */}
            {isLoading ? (
              <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
                {Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="h-48 rounded-lg bg-gray-100 animate-pulse" />
                ))}
              </div>
            ) : packages.length === 0 ? (
              <div className="text-center py-16 text-gray-500">
                <p className="text-lg font-medium">No packages found</p>
                <p className="text-sm mt-1">Try adjusting your search or filters</p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
                {packages.map((pkg) => (
                  <div key={pkg.id} className="border rounded-lg p-4 bg-white shadow-sm hover:shadow-md transition-shadow">
                    <div className="flex items-start justify-between gap-2 mb-2">
                      <div className="flex-1 min-w-0">
                        <p className="font-semibold text-gray-900 truncate">{pkg.name}</p>
                        <p className="text-xs text-gray-500">{pkg.publisher}</p>
                      </div>
                      <TypeBadge type={pkg.type} />
                    </div>
                    <p className="text-sm text-gray-600 line-clamp-2 mb-3">{pkg.description}</p>
                    <div className="flex items-center gap-2 mb-3">
                      <Stars rating={pkg.rating} />
                      <span className="text-xs text-gray-400">({pkg.review_count})</span>
                      <span className="text-xs text-gray-400 ml-auto">{pkg.download_count?.toLocaleString()} downloads</span>
                    </div>
                    {pkg.tags && pkg.tags.length > 0 && (
                      <div className="flex flex-wrap gap-1 mb-3">
                        {pkg.tags.slice(0, 3).map((tag) => (
                          <span key={tag} className="text-xs bg-gray-100 text-gray-500 px-2 py-0.5 rounded">{tag}</span>
                        ))}
                      </div>
                    )}
                    <div className="flex items-center justify-between pt-2 border-t">
                      <span className={`text-sm font-semibold ${pkg.price === 0 ? 'text-green-600' : 'text-gray-900'}`}>
                        {pkg.price === 0 ? 'Free' : (pkg.price_label ?? `$${pkg.price}`)}
                      </span>
                      {installedIds.has(pkg.id) ? (
                        <span className="text-xs text-green-600 font-medium px-2 py-1 bg-green-50 rounded">Installed</span>
                      ) : (
                        <button
                          onClick={() => setInstallConfirm(pkg)}
                          className="px-3 py-1.5 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
                        >
                          Install
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Install Confirmation Dialog */}
      {installConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-white rounded-xl shadow-2xl max-w-md w-full mx-4 p-6">
            <h3 className="text-lg font-bold text-gray-900 mb-2">Install Package</h3>
            <p className="text-sm text-gray-600 mb-4">
              Are you sure you want to install <strong>{installConfirm.name}</strong> by {installConfirm.publisher}?
            </p>
            <div className="bg-gray-50 rounded-lg p-3 mb-4 space-y-1.5">
              <p className="text-xs text-gray-500">This will import:</p>
              <div className="flex items-center gap-2">
                <TypeBadge type={installConfirm.type} />
                <span className="text-sm text-gray-700">{installConfirm.name}</span>
              </div>
              {installConfirm.framework && (
                <p className="text-xs text-gray-500">Framework: {installConfirm.framework}</p>
              )}
              {installConfirm.category && (
                <p className="text-xs text-gray-500">Category: {installConfirm.category}</p>
              )}
              {installConfirm.price > 0 && (
                <p className="text-sm font-semibold text-gray-900 mt-2">
                  Price: {installConfirm.price_label ?? `$${installConfirm.price}`}
                </p>
              )}
            </div>
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => setInstallConfirm(null)}
                className="px-4 py-2 text-sm font-medium rounded border border-gray-300 text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={() => installMutation.mutate({ package_id: installConfirm.id })}
                disabled={installMutation.isPending}
                className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {installMutation.isPending ? 'Installing...' : 'Confirm Install'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function FilterSection({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <label className="block text-xs font-semibold text-gray-500 uppercase mb-2">{label}</label>
      <div className="space-y-1">
        {options.map((opt) => (
          <label
            key={opt.value}
            className={`flex items-center gap-2 px-2 py-1.5 rounded cursor-pointer text-sm ${
              value === opt.value ? 'bg-blue-50 text-blue-700 font-medium' : 'text-gray-600 hover:bg-gray-50'
            }`}
          >
            <input
              type="radio"
              name={label}
              checked={value === opt.value}
              onChange={() => onChange(opt.value)}
              className="sr-only"
            />
            {opt.label}
          </label>
        ))}
      </div>
    </div>
  );
}

function Stars({ rating, light }: { rating: number; light?: boolean }) {
  return (
    <div className="flex gap-0.5">
      {Array.from({ length: 5 }).map((_, i) => (
        <svg
          key={i}
          className={`w-3.5 h-3.5 ${i < Math.round(rating) ? (light ? 'text-yellow-300' : 'text-yellow-400') : (light ? 'text-white/30' : 'text-gray-200')}`}
          fill="currentColor"
          viewBox="0 0 20 20"
        >
          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
        </svg>
      ))}
    </div>
  );
}

function TypeBadge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    control_library: 'bg-blue-100 text-blue-700',
    policy_template: 'bg-purple-100 text-purple-700',
    risk_template: 'bg-orange-100 text-orange-700',
    framework_pack: 'bg-green-100 text-green-700',
    integration: 'bg-cyan-100 text-cyan-700',
  };
  const labels: Record<string, string> = {
    control_library: 'Controls',
    policy_template: 'Policy',
    risk_template: 'Risk',
    framework_pack: 'Framework',
    integration: 'Integration',
  };
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${styles[type] ?? 'bg-gray-100 text-gray-600'}`}>
      {labels[type] ?? type}
    </span>
  );
}
