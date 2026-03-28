'use client';

import { useState, useCallback } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import api from '@/lib/api';

// ---------------------------------------------------------------------------
// Types & Constants
// ---------------------------------------------------------------------------

const STEPS = [
  'Organisation Profile',
  'Industry Assessment',
  'Framework Selection',
  'Team Setup',
  'Risk Appetite',
  'Quick Assessment',
  'Summary & Launch',
] as const;

const INDUSTRIES = [
  'Financial Services', 'Healthcare', 'Technology', 'Manufacturing',
  'Retail', 'Energy', 'Telecommunications', 'Government',
  'Education', 'Professional Services', 'Other',
];

const EMPLOYEE_RANGES = ['1-10', '11-50', '51-200', '201-500', '501-1000', '1001-5000', '5000+'];

const RISK_CATEGORIES = ['Cybersecurity', 'Operational', 'Compliance', 'Financial', 'Reputational', 'Strategic'];
const APPETITE_LEVELS = ['Very Low', 'Low', 'Medium', 'High', 'Very High'];

const CONTROL_STATUSES = [
  { value: 'not_implemented', label: 'Not Implemented' },
  { value: 'partial', label: 'Partially Implemented' },
  { value: 'implemented', label: 'Implemented' },
  { value: 'not_applicable', label: 'N/A' },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function OnboardPage() {
  const [currentStep, setCurrentStep] = useState(0);

  // Step data
  const [orgProfile, setOrgProfile] = useState({
    name: '', legal_name: '', industry: '', country: '', employee_count: '',
  });

  const [industryAnswers, setIndustryAnswers] = useState<Record<string, boolean>>({});

  const [selectedFrameworks, setSelectedFrameworks] = useState<string[]>([]);

  const [teamInvites, setTeamInvites] = useState<{ email: string; name: string; role: string }[]>([]);
  const [inviteForm, setInviteForm] = useState({ email: '', name: '', role: 'viewer' });

  const [riskAppetite, setRiskAppetite] = useState<Record<string, number>>(
    Object.fromEntries(RISK_CATEGORIES.map((c) => [c, 2])),
  );
  const [matrixSize, setMatrixSize] = useState(5);

  const [controlStatuses, setControlStatuses] = useState<Record<string, string>>({});

  const [launching, setLaunching] = useState(false);

  // Queries
  const { data: recommendations } = useQuery({
    queryKey: ['onboard-recommendations'],
    queryFn: () => api.onboarding.getRecommendations(),
    enabled: currentStep >= 2,
  });

  const { data: progress } = useQuery({
    queryKey: ['onboard-progress'],
    queryFn: () => api.onboarding.getProgress(),
  });

  // Mutations
  const saveStep = useMutation({
    mutationFn: ({ step, data }: { step: number; data: any }) => api.onboarding.saveStep(step, data),
  });

  const skipStep = useMutation({
    mutationFn: (step: number) => api.onboarding.skipStep(step),
  });

  const completeOnboarding = useMutation({
    mutationFn: () => api.onboarding.complete(),
    onSuccess: () => {
      if (typeof window !== 'undefined') {
        window.location.href = '/dashboard';
      }
    },
  });

  // Navigation
  const goNext = useCallback(() => {
    const stepData = getStepData(currentStep);
    saveStep.mutate({ step: currentStep + 1, data: stepData });
    setCurrentStep((s) => Math.min(s + 1, STEPS.length - 1));
  }, [currentStep, orgProfile, industryAnswers, selectedFrameworks, teamInvites, riskAppetite, matrixSize, controlStatuses]);

  const goBack = () => setCurrentStep((s) => Math.max(s - 1, 0));

  const handleSkip = () => {
    skipStep.mutate(currentStep + 1);
    setCurrentStep((s) => Math.min(s + 1, STEPS.length - 1));
  };

  function getStepData(step: number): any {
    switch (step) {
      case 0: return orgProfile;
      case 1: return { answers: industryAnswers };
      case 2: return { frameworks: selectedFrameworks };
      case 3: return { invites: teamInvites };
      case 4: return { risk_appetite: riskAppetite, matrix_size: matrixSize };
      case 5: return { control_statuses: controlStatuses };
      default: return {};
    }
  }

  function handleLaunch() {
    setLaunching(true);
    completeOnboarding.mutate();
  }

  // Framework data
  const availableFrameworks = recommendations?.frameworks ?? [
    { id: 'iso27001', name: 'ISO 27001', recommended: true },
    { id: 'uk_gdpr', name: 'UK GDPR', recommended: true },
    { id: 'nist_csf', name: 'NIST CSF 2.0', recommended: false },
    { id: 'cyber_essentials', name: 'Cyber Essentials', recommended: true },
    { id: 'pci_dss', name: 'PCI DSS 4.0', recommended: false },
    { id: 'soc2', name: 'SOC 2', recommended: false },
  ];

  const industryQuestions = recommendations?.questions ?? [
    { id: 'processes_personal_data', text: 'Does your organisation process personal data of EU/UK residents?' },
    { id: 'handles_payment', text: 'Does your organisation handle payment card data?' },
    { id: 'critical_infrastructure', text: 'Is your organisation part of critical national infrastructure?' },
    { id: 'public_sector', text: 'Is your organisation a public sector body?' },
    { id: 'supply_chain', text: 'Do you provide services to other regulated organisations?' },
  ];

  // Quick assessment controls (mock)
  const quickControls = recommendations?.quick_controls ?? [
    { id: 'ac-1', name: 'Access Control Policy', framework: 'ISO 27001' },
    { id: 'ac-2', name: 'User Access Management', framework: 'ISO 27001' },
    { id: 'cm-1', name: 'Change Management', framework: 'ISO 27001' },
    { id: 'ir-1', name: 'Incident Response Plan', framework: 'ISO 27001' },
    { id: 'ra-1', name: 'Risk Assessment Process', framework: 'ISO 27001' },
    { id: 'bc-1', name: 'Business Continuity Plan', framework: 'Cyber Essentials' },
    { id: 'fw-1', name: 'Firewall Configuration', framework: 'Cyber Essentials' },
    { id: 'ma-1', name: 'Malware Protection', framework: 'Cyber Essentials' },
    { id: 'pa-1', name: 'Patch Management', framework: 'Cyber Essentials' },
    { id: 'sc-1', name: 'Secure Configuration', framework: 'Cyber Essentials' },
  ];

  const implementedCount = Object.values(controlStatuses).filter((s) => s === 'implemented').length;
  const assessedCount = Object.values(controlStatuses).filter((s) => s && s !== '').length;
  const liveScore = quickControls.length > 0 ? Math.round((implementedCount / quickControls.length) * 100) : 0;

  const planLimit = progress?.plan_framework_limit ?? 5;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      {/* Header */}
      <div className="bg-white border-b px-6 py-4">
        <h1 className="text-xl font-bold text-gray-900">ComplianceForge Setup</h1>
      </div>

      {/* Progress bar */}
      <div className="bg-white border-b px-6 py-4">
        <div className="flex items-center gap-1 max-w-4xl mx-auto">
          {STEPS.map((step, idx) => (
            <div key={step} className="flex-1 flex items-center">
              <div className="flex flex-col items-center flex-1">
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold ${
                    idx < currentStep
                      ? 'bg-green-600 text-white'
                      : idx === currentStep
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-200 text-gray-500'
                  }`}
                >
                  {idx < currentStep ? '\u2713' : idx + 1}
                </div>
                <span className={`text-xs mt-1 text-center ${idx === currentStep ? 'text-blue-600 font-medium' : 'text-gray-400'}`}>
                  {step}
                </span>
              </div>
              {idx < STEPS.length - 1 && (
                <div className={`h-0.5 flex-1 mx-1 ${idx < currentStep ? 'bg-green-600' : 'bg-gray-200'}`} />
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Step Content */}
      <div className="flex-1 max-w-3xl mx-auto w-full px-6 py-8">
        {/* Step 1: Organisation Profile */}
        {currentStep === 0 && (
          <div className="space-y-6">
            <h2 className="text-2xl font-bold">Organisation Profile</h2>
            <p className="text-gray-500">Tell us about your organisation to tailor your experience.</p>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">Organisation Name</label>
                <input
                  type="text"
                  value={orgProfile.name}
                  onChange={(e) => setOrgProfile({ ...orgProfile, name: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Legal Name</label>
                <input
                  type="text"
                  value={orgProfile.legal_name}
                  onChange={(e) => setOrgProfile({ ...orgProfile, legal_name: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Industry</label>
                <select
                  value={orgProfile.industry}
                  onChange={(e) => setOrgProfile({ ...orgProfile, industry: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                >
                  <option value="">Select industry</option>
                  {INDUSTRIES.map((i) => (
                    <option key={i} value={i}>{i}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Country</label>
                <input
                  type="text"
                  value={orgProfile.country}
                  onChange={(e) => setOrgProfile({ ...orgProfile, country: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                  placeholder="e.g. United Kingdom"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Employee Count</label>
                <select
                  value={orgProfile.employee_count}
                  onChange={(e) => setOrgProfile({ ...orgProfile, employee_count: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm"
                >
                  <option value="">Select range</option>
                  {EMPLOYEE_RANGES.map((r) => (
                    <option key={r} value={r}>{r}</option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        )}

        {/* Step 2: Industry Assessment */}
        {currentStep === 1 && (
          <div className="space-y-6">
            <h2 className="text-2xl font-bold">Industry Assessment</h2>
            <p className="text-gray-500">
              Answer these questions so we can recommend the right frameworks and controls for your organisation.
            </p>
            <div className="space-y-4">
              {industryQuestions.map((q: any) => (
                <div key={q.id} className="flex items-center justify-between border rounded-lg p-4 bg-white">
                  <p className="text-sm font-medium flex-1 pr-4">{q.text}</p>
                  <div className="flex gap-2 shrink-0">
                    <button
                      onClick={() => setIndustryAnswers({ ...industryAnswers, [q.id]: true })}
                      className={`px-4 py-1.5 text-sm font-medium rounded ${
                        industryAnswers[q.id] === true ? 'bg-green-600 text-white' : 'bg-gray-100 text-gray-600'
                      }`}
                    >
                      Yes
                    </button>
                    <button
                      onClick={() => setIndustryAnswers({ ...industryAnswers, [q.id]: false })}
                      className={`px-4 py-1.5 text-sm font-medium rounded ${
                        industryAnswers[q.id] === false ? 'bg-red-600 text-white' : 'bg-gray-100 text-gray-600'
                      }`}
                    >
                      No
                    </button>
                  </div>
                </div>
              ))}
            </div>

            {/* Recommendations preview */}
            {Object.keys(industryAnswers).length > 0 && (
              <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
                <p className="text-sm font-medium text-blue-800 mb-2">Based on your answers, we recommend:</p>
                <ul className="text-sm text-blue-700 list-disc list-inside">
                  {industryAnswers.processes_personal_data && <li>UK GDPR compliance framework</li>}
                  {industryAnswers.handles_payment && <li>PCI DSS 4.0 framework</li>}
                  {industryAnswers.critical_infrastructure && <li>NIS2 Directive compliance</li>}
                  <li>ISO 27001 (recommended for all organisations)</li>
                  <li>Cyber Essentials certification</li>
                </ul>
              </div>
            )}
          </div>
        )}

        {/* Step 3: Framework Selection */}
        {currentStep === 2 && (
          <div className="space-y-6">
            <h2 className="text-2xl font-bold">Select Frameworks</h2>
            <p className="text-gray-500">
              Choose the compliance frameworks relevant to your organisation.
              <span className="ml-1 font-medium">
                {selectedFrameworks.length}/{planLimit} selected (plan limit)
              </span>
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {availableFrameworks.map((fw: any) => {
                const isSelected = selectedFrameworks.includes(fw.id);
                const isAtLimit = selectedFrameworks.length >= planLimit && !isSelected;
                return (
                  <button
                    key={fw.id}
                    onClick={() => {
                      if (isSelected) {
                        setSelectedFrameworks(selectedFrameworks.filter((f) => f !== fw.id));
                      } else if (!isAtLimit) {
                        setSelectedFrameworks([...selectedFrameworks, fw.id]);
                      }
                    }}
                    disabled={isAtLimit}
                    className={`text-left border-2 rounded-lg p-4 transition-colors ${
                      isSelected
                        ? 'border-blue-600 bg-blue-50'
                        : isAtLimit
                        ? 'border-gray-200 bg-gray-50 opacity-50 cursor-not-allowed'
                        : 'border-gray-200 bg-white hover:border-blue-300'
                    }`}
                  >
                    <div className="flex items-start justify-between">
                      <div>
                        <p className="font-semibold">{fw.name}</p>
                        {fw.recommended && (
                          <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full mt-1 inline-block">
                            Recommended
                          </span>
                        )}
                      </div>
                      <div
                        className={`w-5 h-5 rounded border-2 flex items-center justify-center ${
                          isSelected ? 'border-blue-600 bg-blue-600 text-white' : 'border-gray-300'
                        }`}
                      >
                        {isSelected && <span className="text-xs">{'\u2713'}</span>}
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Step 4: Team Setup */}
        {currentStep === 3 && (
          <div className="space-y-6">
            <h2 className="text-2xl font-bold">Invite Your Team</h2>
            <p className="text-gray-500">Add team members to collaborate on compliance. You can always do this later.</p>
            <div className="border rounded-lg p-4 bg-white space-y-3">
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs font-medium mb-1">Email</label>
                  <input
                    type="email"
                    value={inviteForm.email}
                    onChange={(e) => setInviteForm({ ...inviteForm, email: e.target.value })}
                    className="w-full border rounded px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium mb-1">Name</label>
                  <input
                    type="text"
                    value={inviteForm.name}
                    onChange={(e) => setInviteForm({ ...inviteForm, name: e.target.value })}
                    className="w-full border rounded px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium mb-1">Role</label>
                  <div className="flex gap-2">
                    <select
                      value={inviteForm.role}
                      onChange={(e) => setInviteForm({ ...inviteForm, role: e.target.value })}
                      className="flex-1 border rounded px-3 py-2 text-sm"
                    >
                      <option value="viewer">Viewer</option>
                      <option value="editor">Editor</option>
                      <option value="admin">Admin</option>
                      <option value="auditor">Auditor</option>
                    </select>
                    <button
                      onClick={() => {
                        if (inviteForm.email && inviteForm.name) {
                          setTeamInvites([...teamInvites, { ...inviteForm }]);
                          setInviteForm({ email: '', name: '', role: 'viewer' });
                        }
                      }}
                      className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
                    >
                      Add
                    </button>
                  </div>
                </div>
              </div>
            </div>

            {teamInvites.length > 0 && (
              <div className="border rounded-lg overflow-hidden">
                <table className="w-full text-sm">
                  <thead className="bg-gray-50 border-b">
                    <tr>
                      <th className="text-left px-4 py-2">Name</th>
                      <th className="text-left px-4 py-2">Email</th>
                      <th className="text-left px-4 py-2">Role</th>
                      <th className="text-right px-4 py-2" />
                    </tr>
                  </thead>
                  <tbody>
                    {teamInvites.map((inv, idx) => (
                      <tr key={idx} className="border-b last:border-0">
                        <td className="px-4 py-2">{inv.name}</td>
                        <td className="px-4 py-2 text-gray-500">{inv.email}</td>
                        <td className="px-4 py-2 capitalize">{inv.role}</td>
                        <td className="px-4 py-2 text-right">
                          <button
                            onClick={() => setTeamInvites(teamInvites.filter((_, i) => i !== idx))}
                            className="text-red-500 hover:text-red-700 text-xs"
                          >
                            Remove
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {/* Step 5: Risk Appetite */}
        {currentStep === 4 && (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-2xl font-bold">Risk Appetite</h2>
                <p className="text-gray-500">Define how much risk your organisation is willing to accept per category.</p>
              </div>
              <button
                onClick={() => setRiskAppetite(Object.fromEntries(RISK_CATEGORIES.map((c) => [c, 2])))}
                className="px-3 py-1.5 text-sm font-medium rounded border border-gray-300 hover:bg-gray-50"
              >
                Use Defaults
              </button>
            </div>

            <div>
              <label className="block text-sm font-medium mb-1">Risk Matrix Size</label>
              <select
                value={matrixSize}
                onChange={(e) => setMatrixSize(parseInt(e.target.value))}
                className="border rounded px-3 py-2 text-sm"
              >
                <option value={3}>3x3</option>
                <option value={4}>4x4</option>
                <option value={5}>5x5</option>
              </select>
            </div>

            <div className="space-y-4">
              {RISK_CATEGORIES.map((cat) => (
                <div key={cat} className="flex items-center gap-4">
                  <span className="w-32 text-sm font-medium">{cat}</span>
                  <div className="flex gap-1 flex-1">
                    {APPETITE_LEVELS.map((level, idx) => (
                      <button
                        key={level}
                        onClick={() => setRiskAppetite({ ...riskAppetite, [cat]: idx })}
                        className={`flex-1 py-2 text-xs font-medium rounded transition-colors ${
                          riskAppetite[cat] === idx
                            ? idx <= 1
                              ? 'bg-green-600 text-white'
                              : idx === 2
                              ? 'bg-amber-500 text-white'
                              : 'bg-red-600 text-white'
                            : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                        }`}
                      >
                        {level}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Step 6: Quick Assessment */}
        {currentStep === 5 && (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-2xl font-bold">Quick Assessment</h2>
                <p className="text-gray-500">
                  Rate the implementation status of these key controls. {assessedCount}/{quickControls.length} assessed.
                </p>
              </div>
              <div className="text-right">
                <p className="text-3xl font-bold text-blue-600">{liveScore}%</p>
                <p className="text-xs text-gray-400">Live Score</p>
              </div>
            </div>

            <div className="space-y-3">
              {quickControls.map((ctrl: any) => (
                <div key={ctrl.id} className="border rounded-lg p-4 bg-white flex items-center justify-between gap-4">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium">{ctrl.name}</p>
                    <p className="text-xs text-gray-400">{ctrl.framework}</p>
                  </div>
                  <select
                    value={controlStatuses[ctrl.id] ?? ''}
                    onChange={(e) => setControlStatuses({ ...controlStatuses, [ctrl.id]: e.target.value })}
                    className="border rounded px-3 py-1.5 text-sm shrink-0"
                  >
                    <option value="">Select status</option>
                    {CONTROL_STATUSES.map((s) => (
                      <option key={s.value} value={s.value}>{s.label}</option>
                    ))}
                  </select>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Step 7: Summary & Launch */}
        {currentStep === 6 && (
          <div className="space-y-6">
            <h2 className="text-2xl font-bold">Ready to Launch</h2>
            <p className="text-gray-500">Review your setup before launching ComplianceForge.</p>

            <div className="space-y-4">
              <SummarySection title="Organisation" items={[
                `Name: ${orgProfile.name || '(not set)'}`,
                `Industry: ${orgProfile.industry || '(not set)'}`,
                `Country: ${orgProfile.country || '(not set)'}`,
                `Size: ${orgProfile.employee_count || '(not set)'}`,
              ]} />

              <SummarySection title="Frameworks" items={
                selectedFrameworks.length > 0
                  ? selectedFrameworks.map((id) => {
                      const fw = availableFrameworks.find((f: any) => f.id === id);
                      return fw?.name ?? id;
                    })
                  : ['No frameworks selected']
              } />

              <SummarySection title="Team" items={
                teamInvites.length > 0
                  ? teamInvites.map((i) => `${i.name} (${i.email}) - ${i.role}`)
                  : ['No team members invited']
              } />

              <SummarySection title="Risk Appetite" items={
                RISK_CATEGORIES.map((cat) => `${cat}: ${APPETITE_LEVELS[riskAppetite[cat]] ?? 'Medium'}`)
              } />

              <SummarySection title="Quick Assessment" items={[
                `${assessedCount} of ${quickControls.length} controls assessed`,
                `Initial score: ${liveScore}%`,
              ]} />
            </div>

            <div className="pt-4">
              <button
                onClick={handleLaunch}
                disabled={launching || completeOnboarding.isPending}
                className="w-full py-4 text-lg font-bold rounded-lg bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50 transition-all"
              >
                {launching || completeOnboarding.isPending ? (
                  <span className="flex items-center justify-center gap-3">
                    <span className="inline-block w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    Launching ComplianceForge...
                  </span>
                ) : (
                  'Launch ComplianceForge'
                )}
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Bottom Navigation */}
      <div className="bg-white border-t px-6 py-4">
        <div className="max-w-3xl mx-auto flex items-center justify-between">
          <button
            onClick={goBack}
            disabled={currentStep === 0}
            className="px-4 py-2 text-sm font-medium rounded border border-gray-300 hover:bg-gray-50 disabled:opacity-30"
          >
            Back
          </button>

          <div className="flex gap-2">
            {currentStep > 0 && currentStep < STEPS.length - 1 && (
              <button
                onClick={handleSkip}
                className="px-4 py-2 text-sm font-medium text-gray-500 hover:text-gray-700"
              >
                Skip
              </button>
            )}
            {currentStep < STEPS.length - 1 && (
              <button
                onClick={goNext}
                className="px-6 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700"
              >
                Next
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummarySection({ title, items }: { title: string; items: string[] }) {
  return (
    <div className="border rounded-lg p-4 bg-white">
      <h3 className="text-sm font-bold text-gray-700 mb-2">{title}</h3>
      <ul className="space-y-1">
        {items.map((item, idx) => (
          <li key={idx} className="text-sm text-gray-600">{item}</li>
        ))}
      </ul>
    </div>
  );
}
