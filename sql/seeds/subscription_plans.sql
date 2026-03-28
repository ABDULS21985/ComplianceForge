-- Seed Data: Default Subscription Plans
-- ComplianceForge GRC Platform
--
-- Three-tier pricing model aligned with EU GRC market expectations.
-- Pricing in EUR. max_risks/max_vendors = 0 means unlimited.
-- Fixed UUIDs for referenceability in tests and other seeds.

BEGIN;

INSERT INTO subscription_plans (id, name, slug, description, tier, pricing_eur_monthly, pricing_eur_annual, max_users, max_frameworks, max_risks, max_vendors, max_storage_gb, features, is_active, sort_order) VALUES

-- Starter: small teams getting started with compliance
('b0000000-0000-0000-0000-000000000001',
 'Starter',
 'starter',
 'Essential GRC tools for small teams. Includes core compliance management, basic risk register, and policy management for up to 3 frameworks.',
 'starter',
 99.00,
 990.00,
 5,         -- max_users
 3,         -- max_frameworks
 50,        -- max_risks
 10,        -- max_vendors
 5,         -- max_storage_gb
 '{
    "sso": false,
    "api_access": false,
    "custom_branding": false,
    "advanced_reporting": false,
    "continuous_monitoring": false,
    "ai_scoring": false,
    "multi_language": false,
    "priority_support": false,
    "audit_workspace": true,
    "basic_reporting": true,
    "risk_register": true,
    "policy_management": true,
    "control_tracking": true
 }',
 true,
 1),

-- Professional: mid-market teams with compliance programs
('b0000000-0000-0000-0000-000000000002',
 'Professional',
 'professional',
 'Advanced GRC platform for growing compliance programs. Unlimited risks, SSO, API access, advanced reporting, and continuous monitoring for up to 25 users.',
 'professional',
 299.00,
 2990.00,
 25,        -- max_users
 5,         -- max_frameworks
 0,         -- max_risks (unlimited)
 50,        -- max_vendors
 25,        -- max_storage_gb
 '{
    "sso": true,
    "api_access": true,
    "custom_branding": false,
    "advanced_reporting": true,
    "continuous_monitoring": true,
    "ai_scoring": true,
    "multi_language": false,
    "priority_support": true,
    "audit_workspace": true,
    "basic_reporting": true,
    "risk_register": true,
    "policy_management": true,
    "control_tracking": true,
    "vendor_management": true,
    "incident_management": true,
    "dsr_management": true
 }',
 true,
 2),

-- Enterprise: large organizations with complex compliance needs
('b0000000-0000-0000-0000-000000000003',
 'Enterprise',
 'enterprise',
 'Full-featured GRC suite for enterprise compliance. Unlimited risks and vendors, 100 users, all frameworks, custom branding, multi-language support, dedicated support, and ABAC.',
 'enterprise',
 799.00,
 7990.00,
 100,       -- max_users
 9,         -- max_frameworks
 0,         -- max_risks (unlimited)
 0,         -- max_vendors (unlimited)
 100,       -- max_storage_gb
 '{
    "sso": true,
    "api_access": true,
    "custom_branding": true,
    "advanced_reporting": true,
    "continuous_monitoring": true,
    "ai_scoring": true,
    "multi_language": true,
    "priority_support": true,
    "dedicated_csm": true,
    "audit_workspace": true,
    "basic_reporting": true,
    "risk_register": true,
    "policy_management": true,
    "control_tracking": true,
    "vendor_management": true,
    "incident_management": true,
    "dsr_management": true,
    "abac": true,
    "field_level_security": true,
    "custom_integrations": true,
    "sla_guarantee": true,
    "on_premise_option": true
 }',
 true,
 3);

COMMIT;
