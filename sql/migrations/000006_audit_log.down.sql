-- Rollback Migration 006: Drop audit log and its partitions.

-- Drop the partition creation utility function
DROP FUNCTION IF EXISTS create_audit_log_partition(DATE);

-- Drop RLS policies
DROP POLICY IF EXISTS audit_logs_tenant_isolation_select ON audit_logs;
DROP POLICY IF EXISTS audit_logs_tenant_isolation_insert ON audit_logs;

-- Drop the partitioned table (cascades to all partitions)
DROP TABLE IF EXISTS audit_logs CASCADE;
