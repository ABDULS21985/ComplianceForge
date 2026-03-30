export const FRAMEWORK_COLORS: Record<string, string> = {
  ISO27001: '#1A56DB',
  UK_GDPR: '#7C3AED',
  NCSC_CAF: '#059669',
  CYBER_ESSENTIALS: '#EA580C',
  NIST_800_53: '#DC2626',
  NIST_CSF_2: '#2563EB',
  PCI_DSS_4: '#0891B2',
  ITIL_4: '#4F46E5',
  COBIT_2019: '#9333EA',
};

export const RISK_LEVEL_ORDER: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  very_low: 4,
};

export const MATURITY_LABELS: Record<number, string> = {
  0: 'Non-existent',
  1: 'Initial',
  2: 'Managed',
  3: 'Defined',
  4: 'Quantitatively Managed',
  5: 'Optimizing',
};

export const NAV_ITEMS = [
  { label: 'Dashboard', href: '/dashboard', icon: 'LayoutDashboard' },
  { label: 'Frameworks', href: '/frameworks', icon: 'Shield' },
  { label: 'Risk Register', href: '/risks', icon: 'AlertTriangle' },
  { label: 'Policies', href: '/policies', icon: 'FileText' },
  { label: 'Exceptions', href: '/exceptions', icon: 'ShieldOff' },
  { label: 'Evidence', href: '/evidence', icon: 'FolderCheck' },
  { label: 'Audits', href: '/audits', icon: 'ClipboardCheck' },
  { label: 'Incidents', href: '/incidents', icon: 'AlertOctagon', badgeKey: 'open_incidents' },
  { label: 'Vendors', href: '/vendors', icon: 'Building2' },
  { label: 'Assessments', href: '/vendor-assessments', icon: 'ClipboardList' },
  { label: 'Assets', href: '/assets', icon: 'Server' },
  { label: 'Data Governance', href: '/data', icon: 'Database' },
  { label: 'Board', href: '/board', icon: 'Users' },
  { label: 'Reports', href: '/reports', icon: 'BarChart3' },
  { label: 'DSR Requests', href: '/dsr', icon: 'UserCheck' },
  { label: 'NIS2', href: '/nis2', icon: 'ShieldCheck' },
  { label: 'Monitoring', href: '/monitoring', icon: 'Activity' },
  { label: 'Remediation', href: '/remediation', icon: 'Wrench' },
  { label: 'Marketplace', href: '/marketplace', icon: 'Store' },
  { label: 'Regulatory', href: '/regulatory', icon: 'Scale' },
  { label: 'BIA', href: '/bia', icon: 'Zap' },
  { label: 'Analytics', href: '/analytics', icon: 'TrendingUp' },
  { label: 'Workflows', href: '/workflows', icon: 'GitPullRequest' },
  { label: 'Notifications', href: '/settings/notifications', icon: 'Bell' },
  { label: 'Settings', href: '/settings', icon: 'Settings' },
] as const;

export const COUNTRIES_EU_UK = [
  { code: 'GB', name: 'United Kingdom' }, { code: 'DE', name: 'Germany' },
  { code: 'FR', name: 'France' }, { code: 'IT', name: 'Italy' },
  { code: 'ES', name: 'Spain' }, { code: 'NL', name: 'Netherlands' },
  { code: 'BE', name: 'Belgium' }, { code: 'AT', name: 'Austria' },
  { code: 'SE', name: 'Sweden' }, { code: 'DK', name: 'Denmark' },
  { code: 'FI', name: 'Finland' }, { code: 'IE', name: 'Ireland' },
  { code: 'PT', name: 'Portugal' }, { code: 'PL', name: 'Poland' },
  { code: 'CZ', name: 'Czech Republic' }, { code: 'RO', name: 'Romania' },
  { code: 'HU', name: 'Hungary' }, { code: 'SK', name: 'Slovakia' },
  { code: 'HR', name: 'Croatia' }, { code: 'BG', name: 'Bulgaria' },
  { code: 'LT', name: 'Lithuania' }, { code: 'LV', name: 'Latvia' },
  { code: 'EE', name: 'Estonia' }, { code: 'SI', name: 'Slovenia' },
  { code: 'LU', name: 'Luxembourg' }, { code: 'MT', name: 'Malta' },
  { code: 'CY', name: 'Cyprus' }, { code: 'NO', name: 'Norway' },
  { code: 'CH', name: 'Switzerland' },
];
