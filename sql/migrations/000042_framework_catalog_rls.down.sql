DROP POLICY IF EXISTS framework_controls_writable_parent_delete ON framework_controls;
DROP POLICY IF EXISTS framework_controls_writable_parent_update ON framework_controls;
DROP POLICY IF EXISTS framework_controls_writable_parent_insert ON framework_controls;
DROP POLICY IF EXISTS framework_controls_visible_parent_select ON framework_controls;
ALTER TABLE framework_controls NO FORCE ROW LEVEL SECURITY;
ALTER TABLE framework_controls DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS framework_domains_writable_parent_delete ON framework_domains;
DROP POLICY IF EXISTS framework_domains_writable_parent_update ON framework_domains;
DROP POLICY IF EXISTS framework_domains_writable_parent_insert ON framework_domains;
DROP POLICY IF EXISTS framework_domains_visible_parent_select ON framework_domains;
ALTER TABLE framework_domains NO FORCE ROW LEVEL SECURITY;
ALTER TABLE framework_domains DISABLE ROW LEVEL SECURITY;
