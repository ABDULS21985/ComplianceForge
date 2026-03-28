// ComplianceForge React Query Hooks
// Wraps every API endpoint with proper caching, invalidation, and toast notifications

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type PaginationParams } from "./api";

// ---------------------------------------------------------------------------
// Cache key factories
// ---------------------------------------------------------------------------

export const queryKeys = {
  // Auth
  me: ["auth", "me"] as const,

  // Dashboard
  dashboard: ["dashboard"] as const,

  // Frameworks
  frameworks: ["frameworks"] as const,
  frameworksList: (params?: PaginationParams) => ["frameworks", "list", params] as const,
  framework: (id: string) => ["frameworks", id] as const,
  frameworkControls: (id: string, params?: PaginationParams) => ["frameworks", id, "controls", params] as const,
  frameworkImplementations: (id: string, params?: PaginationParams) => ["frameworks", id, "implementations", params] as const,

  // Compliance
  complianceScores: (params?: { framework_id?: string }) => ["compliance", "scores", params] as const,
  complianceGaps: (params?: { framework_id?: string }) => ["compliance", "gaps", params] as const,
  crossMapping: (source: string, target: string) => ["compliance", "cross-mapping", source, target] as const,

  // Risks
  risks: ["risks"] as const,
  risksList: (params?: Record<string, unknown>) => ["risks", "list", params] as const,
  risk: (id: string) => ["risks", id] as const,
  riskHeatmap: ["risks", "heatmap"] as const,

  // Policies
  policies: ["policies"] as const,
  policiesList: (params?: Record<string, unknown>) => ["policies", "list", params] as const,
  policy: (id: string) => ["policies", id] as const,
  policyAttestationStats: (id: string) => ["policies", id, "attestation-stats"] as const,

  // Audits
  audits: ["audits"] as const,
  auditsList: (params?: Record<string, unknown>) => ["audits", "list", params] as const,
  audit: (id: string) => ["audits", id] as const,
  auditFindings: (auditId: string, params?: PaginationParams) => ["audits", auditId, "findings", params] as const,
  auditFindingsStats: (auditId: string) => ["audits", auditId, "findings", "stats"] as const,

  // Incidents
  incidents: ["incidents"] as const,
  incidentsList: (params?: Record<string, unknown>) => ["incidents", "list", params] as const,
  incident: (id: string) => ["incidents", id] as const,
  incidentStats: ["incidents", "stats"] as const,
  urgentBreaches: ["incidents", "urgent-breaches"] as const,

  // Vendors
  vendors: ["vendors"] as const,
  vendorsList: (params?: Record<string, unknown>) => ["vendors", "list", params] as const,
  vendor: (id: string) => ["vendors", id] as const,
  vendorStats: ["vendors", "stats"] as const,

  // Assets
  assets: ["assets"] as const,
  assetsList: (params?: Record<string, unknown>) => ["assets", "list", params] as const,
  asset: (id: string) => ["assets", id] as const,
  assetStats: ["assets", "stats"] as const,

  // Controls
  control: (id: string) => ["controls", id] as const,
  controlEvidence: (controlId: string, params?: PaginationParams) => ["controls", controlId, "evidence", params] as const,

  // Settings
  org: ["settings", "organization"] as const,
  users: ["settings", "users"] as const,
  usersList: (params?: Record<string, unknown>) => ["settings", "users", "list", params] as const,
  user: (id: string) => ["settings", "users", id] as const,
  roles: ["settings", "roles"] as const,
  auditLog: (params?: Record<string, unknown>) => ["settings", "audit-log", params] as const,

  // Reports
  complianceReport: (params?: Record<string, unknown>) => ["reports", "compliance", params] as const,
  riskReport: (params?: Record<string, unknown>) => ["reports", "risk", params] as const,
} as const;

// ---------------------------------------------------------------------------
// DASHBOARD
// ---------------------------------------------------------------------------

export function useDashboard(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.dashboard,
    queryFn: () => api.dashboard.summary(),
    staleTime: 30 * 1000,
    refetchOnWindowFocus: true,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// AUTH
// ---------------------------------------------------------------------------

export function useMe(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.me,
    queryFn: () => api.auth.me(),
    staleTime: 5 * 60 * 1000,
    ...options,
  });
}

export function useLogin() {
  return useMutation({
    mutationFn: (data: { email: string; password: string }) => api.auth.login(data),
    onError: () => {
      toast.error("Login failed. Please check your credentials.");
    },
  });
}

export function useRegister() {
  return useMutation({
    mutationFn: (data: { email: string; password: string; first_name: string; last_name: string }) =>
      api.auth.register(data),
    onError: () => {
      toast.error("Registration failed. Please try again.");
    },
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.auth.logout(),
    onSuccess: () => {
      qc.clear();
    },
  });
}

// ---------------------------------------------------------------------------
// FRAMEWORKS
// ---------------------------------------------------------------------------

export function useFrameworks(params?: PaginationParams, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.frameworksList(params),
    queryFn: () => api.frameworks.list(params),
    staleTime: 5 * 60 * 1000,
    ...options,
  });
}

export function useFramework(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.framework(id),
    queryFn: () => api.frameworks.get(id),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    ...options,
  });
}

export function useFrameworkControls(
  id: string,
  params?: PaginationParams,
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.frameworkControls(id, params),
    queryFn: () => api.frameworks.getControls(id, params),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    ...options,
  });
}

export function useFrameworkImplementations(
  id: string,
  params?: PaginationParams,
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.frameworkImplementations(id, params),
    queryFn: () => api.frameworks.getImplementations(id, params),
    enabled: !!id,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// COMPLIANCE
// ---------------------------------------------------------------------------

export function useComplianceScores(params?: { framework_id?: string }, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.complianceScores(params),
    queryFn: () => api.compliance.scores(params),
    staleTime: 30 * 1000,
    ...options,
  });
}

export function useComplianceGaps(params?: { framework_id?: string }, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.complianceGaps(params),
    queryFn: () => api.compliance.gaps(params),
    ...options,
  });
}

export function useCrossMapping(sourceId: string, targetId: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.crossMapping(sourceId, targetId),
    queryFn: () => api.compliance.crossMapping(sourceId, targetId),
    enabled: !!sourceId && !!targetId,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// RISKS
// ---------------------------------------------------------------------------

export function useRisks(
  params?: PaginationParams & { status?: string; category_id?: string; owner_id?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.risksList(params as Record<string, unknown>),
    queryFn: () => api.risks.list(params),
    ...options,
  });
}

export function useRisk(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.risk(id),
    queryFn: () => api.risks.get(id),
    enabled: !!id,
    ...options,
  });
}

export function useCreateRisk() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.risks.create(data),
    onSuccess: () => {
      toast.success("Risk created successfully.");
      qc.invalidateQueries({ queryKey: queryKeys.risks });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
      qc.invalidateQueries({ queryKey: queryKeys.riskHeatmap });
    },
    onError: () => {
      toast.error("Failed to create risk.");
    },
  });
}

export function useUpdateRisk() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.risks.update(id, data),
    onSuccess: (_data, variables) => {
      toast.success("Risk updated successfully.");
      qc.invalidateQueries({ queryKey: queryKeys.risk(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.risks });
      qc.invalidateQueries({ queryKey: queryKeys.riskHeatmap });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
    onError: () => {
      toast.error("Failed to update risk.");
    },
  });
}

export function useRiskHeatmap(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.riskHeatmap,
    queryFn: () => api.risks.getHeatmap(),
    staleTime: 30 * 1000,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// POLICIES
// ---------------------------------------------------------------------------

export function usePolicies(
  params?: PaginationParams & { status?: string; category_id?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.policiesList(params as Record<string, unknown>),
    queryFn: () => api.policies.list(params),
    ...options,
  });
}

export function usePolicy(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.policy(id),
    queryFn: () => api.policies.get(id),
    enabled: !!id,
    ...options,
  });
}

export function useCreatePolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.policies.create(data),
    onSuccess: () => {
      toast.success("Policy created successfully.");
      qc.invalidateQueries({ queryKey: queryKeys.policies });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
    onError: () => {
      toast.error("Failed to create policy.");
    },
  });
}

export function useUpdatePolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.policies.update(id, data),
    onSuccess: (_data, variables) => {
      toast.success("Policy updated.");
      qc.invalidateQueries({ queryKey: queryKeys.policy(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.policies });
    },
    onError: () => {
      toast.error("Failed to update policy.");
    },
  });
}

export function usePublishPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.policies.publish(id),
    onSuccess: (_data, id) => {
      toast.success("Policy published.");
      qc.invalidateQueries({ queryKey: queryKeys.policy(id) });
      qc.invalidateQueries({ queryKey: queryKeys.policies });
    },
    onError: () => {
      toast.error("Failed to publish policy.");
    },
  });
}

export function useAttestPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { attested: boolean; comment?: string } }) =>
      api.policies.attest(id, data),
    onSuccess: (_data, variables) => {
      toast.success("Attestation recorded.");
      qc.invalidateQueries({ queryKey: queryKeys.policyAttestationStats(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.policy(variables.id) });
    },
    onError: () => {
      toast.error("Failed to record attestation.");
    },
  });
}

export function usePolicyAttestationStats(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.policyAttestationStats(id),
    queryFn: () => api.policies.attestationStats(id),
    enabled: !!id,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// AUDITS
// ---------------------------------------------------------------------------

export function useAudits(
  params?: PaginationParams & { status?: string; audit_type?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.auditsList(params as Record<string, unknown>),
    queryFn: () => api.audits.list(params),
    ...options,
  });
}

export function useAudit(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.audit(id),
    queryFn: () => api.audits.get(id),
    enabled: !!id,
    ...options,
  });
}

export function useCreateAudit() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.audits.create(data),
    onSuccess: () => {
      toast.success("Audit created.");
      qc.invalidateQueries({ queryKey: queryKeys.audits });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
    onError: () => {
      toast.error("Failed to create audit.");
    },
  });
}

export function useCreateFinding(auditId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.audits.createFinding(auditId, data),
    onSuccess: () => {
      toast.success("Finding created.");
      qc.invalidateQueries({ queryKey: queryKeys.auditFindings(auditId) });
      qc.invalidateQueries({ queryKey: queryKeys.auditFindingsStats(auditId) });
      qc.invalidateQueries({ queryKey: queryKeys.audit(auditId) });
    },
    onError: () => {
      toast.error("Failed to create finding.");
    },
  });
}

export function useAuditFindings(auditId: string, params?: PaginationParams, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.auditFindings(auditId, params),
    queryFn: () => api.audits.getFindings(auditId, params),
    enabled: !!auditId,
    ...options,
  });
}

export function useAuditFindingsStats(auditId: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.auditFindingsStats(auditId),
    queryFn: () => api.audits.findingsStats(auditId),
    enabled: !!auditId,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// INCIDENTS
// ---------------------------------------------------------------------------

export function useIncidents(
  params?: PaginationParams & { status?: string; severity?: string; is_data_breach?: boolean },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.incidentsList(params as Record<string, unknown>),
    queryFn: () => api.incidents.list(params),
    refetchOnWindowFocus: true,
    ...options,
  });
}

export function useIncident(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.incident(id),
    queryFn: () => api.incidents.get(id),
    enabled: !!id,
    refetchOnWindowFocus: true,
    ...options,
  });
}

export function useCreateIncident() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.incidents.create(data),
    onSuccess: () => {
      toast.success("Incident reported.");
      qc.invalidateQueries({ queryKey: queryKeys.incidents });
      qc.invalidateQueries({ queryKey: queryKeys.incidentStats });
      qc.invalidateQueries({ queryKey: queryKeys.urgentBreaches });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
    onError: () => {
      toast.error("Failed to report incident.");
    },
  });
}

export function useUpdateIncidentStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { status: string; notes?: string } }) =>
      api.incidents.updateStatus(id, data),
    // Optimistic update for status changes
    onMutate: async ({ id, data }) => {
      await qc.cancelQueries({ queryKey: queryKeys.incident(id) });
      const previous = qc.getQueryData(queryKeys.incident(id));
      qc.setQueryData(queryKeys.incident(id), (old: Record<string, unknown> | undefined) =>
        old ? { ...old, status: data.status } : old
      );
      return { previous, id };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        qc.setQueryData(queryKeys.incident(context.id), context.previous);
      }
      toast.error("Failed to update incident status.");
    },
    onSuccess: (_data, variables) => {
      toast.success("Incident status updated.");
      qc.invalidateQueries({ queryKey: queryKeys.incident(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.incidents });
      qc.invalidateQueries({ queryKey: queryKeys.incidentStats });
      qc.invalidateQueries({ queryKey: queryKeys.urgentBreaches });
    },
  });
}

export function useNotifyDPA() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data?: { message?: string } }) =>
      api.incidents.notifyDPA(id, data),
    onSuccess: (_data, variables) => {
      toast.success("DPA notification sent.");
      qc.invalidateQueries({ queryKey: queryKeys.incident(variables.id) });
    },
    onError: () => {
      toast.error("Failed to send DPA notification.");
    },
  });
}

export function useNis2EarlyWarning() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data?: unknown }) =>
      api.incidents.nis2EarlyWarning(id, data),
    onSuccess: (_data, variables) => {
      toast.success("NIS2 early warning submitted.");
      qc.invalidateQueries({ queryKey: queryKeys.incident(variables.id) });
    },
    onError: () => {
      toast.error("Failed to submit NIS2 early warning.");
    },
  });
}

export function useIncidentStats(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.incidentStats,
    queryFn: () => api.incidents.stats(),
    staleTime: 30 * 1000,
    refetchOnWindowFocus: true,
    ...options,
  });
}

export function useUrgentBreaches(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.urgentBreaches,
    queryFn: () => api.incidents.urgentBreaches(),
    refetchInterval: 60_000,
    refetchOnWindowFocus: true,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// VENDORS
// ---------------------------------------------------------------------------

export function useVendors(
  params?: PaginationParams & { risk_tier?: string; status?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.vendorsList(params as Record<string, unknown>),
    queryFn: () => api.vendors.list(params),
    ...options,
  });
}

export function useVendor(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.vendor(id),
    queryFn: () => api.vendors.get(id),
    enabled: !!id,
    ...options,
  });
}

export function useCreateVendor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.vendors.create(data),
    onSuccess: () => {
      toast.success("Vendor onboarded.");
      qc.invalidateQueries({ queryKey: queryKeys.vendors });
      qc.invalidateQueries({ queryKey: queryKeys.vendorStats });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
    onError: () => {
      toast.error("Failed to onboard vendor.");
    },
  });
}

export function useUpdateVendor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.vendors.update(id, data),
    onSuccess: (_data, variables) => {
      toast.success("Vendor updated.");
      qc.invalidateQueries({ queryKey: queryKeys.vendor(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.vendors });
      qc.invalidateQueries({ queryKey: queryKeys.vendorStats });
    },
    onError: () => {
      toast.error("Failed to update vendor.");
    },
  });
}

export function useVendorStats(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.vendorStats,
    queryFn: () => api.vendors.stats(),
    staleTime: 30 * 1000,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// ASSETS
// ---------------------------------------------------------------------------

export function useAssets(
  params?: PaginationParams & { asset_type?: string; criticality?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.assetsList(params as Record<string, unknown>),
    queryFn: () => api.assets.list(params),
    ...options,
  });
}

export function useAsset(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.asset(id),
    queryFn: () => api.assets.get(id),
    enabled: !!id,
    ...options,
  });
}

export function useCreateAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.assets.create(data),
    onSuccess: () => {
      toast.success("Asset registered.");
      qc.invalidateQueries({ queryKey: queryKeys.assets });
      qc.invalidateQueries({ queryKey: queryKeys.assetStats });
      qc.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
    onError: () => {
      toast.error("Failed to register asset.");
    },
  });
}

export function useUpdateAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.assets.update(id, data),
    onSuccess: (_data, variables) => {
      toast.success("Asset updated.");
      qc.invalidateQueries({ queryKey: queryKeys.asset(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.assets });
      qc.invalidateQueries({ queryKey: queryKeys.assetStats });
    },
    onError: () => {
      toast.error("Failed to update asset.");
    },
  });
}

export function useAssetStats(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.assetStats,
    queryFn: () => api.assets.stats(),
    staleTime: 30 * 1000,
    ...options,
  });
}

// ---------------------------------------------------------------------------
// CONTROLS
// ---------------------------------------------------------------------------

export function useControlImplementation(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.control(id),
    queryFn: () => api.controls.get(id),
    enabled: !!id,
    ...options,
  });
}

export function useUpdateControl() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.controls.update(id, data),
    onSuccess: (_data, variables) => {
      toast.success("Control updated.");
      qc.invalidateQueries({ queryKey: queryKeys.control(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.complianceScores() });
    },
    onError: () => {
      toast.error("Failed to update control.");
    },
  });
}

export function useUploadEvidence(controlId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (formData: FormData) => api.controls.uploadEvidence(controlId, formData),
    onSuccess: () => {
      toast.success("Evidence uploaded.");
      qc.invalidateQueries({ queryKey: queryKeys.controlEvidence(controlId) });
      qc.invalidateQueries({ queryKey: queryKeys.control(controlId) });
    },
    onError: () => {
      toast.error("Failed to upload evidence.");
    },
  });
}

export function useControlEvidence(controlId: string, params?: PaginationParams, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.controlEvidence(controlId, params),
    queryFn: () => api.controls.listEvidence(controlId, params),
    enabled: !!controlId,
    ...options,
  });
}

export function useDownloadEvidence() {
  return useMutation({
    mutationFn: ({ controlId, evidenceId }: { controlId: string; evidenceId: string }) =>
      api.controls.downloadEvidence(controlId, evidenceId),
    onError: () => {
      toast.error("Failed to download evidence.");
    },
  });
}

export function useReviewEvidence(controlId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ evidenceId, data }: { evidenceId: string; data: { status: string; comment?: string } }) =>
      api.controls.reviewEvidence(controlId, evidenceId, data),
    onSuccess: () => {
      toast.success("Evidence review recorded.");
      qc.invalidateQueries({ queryKey: queryKeys.controlEvidence(controlId) });
    },
    onError: () => {
      toast.error("Failed to review evidence.");
    },
  });
}

export function useRecordControlTest(controlId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.controls.recordTest(controlId, data),
    onSuccess: () => {
      toast.success("Control test recorded.");
      qc.invalidateQueries({ queryKey: queryKeys.control(controlId) });
    },
    onError: () => {
      toast.error("Failed to record test.");
    },
  });
}

// ---------------------------------------------------------------------------
// SETTINGS
// ---------------------------------------------------------------------------

export function useOrganization(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.org,
    queryFn: () => api.settings.getOrg(),
    staleTime: 5 * 60 * 1000,
    ...options,
  });
}

export function useUpdateOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.settings.updateOrg(data),
    onSuccess: () => {
      toast.success("Organization settings updated.");
      qc.invalidateQueries({ queryKey: queryKeys.org });
    },
    onError: () => {
      toast.error("Failed to update organization settings.");
    },
  });
}

export function useUsers(
  params?: PaginationParams & { is_active?: boolean; role_id?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.usersList(params as Record<string, unknown>),
    queryFn: () => api.settings.listUsers(params),
    ...options,
  });
}

export function useUser(id: string, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.user(id),
    queryFn: () => api.settings.getUser(id),
    enabled: !!id,
    ...options,
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: unknown) => api.settings.createUser(data),
    onSuccess: () => {
      toast.success("User created.");
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
    onError: () => {
      toast.error("Failed to create user.");
    },
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: unknown }) => api.settings.updateUser(id, data),
    onSuccess: (_data, variables) => {
      toast.success("User updated.");
      qc.invalidateQueries({ queryKey: queryKeys.user(variables.id) });
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
    onError: () => {
      toast.error("Failed to update user.");
    },
  });
}

export function useDeactivateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.settings.deactivateUser(id),
    onSuccess: (_data, id) => {
      toast.success("User deactivated.");
      qc.invalidateQueries({ queryKey: queryKeys.user(id) });
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
    onError: () => {
      toast.error("Failed to deactivate user.");
    },
  });
}

export function useAssignRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, roleIds }: { userId: string; roleIds: string[] }) =>
      api.settings.assignRole(userId, { role_ids: roleIds }),
    onSuccess: (_data, variables) => {
      toast.success("Role assigned.");
      qc.invalidateQueries({ queryKey: queryKeys.user(variables.userId) });
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
    onError: () => {
      toast.error("Failed to assign role.");
    },
  });
}

export function useRoles(options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.roles,
    queryFn: () => api.settings.listRoles(),
    staleTime: 5 * 60 * 1000,
    ...options,
  });
}

export function useAuditLog(
  params?: PaginationParams & { user_id?: string; action?: string; from_date?: string; to_date?: string },
  options?: Partial<UseQueryOptions>
) {
  return useQuery({
    queryKey: queryKeys.auditLog(params as Record<string, unknown>),
    queryFn: () => api.settings.auditLog(params),
    ...options,
  });
}

// ---------------------------------------------------------------------------
// REPORTS
// ---------------------------------------------------------------------------

export function useComplianceReport(params?: { framework_id?: string; format?: string }, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.complianceReport(params as Record<string, unknown>),
    queryFn: () => api.reports.compliance(params),
    enabled: false, // on-demand only
    ...options,
  });
}

export function useRiskReport(params?: { format?: string }, options?: Partial<UseQueryOptions>) {
  return useQuery({
    queryKey: queryKeys.riskReport(params as Record<string, unknown>),
    queryFn: () => api.reports.risk(params),
    enabled: false, // on-demand only
    ...options,
  });
}
