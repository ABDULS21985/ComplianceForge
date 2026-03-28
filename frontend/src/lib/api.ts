// ComplianceForge API Client
// Singleton HTTP client with JWT auth, retry logic, and typed endpoint methods

import { getToken, clearToken } from "./auth";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1";
const IS_DEV = process.env.NODE_ENV === "development";
const MAX_RETRIES = 3;
const INITIAL_BACKOFF_MS = 500;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface PaginationParams {
  page?: number;
  page_size?: number;
  search?: string;
  sort_by?: string;
  sort_order?: "asc" | "desc";
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface ApiError {
  status: number;
  message: string;
  detail?: unknown;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function buildQuery(params?: Record<string, unknown>): string {
  if (!params) return "";
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== "") {
      if (Array.isArray(v)) {
        v.forEach((item) => qs.append(k, String(item)));
      } else {
        qs.set(k, String(v));
      }
    }
  }
  const str = qs.toString();
  return str ? `?${str}` : "";
}

// ---------------------------------------------------------------------------
// API Client
// ---------------------------------------------------------------------------

class ApiClient {
  private static instance: ApiClient;

  private constructor() {}

  static getInstance(): ApiClient {
    if (!ApiClient.instance) {
      ApiClient.instance = new ApiClient();
    }
    return ApiClient.instance;
  }

  // ---- Core request method ------------------------------------------------

  async request<T>(
    method: string,
    path: string,
    options: {
      body?: unknown;
      params?: Record<string, unknown>;
      signal?: AbortSignal;
      headers?: Record<string, string>;
      isFormData?: boolean;
      retries?: number;
    } = {}
  ): Promise<T> {
    const { body, params, signal, headers: extraHeaders, isFormData, retries = 0 } = options;
    const url = `${BASE_URL}${path}${buildQuery(params)}`;

    const headers: Record<string, string> = { ...extraHeaders };
    const token = getToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    if (!isFormData) {
      headers["Content-Type"] = "application/json";
    }

    const init: RequestInit = { method, headers, signal };
    if (body !== undefined) {
      init.body = isFormData ? (body as FormData) : JSON.stringify(body);
    }

    if (IS_DEV) {
      console.log(`[API] ${method} ${url}`, body ?? "");
    }

    let response: Response;
    try {
      response = await fetch(url, init);
    } catch (err) {
      // Network error – retry on 5xx-like failures
      if (retries < MAX_RETRIES) {
        await sleep(INITIAL_BACKOFF_MS * Math.pow(2, retries));
        return this.request<T>(method, path, { ...options, retries: retries + 1 });
      }
      throw err;
    }

    // 401 → clear session and redirect
    if (response.status === 401) {
      clearToken();
      if (typeof window !== "undefined") {
        window.location.href = "/login";
      }
      throw { status: 401, message: "Unauthorized" } satisfies ApiError;
    }

    // 5xx → retry with backoff
    if (response.status >= 500 && retries < MAX_RETRIES) {
      await sleep(INITIAL_BACKOFF_MS * Math.pow(2, retries));
      return this.request<T>(method, path, { ...options, retries: retries + 1 });
    }

    // Parse body
    const contentType = response.headers.get("content-type") ?? "";
    let data: unknown;
    if (contentType.includes("application/json")) {
      data = await response.json();
    } else if (contentType.includes("application/octet-stream") || contentType.includes("application/pdf")) {
      data = await response.blob();
    } else {
      data = await response.text();
    }

    if (!response.ok) {
      const apiError: ApiError = {
        status: response.status,
        message: (data as Record<string, string>)?.message ?? response.statusText,
        detail: data,
      };
      throw apiError;
    }

    return data as T;
  }

  // ---- Convenience HTTP verbs -------------------------------------------

  get<T>(path: string, params?: Record<string, unknown>, signal?: AbortSignal) {
    return this.request<T>("GET", path, { params, signal });
  }

  post<T>(path: string, body?: unknown, opts?: { signal?: AbortSignal; isFormData?: boolean }) {
    return this.request<T>("POST", path, { body, ...opts });
  }

  put<T>(path: string, body?: unknown, signal?: AbortSignal) {
    return this.request<T>("PUT", path, { body, signal });
  }

  patch<T>(path: string, body?: unknown, signal?: AbortSignal) {
    return this.request<T>("PATCH", path, { body, signal });
  }

  delete<T>(path: string, signal?: AbortSignal) {
    return this.request<T>("DELETE", path, { signal });
  }

  // ========================================================================
  // AUTH
  // ========================================================================

  auth = {
    login: (data: { email: string; password: string }) =>
      this.post<{ access_token: string; refresh_token: string; user: unknown }>("/auth/login", data),

    register: (data: { email: string; password: string; first_name: string; last_name: string }) =>
      this.post<{ access_token: string; refresh_token: string; user: unknown }>("/auth/register", data),

    refresh: (refreshToken: string) =>
      this.post<{ access_token: string; refresh_token: string }>("/auth/refresh", { refresh_token: refreshToken }),

    logout: () => this.post<void>("/auth/logout"),

    me: () => this.get<unknown>("/auth/me"),
  };

  // ========================================================================
  // FRAMEWORKS
  // ========================================================================

  frameworks = {
    list: (params?: PaginationParams) =>
      this.get<PaginatedResponse<unknown>>("/frameworks", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/frameworks/${id}`),

    getControls: (id: string, params?: PaginationParams) =>
      this.get<PaginatedResponse<unknown>>(`/frameworks/${id}/controls`, params as Record<string, unknown>),

    searchControls: (id: string, query: string, params?: PaginationParams) =>
      this.get<PaginatedResponse<unknown>>(`/frameworks/${id}/controls/search`, { query, ...params } as Record<string, unknown>),

    getImplementations: (id: string, params?: PaginationParams) =>
      this.get<PaginatedResponse<unknown>>(`/frameworks/${id}/implementations`, params as Record<string, unknown>),
  };

  // ========================================================================
  // COMPLIANCE
  // ========================================================================

  compliance = {
    scores: (params?: { framework_id?: string }) =>
      this.get<unknown>("/compliance/scores", params as Record<string, unknown>),

    gaps: (params?: { framework_id?: string }) =>
      this.get<unknown>("/compliance/gaps", params as Record<string, unknown>),

    crossMapping: (sourceFrameworkId: string, targetFrameworkId: string) =>
      this.get<unknown>("/compliance/cross-mapping", {
        source_framework_id: sourceFrameworkId,
        target_framework_id: targetFrameworkId,
      }),
  };

  // ========================================================================
  // RISKS
  // ========================================================================

  risks = {
    list: (params?: PaginationParams & { status?: string; category_id?: string; owner_id?: string }) =>
      this.get<PaginatedResponse<unknown>>("/risks", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/risks/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/risks", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/risks/${id}`, data),

    getHeatmap: () =>
      this.get<unknown>("/risks/heatmap"),
  };

  // ========================================================================
  // POLICIES
  // ========================================================================

  policies = {
    list: (params?: PaginationParams & { status?: string; category_id?: string }) =>
      this.get<PaginatedResponse<unknown>>("/policies", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/policies/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/policies", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/policies/${id}`, data),

    publish: (id: string) =>
      this.post<unknown>(`/policies/${id}/publish`),

    attest: (id: string, data: { attested: boolean; comment?: string }) =>
      this.post<unknown>(`/policies/${id}/attest`, data),

    attestationStats: (id: string) =>
      this.get<unknown>(`/policies/${id}/attestation-stats`),
  };

  // ========================================================================
  // AUDITS
  // ========================================================================

  audits = {
    list: (params?: PaginationParams & { status?: string; audit_type?: string }) =>
      this.get<PaginatedResponse<unknown>>("/audits", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/audits/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/audits", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/audits/${id}`, data),

    createFinding: (auditId: string, data: unknown) =>
      this.post<unknown>(`/audits/${auditId}/findings`, data),

    getFindings: (auditId: string, params?: PaginationParams) =>
      this.get<PaginatedResponse<unknown>>(`/audits/${auditId}/findings`, params as Record<string, unknown>),

    findingsStats: (auditId: string) =>
      this.get<unknown>(`/audits/${auditId}/findings/stats`),
  };

  // ========================================================================
  // INCIDENTS
  // ========================================================================

  incidents = {
    list: (params?: PaginationParams & { status?: string; severity?: string; is_data_breach?: boolean }) =>
      this.get<PaginatedResponse<unknown>>("/incidents", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/incidents/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/incidents", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/incidents/${id}`, data),

    updateStatus: (id: string, data: { status: string; notes?: string }) =>
      this.patch<unknown>(`/incidents/${id}/status`, data),

    notifyDPA: (id: string, data?: { message?: string }) =>
      this.post<unknown>(`/incidents/${id}/notify-dpa`, data),

    nis2EarlyWarning: (id: string, data?: unknown) =>
      this.post<unknown>(`/incidents/${id}/nis2-early-warning`, data),

    stats: () =>
      this.get<unknown>("/incidents/stats"),

    urgentBreaches: () =>
      this.get<unknown>("/incidents/urgent-breaches"),
  };

  // ========================================================================
  // VENDORS
  // ========================================================================

  vendors = {
    list: (params?: PaginationParams & { risk_tier?: string; status?: string }) =>
      this.get<PaginatedResponse<unknown>>("/vendors", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/vendors/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/vendors", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/vendors/${id}`, data),

    stats: () =>
      this.get<unknown>("/vendors/stats"),
  };

  // ========================================================================
  // ASSETS
  // ========================================================================

  assets = {
    list: (params?: PaginationParams & { asset_type?: string; criticality?: string }) =>
      this.get<PaginatedResponse<unknown>>("/assets", params as Record<string, unknown>),

    get: (id: string) =>
      this.get<unknown>(`/assets/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/assets", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/assets/${id}`, data),

    stats: () =>
      this.get<unknown>("/assets/stats"),
  };

  // ========================================================================
  // CONTROLS
  // ========================================================================

  controls = {
    get: (id: string) =>
      this.get<unknown>(`/controls/${id}`),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/controls/${id}`, data),

    uploadEvidence: (controlId: string, formData: FormData) =>
      this.post<unknown>(`/controls/${controlId}/evidence`, formData, { isFormData: true }),

    listEvidence: (controlId: string, params?: PaginationParams) =>
      this.get<PaginatedResponse<unknown>>(`/controls/${controlId}/evidence`, params as Record<string, unknown>),

    downloadEvidence: (controlId: string, evidenceId: string) =>
      this.get<Blob>(`/controls/${controlId}/evidence/${evidenceId}/download`),

    reviewEvidence: (controlId: string, evidenceId: string, data: { status: string; comment?: string }) =>
      this.post<unknown>(`/controls/${controlId}/evidence/${evidenceId}/review`, data),

    recordTest: (controlId: string, data: unknown) =>
      this.post<unknown>(`/controls/${controlId}/tests`, data),
  };

  // ========================================================================
  // SETTINGS
  // ========================================================================

  settings = {
    getOrg: () =>
      this.get<unknown>("/settings/organization"),

    updateOrg: (data: unknown) =>
      this.put<unknown>("/settings/organization", data),

    listUsers: (params?: PaginationParams & { is_active?: boolean; role_id?: string }) =>
      this.get<PaginatedResponse<unknown>>("/settings/users", params as Record<string, unknown>),

    getUser: (id: string) =>
      this.get<unknown>(`/settings/users/${id}`),

    createUser: (data: unknown) =>
      this.post<unknown>("/settings/users", data),

    updateUser: (id: string, data: unknown) =>
      this.put<unknown>(`/settings/users/${id}`, data),

    deactivateUser: (id: string) =>
      this.post<unknown>(`/settings/users/${id}/deactivate`),

    assignRole: (userId: string, data: { role_ids: string[] }) =>
      this.post<unknown>(`/settings/users/${userId}/roles`, data),

    listRoles: () =>
      this.get<unknown[]>("/settings/roles"),

    auditLog: (params?: PaginationParams & { user_id?: string; action?: string; from_date?: string; to_date?: string }) =>
      this.get<PaginatedResponse<unknown>>("/settings/audit-log", params as Record<string, unknown>),
  };

  // ========================================================================
  // NOTIFICATIONS
  // ========================================================================

  notifications = {
    list: (params?: { page?: number; page_size?: number }) =>
      this.get<unknown>("/notifications", params as Record<string, unknown>),

    markAsRead: (id: string) =>
      this.put<void>(`/notifications/${id}/read`),

    markAllAsRead: () =>
      this.put<void>("/notifications/read-all"),

    unreadCount: () =>
      this.get<{ count: number }>("/notifications/unread-count"),

    getPreferences: () =>
      this.get<unknown>("/notifications/preferences"),

    updatePreferences: (prefs: unknown) =>
      this.put<unknown>("/notifications/preferences", prefs),
  };

  // ========================================================================
  // DSR (Data Subject Requests)
  // ========================================================================

  dsr = {
    list: (params?: Record<string, unknown>) =>
      this.get<unknown>("/dsr", params),

    get: (id: string) =>
      this.get<unknown>(`/dsr/${id}`),

    create: (data: unknown) =>
      this.post<unknown>("/dsr", data),

    update: (id: string, data: unknown) =>
      this.put<unknown>(`/dsr/${id}`, data),

    verifyIdentity: (id: string, data: unknown) =>
      this.post<unknown>(`/dsr/${id}/verify-identity`, data),

    assign: (id: string, data: unknown) =>
      this.post<unknown>(`/dsr/${id}/assign`, data),

    extend: (id: string, data: unknown) =>
      this.post<unknown>(`/dsr/${id}/extend`, data),

    complete: (id: string, data: unknown) =>
      this.post<unknown>(`/dsr/${id}/complete`, data),

    reject: (id: string, data: unknown) =>
      this.post<unknown>(`/dsr/${id}/reject`, data),

    updateTask: (id: string, taskId: string, data: unknown) =>
      this.put<unknown>(`/dsr/${id}/tasks/${taskId}`, data),

    dashboard: () =>
      this.get<unknown>("/dsr/dashboard"),

    overdue: () =>
      this.get<unknown>("/dsr/overdue"),

    templates: () =>
      this.get<unknown>("/dsr/templates"),
  };

  // ========================================================================
  // NIS2
  // ========================================================================

  nis2 = {
    getAssessment: () =>
      this.get<unknown>("/nis2/assessment"),

    createAssessment: (data: unknown) =>
      this.post<unknown>("/nis2/assessment", data),

    listIncidentReports: () =>
      this.get<unknown>("/nis2/incidents"),

    getIncidentReport: (id: string) =>
      this.get<unknown>(`/nis2/incidents/${id}`),

    submitEarlyWarning: (id: string, data: unknown) =>
      this.post<unknown>(`/nis2/incidents/${id}/early-warning`, data),

    submitNotification: (id: string, data: unknown) =>
      this.post<unknown>(`/nis2/incidents/${id}/notification`, data),

    submitFinalReport: (id: string, data: unknown) =>
      this.post<unknown>(`/nis2/incidents/${id}/final-report`, data),

    getMeasures: () =>
      this.get<unknown>("/nis2/measures"),

    updateMeasure: (id: string, data: unknown) =>
      this.put<unknown>(`/nis2/measures/${id}`, data),

    getManagement: () =>
      this.get<unknown>("/nis2/management"),

    recordTraining: (data: unknown) =>
      this.post<unknown>("/nis2/management", data),

    dashboard: () =>
      this.get<unknown>("/nis2/dashboard"),
  };

  // ========================================================================
  // MONITORING
  // ========================================================================

  monitoring = {
    listConfigs: () =>
      this.get<unknown>("/monitoring/configs"),

    createConfig: (data: unknown) =>
      this.post<unknown>("/monitoring/configs", data),

    updateConfig: (id: string, data: unknown) =>
      this.put<unknown>(`/monitoring/configs/${id}`, data),

    runNow: (id: string) =>
      this.post<unknown>(`/monitoring/configs/${id}/run-now`),

    getHistory: (id: string, params?: Record<string, unknown>) =>
      this.get<unknown>(`/monitoring/configs/${id}/history`, params),

    listMonitors: () =>
      this.get<unknown>("/monitoring/monitors"),

    createMonitor: (data: unknown) =>
      this.post<unknown>("/monitoring/monitors", data),

    updateMonitor: (id: string, data: unknown) =>
      this.put<unknown>(`/monitoring/monitors/${id}`, data),

    getMonitorResults: (id: string) =>
      this.get<unknown>(`/monitoring/monitors/${id}/results`),

    listDrift: (params?: Record<string, unknown>) =>
      this.get<unknown>("/monitoring/drift", params),

    acknowledgeDrift: (id: string) =>
      this.put<unknown>(`/monitoring/drift/${id}/acknowledge`),

    resolveDrift: (id: string, data: unknown) =>
      this.put<unknown>(`/monitoring/drift/${id}/resolve`, data),

    dashboard: () =>
      this.get<unknown>("/monitoring/dashboard"),
  };

  // ========================================================================
  // REPORTS
  // ========================================================================

  reports = {
    compliance: (params?: { framework_id?: string; format?: string }) =>
      this.get<unknown>("/reports/compliance", params as Record<string, unknown>),

    risk: (params?: { format?: string }) =>
      this.get<unknown>("/reports/risk", params as Record<string, unknown>),

    generate: (data: unknown) =>
      this.post<unknown>("/reports/generate", data),

    getRunStatus: (id: string) =>
      this.get<unknown>(`/reports/status/${id}`),

    download: (id: string) =>
      this.get<Blob>(`/reports/download/${id}`),

    listDefinitions: () =>
      this.get<unknown>("/reports/definitions"),

    createDefinition: (data: unknown) =>
      this.post<unknown>("/reports/definitions", data),

    listSchedules: () =>
      this.get<unknown>("/reports/schedules"),

    createSchedule: (data: unknown) =>
      this.post<unknown>("/reports/schedules", data),

    listHistory: (params?: Record<string, unknown>) =>
      this.get<unknown>("/reports/history", params),

    generateFromDefinition: (id: string) =>
      this.post<unknown>(`/reports/definitions/${id}/generate`),
  };

  // ========================================================================
  // WORKFLOWS
  // ========================================================================

  workflows = {
    myApprovals: (params?: any) => this.get<any>('/workflows/my-approvals', params),
    listDefinitions: () => this.get<any>('/workflows/definitions'),
    listInstances: (params?: any) => this.get<any>('/workflows/instances', params),
    getInstance: (id: string) => this.get<any>(`/workflows/instances/${id}`),
    start: (data: any) => this.post<any>('/workflows/start', data),
    cancelInstance: (id: string, data: any) => this.post<any>(`/workflows/instances/${id}/cancel`, data),
    approveStep: (id: string, data: any) => this.post<any>(`/workflows/executions/${id}/approve`, data),
    rejectStep: (id: string, data: any) => this.post<any>(`/workflows/executions/${id}/reject`, data),
    delegateStep: (id: string, data: any) => this.post<any>(`/workflows/executions/${id}/delegate`, data),
    listDelegations: () => this.get<any>('/workflows/delegations'),
    createDelegation: (data: any) => this.post<any>('/workflows/delegations', data),
  };

  // ========================================================================
  // INTEGRATIONS
  // ========================================================================

  integrations = {
    list: () => this.get<any>('/integrations'),
    create: (data: any) => this.post<any>('/integrations', data),
    getById: (id: string) => this.get<any>(`/integrations/${id}`),
    update: (id: string, data: any) => this.put<any>(`/integrations/${id}`, data),
    remove: (id: string) => this.delete<any>(`/integrations/${id}`),
    test: (id: string) => this.post<any>(`/integrations/${id}/test`),
    sync: (id: string, data?: any) => this.post<any>(`/integrations/${id}/sync`, data),
    logs: (id: string, params?: any) => this.get<any>(`/integrations/${id}/logs`, params),
    getSSOConfig: () => this.get<any>('/settings/sso'),
    updateSSOConfig: (data: any) => this.put<any>('/settings/sso', data),
    listAPIKeys: () => this.get<any>('/settings/api-keys'),
    createAPIKey: (data: any) => this.post<any>('/settings/api-keys', data),
    revokeAPIKey: (id: string) => this.delete<any>(`/settings/api-keys/${id}`),
  };

  // ========================================================================
  // ACCESS (ABAC)
  // ========================================================================

  access = {
    listPolicies: () => this.get<any>('/access/policies'),
    createPolicy: (data: any) => this.post<any>('/access/policies', data),
    updatePolicy: (id: string, data: any) => this.put<any>(`/access/policies/${id}`, data),
    deletePolicy: (id: string) => this.delete<any>(`/access/policies/${id}`),
    assignPolicy: (id: string, data: any) => this.post<any>(`/access/policies/${id}/assignments`, data),
    removeAssignment: (policyId: string, assignmentId: string) => this.delete<any>(`/access/policies/${policyId}/assignments/${assignmentId}`),
    testEvaluate: (data: any) => this.post<any>('/access/evaluate', data),
    auditLog: (params?: any) => this.get<any>('/access/audit-log', params),
    myPermissions: () => this.get<any>('/access/my-permissions'),
    fieldPermissions: (resourceType: string) => this.get<any>(`/access/field-permissions?resource_type=${resourceType}`),
  };

  // ========================================================================
  // ONBOARDING
  // ========================================================================

  onboarding = {
    getProgress: () => this.get<any>('/onboard/progress'),
    saveStep: (step: number, data: any) => this.put<any>(`/onboard/step/${step}`, data),
    skipStep: (step: number) => this.post<any>(`/onboard/step/${step}/skip`),
    complete: () => this.post<any>('/onboard/complete'),
    getRecommendations: () => this.get<any>('/onboard/recommendations'),
  };

  // ========================================================================
  // SUBSCRIPTION
  // ========================================================================

  subscription = {
    get: () => this.get<any>('/subscription'),
    changePlan: (data: any) => this.put<any>('/subscription/plan', data),
    cancel: (data: any) => this.post<any>('/subscription/cancel', data),
    listPlans: () => this.get<any>('/subscription/plans'),
    usage: () => this.get<any>('/subscription/usage'),
  };

  // ========================================================================
  // DASHBOARD
  // ========================================================================

  dashboard = {
    summary: () =>
      this.get<unknown>("/dashboard/summary"),
  };
}

// ---------------------------------------------------------------------------
// Export singleton
// ---------------------------------------------------------------------------

export const api = ApiClient.getInstance();
export default api;
