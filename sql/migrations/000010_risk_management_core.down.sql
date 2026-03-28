-- Rollback Migration 010: Drop core risk management tables.

-- Drop RLS policies
DROP POLICY IF EXISTS risks_tenant_select ON risks;
DROP POLICY IF EXISTS risks_tenant_insert ON risks;
DROP POLICY IF EXISTS risks_tenant_update ON risks;
DROP POLICY IF EXISTS risks_tenant_delete ON risks;

DROP POLICY IF EXISTS matrix_tenant_select ON risk_matrices;
DROP POLICY IF EXISTS matrix_tenant_insert ON risk_matrices;
DROP POLICY IF EXISTS matrix_tenant_update ON risk_matrices;
DROP POLICY IF EXISTS matrix_tenant_delete ON risk_matrices;

DROP POLICY IF EXISTS appetite_tenant_select ON risk_appetite_statements;
DROP POLICY IF EXISTS appetite_tenant_insert ON risk_appetite_statements;
DROP POLICY IF EXISTS appetite_tenant_update ON risk_appetite_statements;
DROP POLICY IF EXISTS appetite_tenant_delete ON risk_appetite_statements;

DROP POLICY IF EXISTS risk_cat_tenant_select ON risk_categories;
DROP POLICY IF EXISTS risk_cat_tenant_insert ON risk_categories;
DROP POLICY IF EXISTS risk_cat_tenant_update ON risk_categories;
DROP POLICY IF EXISTS risk_cat_tenant_delete ON risk_categories;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_risks_generate_ref ON risks;
DROP TRIGGER IF EXISTS trg_risks_calculate_scores ON risks;
DROP TRIGGER IF EXISTS trg_risks_search_vector ON risks;
DROP TRIGGER IF EXISTS trg_risks_updated_at ON risks;
DROP TRIGGER IF EXISTS trg_risk_matrices_updated_at ON risk_matrices;
DROP TRIGGER IF EXISTS trg_risk_appetite_updated_at ON risk_appetite_statements;
DROP TRIGGER IF EXISTS trg_risk_categories_updated_at ON risk_categories;

-- Drop functions
DROP FUNCTION IF EXISTS risks_generate_ref();
DROP FUNCTION IF EXISTS risks_calculate_scores();
DROP FUNCTION IF EXISTS risks_search_vector_update();

-- Drop tables in dependency order
DROP TABLE IF EXISTS risks CASCADE;
DROP TABLE IF EXISTS risk_matrices CASCADE;
DROP TABLE IF EXISTS risk_appetite_statements CASCADE;
DROP TABLE IF EXISTS risk_categories CASCADE;
