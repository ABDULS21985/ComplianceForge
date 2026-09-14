export type AssetType = 'hardware' | 'software' | 'data' | 'service' | 'network' | 'people' | 'facility';
export type AssetCriticality = 'critical' | 'high' | 'medium' | 'low';
export type AssetClassification = 'public' | 'internal' | 'confidential' | 'restricted';
export type AssetStatus = 'active' | 'inactive' | 'decommissioned';

export interface AssetPerson {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
}

export interface Asset {
  id: string;
  organization_id: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
  asset_ref: string;
  name: string;
  asset_type: AssetType;
  category?: string;
  description?: string;
  criticality: AssetCriticality;
  owner_user_id?: string;
  owner?: AssetPerson;
  location?: string;
  ip_address?: string;
  classification: AssetClassification;
  processes_personal_data: boolean;
  linked_vendor_id?: string;
  status: AssetStatus;
  tags: string[];
  metadata: Record<string, unknown>;
  version: number;
  created_by: string;
}

export interface AssetCreateInput {
  name: string;
  asset_type: AssetType;
  category?: string;
  description?: string;
  criticality: AssetCriticality;
  owner_user_id?: string;
  location?: string;
  ip_address?: string;
  classification: AssetClassification;
  processes_personal_data: boolean;
  linked_vendor_id?: string;
  tags?: string[];
  metadata?: Record<string, unknown>;
}

export interface AssetPatch {
  name?: string;
  asset_type?: AssetType;
  category?: string;
  description?: string;
  criticality?: AssetCriticality;
  owner_user_id?: string;
  clear_owner?: boolean;
  location?: string;
  ip_address?: string;
  clear_ip_address?: boolean;
  classification?: AssetClassification;
  processes_personal_data?: boolean;
  linked_vendor_id?: string;
  clear_linked_vendor?: boolean;
  status?: AssetStatus;
  tags?: string[];
  metadata?: Record<string, unknown>;
  expected_version: number;
}

export interface AssetListParams {
  page?: number;
  page_size?: number;
  asset_type?: AssetType;
  criticality?: AssetCriticality;
  classification?: AssetClassification;
  status?: AssetStatus;
  owner_user_id?: string;
  tag?: string;
  search?: string;
  processes_personal_data?: boolean;
  sort_by?: 'updated_at' | 'created_at' | 'name' | 'asset_ref' | 'asset_type' | 'criticality';
  sort_dir?: 'asc' | 'desc';
}

export interface AssetStats {
  total: number;
  critical: number;
  personal_data: number;
  active: number;
  by_type: Partial<Record<AssetType, number>>;
}

export interface AssetLifecycleEvent {
  id: string;
  asset_id: string;
  event_type: 'created' | 'updated' | 'status_changed' | 'decommissioned' | 'deleted';
  actor_user_id: string;
  asset_version: number;
  details: Record<string, unknown>;
  created_at: string;
}

export interface AssetCollectionEnvelope<T> {
  data: T[];
  pagination: {
    page: number;
    page_size: number;
    total_items: number;
    total_pages: number;
  };
}

export interface AssetPage<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}
