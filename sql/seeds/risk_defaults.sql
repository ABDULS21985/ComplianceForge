-- Seed Data: Default Risk Categories & Sample Risk Matrix
-- ComplianceForge GRC Platform
--
-- System-default risk categories aligned with ISO 31000, COSO ERM, and
-- common enterprise risk taxonomies. These are available to all organizations.
--
-- The sample 5×5 risk matrix follows the standard likelihood × impact model
-- used by most European enterprises and required by many regulators.

BEGIN;

-- ============================================================================
-- SYSTEM DEFAULT RISK CATEGORIES
-- ============================================================================
-- organization_id = NULL = system-wide defaults available to all tenants.
-- Fixed UUIDs for referenceability.

INSERT INTO risk_categories (id, organization_id, name, code, description, parent_category_id, color_hex, icon, sort_order, is_system_default) VALUES

('a0000000-0000-0000-0000-000000000001', NULL, 'Strategic Risk', 'STRATEGIC',
 'Risks arising from strategic decisions, market changes, competitive dynamics, M&A, and business model disruption. Includes risks to long-term objectives and competitive positioning.',
 NULL, '#EF4444', 'target', 1, true),

('a0000000-0000-0000-0000-000000000002', NULL, 'Operational Risk', 'OPERATIONAL',
 'Risks from inadequate or failed internal processes, people, and systems, or from external events. Includes business continuity, process failures, and supply chain disruption.',
 NULL, '#F97316', 'settings', 2, true),

('a0000000-0000-0000-0000-000000000003', NULL, 'Financial Risk', 'FINANCIAL',
 'Risks related to financial loss, liquidity, credit, market volatility, and treasury operations. Includes currency, interest rate, and counterparty risks.',
 NULL, '#EAB308', 'trending-down', 3, true),

('a0000000-0000-0000-0000-000000000004', NULL, 'Compliance Risk', 'COMPLIANCE',
 'Risks of legal or regulatory sanctions, financial loss, or reputational damage from failure to comply with laws, regulations, rules, standards, or codes of conduct.',
 NULL, '#8B5CF6', 'scale', 4, true),

('a0000000-0000-0000-0000-000000000005', NULL, 'Cybersecurity Risk', 'CYBERSECURITY',
 'Risks from cyber threats including data breaches, ransomware, phishing, DDoS, insider threats, and supply chain compromises. Aligned with NIST CSF and NCSC CAF.',
 NULL, '#DC2626', 'shield-alert', 5, true),

('a0000000-0000-0000-0000-000000000006', NULL, 'Reputational Risk', 'REPUTATIONAL',
 'Risks of negative public perception, brand damage, or loss of stakeholder confidence. Often a secondary consequence of other risk events materialising.',
 NULL, '#EC4899', 'megaphone', 6, true),

('a0000000-0000-0000-0000-000000000007', NULL, 'Third-Party Risk', 'THIRD_PARTY',
 'Risks introduced through suppliers, vendors, partners, and outsourced service providers. Includes concentration risk, vendor lock-in, and supply chain integrity.',
 NULL, '#14B8A6', 'users', 7, true),

('a0000000-0000-0000-0000-000000000008', NULL, 'Environmental / ESG Risk', 'ESG',
 'Environmental, social, and governance risks including climate change, sustainability, carbon exposure, social responsibility, and governance failures.',
 NULL, '#22C55E', 'leaf', 8, true),

('a0000000-0000-0000-0000-000000000009', NULL, 'Legal Risk', 'LEGAL',
 'Risks from litigation, contractual disputes, intellectual property infringement, and regulatory enforcement actions.',
 NULL, '#6366F1', 'gavel', 9, true),

('a0000000-0000-0000-0000-000000000010', NULL, 'Technology Risk', 'TECHNOLOGY',
 'Risks from technology failures, obsolescence, technical debt, system downtime, and inadequate IT infrastructure. Distinct from cybersecurity (which focuses on threat actors).',
 NULL, '#0EA5E9', 'cpu', 10, true),

('a0000000-0000-0000-0000-000000000011', NULL, 'People / HR Risk', 'PEOPLE',
 'Risks related to workforce: key person dependency, talent retention, skills gaps, workplace safety, culture, and employment law compliance.',
 NULL, '#F59E0B', 'user-check', 11, true),

('a0000000-0000-0000-0000-000000000012', NULL, 'Geopolitical Risk', 'GEOPOLITICAL',
 'Risks from political instability, sanctions, trade wars, regulatory divergence across jurisdictions, and geopolitical conflict affecting operations or supply chains.',
 NULL, '#78716C', 'globe', 12, true);

-- Sub-categories for Cybersecurity (as an example of hierarchical taxonomy)
INSERT INTO risk_categories (id, organization_id, name, code, description, parent_category_id, color_hex, icon, sort_order, is_system_default) VALUES
('a0000000-0000-0000-0000-000000000051', NULL, 'Data Breach', 'CYBER_DATA_BREACH',
 'Unauthorized access to or exfiltration of sensitive data including personal data (GDPR), financial data (PCI DSS), and intellectual property.',
 'a0000000-0000-0000-0000-000000000005', '#DC2626', 'database', 1, true),
('a0000000-0000-0000-0000-000000000052', NULL, 'Ransomware', 'CYBER_RANSOMWARE',
 'Risk of ransomware attack encrypting critical systems and data, potentially causing operational disruption and data loss.',
 'a0000000-0000-0000-0000-000000000005', '#DC2626', 'lock', 2, true),
('a0000000-0000-0000-0000-000000000053', NULL, 'Insider Threat', 'CYBER_INSIDER',
 'Risk from malicious or negligent actions by employees, contractors, or other insiders with authorized access.',
 'a0000000-0000-0000-0000-000000000005', '#DC2626', 'user-x', 3, true),
('a0000000-0000-0000-0000-000000000054', NULL, 'Supply Chain Compromise', 'CYBER_SUPPLY_CHAIN',
 'Risk of cyber attack via compromised third-party software, services, or hardware in the supply chain (e.g., SolarWinds-style attacks).',
 'a0000000-0000-0000-0000-000000000005', '#DC2626', 'link', 4, true),
('a0000000-0000-0000-0000-000000000055', NULL, 'Cloud Security', 'CYBER_CLOUD',
 'Risks specific to cloud computing: misconfiguration, shared responsibility gaps, data residency, and cloud provider incidents.',
 'a0000000-0000-0000-0000-000000000005', '#DC2626', 'cloud', 5, true);

-- ============================================================================
-- SAMPLE 5×5 RISK MATRIX
-- ============================================================================
-- This is a template. Each organization will get their own copy when they
-- onboard, but this provides the default configuration.
-- Using a well-known placeholder org UUID — the application copies this into
-- each new org's context during provisioning.

-- NOTE: This uses a placeholder org ID. In practice, the application's org
-- provisioning service copies this template into each new org's context.
-- We insert it here with a NULL-safe approach using a dedicated system org.

-- For seed purposes, we'll document the matrix structure that the application
-- should use when provisioning new organizations:

/*
TEMPLATE: Standard 5×5 Risk Matrix (European Enterprise Default)

The application should create this for each new organization:

INSERT INTO risk_matrices (organization_id, name, description, likelihood_scale, impact_scale, risk_levels, matrix_size, is_default)
VALUES (<new_org_id>, 'Standard 5×5 Risk Matrix', 'Default risk assessment matrix aligned with ISO 31000 and EU regulatory expectations.',

-- Likelihood Scale:
'[
  {"level": 1, "label": "Rare",          "description": "May only occur in exceptional circumstances",     "probability_range": "0-5%",    "frequency": "Once in 20+ years"},
  {"level": 2, "label": "Unlikely",      "description": "Could occur at some time but not expected",       "probability_range": "5-20%",   "frequency": "Once in 5-20 years"},
  {"level": 3, "label": "Possible",      "description": "Might occur at some time in the future",          "probability_range": "20-50%",  "frequency": "Once in 1-5 years"},
  {"level": 4, "label": "Likely",        "description": "Will probably occur in most circumstances",       "probability_range": "50-80%",  "frequency": "Once or more per year"},
  {"level": 5, "label": "Almost Certain","description": "Expected to occur in most circumstances",         "probability_range": "80-100%", "frequency": "Multiple times per year"}
]'::jsonb,

-- Impact Scale (multi-dimensional):
'[
  {"level": 1, "label": "Insignificant", "financial": "<€10K",         "operational": "No disruption",              "reputational": "No external awareness",          "compliance": "Minor non-compliance, no action",    "safety": "No injury"},
  {"level": 2, "label": "Minor",         "financial": "€10K-€100K",    "operational": "<4 hours disruption",        "reputational": "Local media/limited awareness",  "compliance": "Regulatory inquiry",                  "safety": "First aid treatment"},
  {"level": 3, "label": "Moderate",      "financial": "€100K-€1M",     "operational": "4-24 hours disruption",      "reputational": "National media, social media",   "compliance": "Formal regulatory investigation",     "safety": "Medical treatment required"},
  {"level": 4, "label": "Major",         "financial": "€1M-€10M",      "operational": "1-7 days disruption",        "reputational": "Sustained negative coverage",    "compliance": "Regulatory sanction/fine",             "safety": "Serious injury/hospitalisation"},
  {"level": 5, "label": "Catastrophic",  "financial": ">€10M",         "operational": ">7 days disruption",         "reputational": "International, lasting damage",  "compliance": "Criminal prosecution/licence loss",   "safety": "Fatality or permanent disability"}
]'::jsonb,

-- Risk Levels (score = likelihood × impact):
'[
  {"min_score": 1,  "max_score": 3,  "label": "Very Low",  "color": "#22C55E", "action": "Accept and monitor. Review annually."},
  {"min_score": 4,  "max_score": 6,  "label": "Low",       "color": "#84CC16", "action": "Monitor and review. Consider low-cost mitigations."},
  {"min_score": 7,  "max_score": 12, "label": "Medium",    "color": "#EAB308", "action": "Active management required. Implement treatment plan within 90 days."},
  {"min_score": 13, "max_score": 19, "label": "High",      "color": "#F97316", "action": "Senior management attention. Treatment plan within 30 days."},
  {"min_score": 20, "max_score": 25, "label": "Critical",  "color": "#EF4444", "action": "Board/executive attention. Immediate action required."}
]'::jsonb,

5, true);
*/

COMMIT;
