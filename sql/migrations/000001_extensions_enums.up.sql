-- Migration 001: Extensions & Enum Types
-- ComplianceForge GRC Platform
--
-- This migration establishes the foundational PostgreSQL extensions and
-- custom enum types used throughout the schema. Extensions are chosen for:
--   - uuid-ossp / pgcrypto: UUID generation and cryptographic functions
--   - btree_gist: GiST index support for exclusion constraints (used in scheduling)
--   - pg_trgm: Trigram similarity for fuzzy text search across compliance controls

-- ============================================================================
-- EXTENSIONS
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";      -- uuid_generate_v4() fallback
CREATE EXTENSION IF NOT EXISTS "pgcrypto";        -- gen_random_uuid(), crypt(), gen_salt()
CREATE EXTENSION IF NOT EXISTS "btree_gist";      -- GiST index operator classes
CREATE EXTENSION IF NOT EXISTS "pg_trgm";         -- trigram similarity / fuzzy search

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

-- Organization lifecycle status.
-- Transitions: trial → active → suspended → deactivated (or trial → deactivated).
CREATE TYPE org_status AS ENUM (
    'active',
    'suspended',
    'trial',
    'deactivated'
);

-- Subscription tier determines feature gates and limits.
CREATE TYPE org_tier AS ENUM (
    'starter',
    'professional',
    'enterprise',
    'unlimited'
);

-- User account status. Locked is set automatically after N failed logins.
CREATE TYPE user_status AS ENUM (
    'active',
    'inactive',
    'locked',
    'pending_verification'
);

-- Role identifiers. Ordered from most to least privileged.
-- super_admin is platform-level; org_admin is the highest org-level role.
-- external_auditor has read-only scoped access for third-party audit firms.
CREATE TYPE user_role AS ENUM (
    'super_admin',
    'org_admin',
    'compliance_manager',
    'risk_manager',
    'auditor',
    'policy_owner',
    'dpo',
    'ciso',
    'viewer',
    'external_auditor'
);

-- CRUD + domain-specific actions for the RBAC permission model.
CREATE TYPE permission_action AS ENUM (
    'create',
    'read',
    'update',
    'delete',
    'approve',
    'assign',
    'export',
    'configure'
);

-- Supported MFA methods. hardware_key covers FIDO2 / WebAuthn.
CREATE TYPE mfa_method AS ENUM (
    'totp',
    'sms',
    'email',
    'hardware_key'
);

-- Subscription billing status, aligned with Stripe-style lifecycle.
CREATE TYPE subscription_status AS ENUM (
    'active',
    'past_due',
    'canceled',
    'trialing'
);
