export interface PermissionGrant {
  id?: string;
  resource: string;
  action: string;
  description?: string;
}

export interface ManagedRole {
  id: string;
  organization_id?: string;
  name: string;
  slug: string;
  description?: string;
  is_system_role: boolean;
  is_custom: boolean;
  version: number;
  created_by?: string;
  updated_by?: string;
  permissions: PermissionGrant[];
  assigned_users: number;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

export interface ManagedRoleCreateInput {
  name: string;
  slug?: string;
  description?: string;
  permissions: PermissionGrant[];
}

export interface ManagedRolePatch {
  name?: string;
  slug?: string;
  description?: string;
  clear_description?: boolean;
  permissions?: PermissionGrant[];
  expected_version: number;
}

export interface ManagedRoleCloneInput {
  name: string;
  slug?: string;
  description?: string;
}

export interface ManagedRoleListParams {
  search?: string;
  include_system?: boolean;
  page?: number;
  page_size?: number;
}

export interface ManagedRoleImpact {
  role_id: string;
  assigned_users: number;
  current_permissions: number;
  proposed_permissions: number;
  added: PermissionGrant[];
  removed: PermissionGrant[];
}

export interface ManagedRoleAssignment {
  role_id: string;
  user_id: string;
  email: string;
  first_name: string;
  last_name: string;
  assigned_by?: string;
  assigned_at: string;
}

export interface ManagedRoleAssignmentInput {
  user_id: string;
  reason: string;
}

export interface ManagedRoleUnassignmentInput {
  reason: string;
}

export interface RoleChangeEvent {
  id: string;
  role_id: string;
  target_user_id?: string;
  event_type: string;
  actor_user_id: string;
  role_version: number;
  reason?: string;
  before_state?: unknown;
  after_state?: unknown;
  created_at: string;
}
