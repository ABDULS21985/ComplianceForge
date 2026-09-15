export type DirectoryUserStatus =
  | 'active'
  | 'inactive'
  | 'locked'
  | 'pending_verification';

export type DirectoryInvitationStatus =
  | 'not_required'
  | 'ready'
  | 'sent'
  | 'accepted'
  | 'expired'
  | 'revoked';

export interface DirectoryUserReference {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
}

export interface DirectoryUser {
  id: string;
  organization_id: string;
  email: string;
  first_name: string;
  last_name: string;
  job_title?: string;
  department?: string;
  phone?: string;
  avatar_url?: string;
  status: DirectoryUserStatus;
  is_super_admin: boolean;
  timezone?: string;
  language: string;
  last_login_at?: string;
  mfa_enabled: boolean;
  manager_user_id?: string;
  manager?: DirectoryUserReference;
  employee_id?: string;
  location?: string;
  invitation_status: DirectoryInvitationStatus;
  invited_at?: string;
  invitation_expires_at?: string;
  invited_by?: string;
  suspended_at?: string;
  suspended_by?: string;
  suspension_reason?: string;
  reactivated_at?: string;
  deprovisioned_at?: string;
  deprovisioned_by?: string;
  deprovision_reason?: string;
  updated_by?: string;
  version: number;
  role_slugs: string[];
  group_count: number;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

export interface DirectoryUserListParams {
  search?: string;
  status?: DirectoryUserStatus;
  department?: string;
  location?: string;
  manager_user_id?: string;
  role_slug?: string;
  group_id?: string;
  sort_by?: 'updated_at' | 'created_at' | 'email' | 'name' | 'department' | 'last_login_at';
  sort_dir?: 'asc' | 'desc';
  page?: number;
  page_size?: number;
}
