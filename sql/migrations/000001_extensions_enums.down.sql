-- Rollback Migration 001: Drop all enum types and extensions.
-- Order matters: enums first (they may be referenced by columns), then extensions.

DROP TYPE IF EXISTS subscription_status CASCADE;
DROP TYPE IF EXISTS mfa_method CASCADE;
DROP TYPE IF EXISTS permission_action CASCADE;
DROP TYPE IF EXISTS user_role CASCADE;
DROP TYPE IF EXISTS user_status CASCADE;
DROP TYPE IF EXISTS org_tier CASCADE;
DROP TYPE IF EXISTS org_status CASCADE;

DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "btree_gist";
DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
