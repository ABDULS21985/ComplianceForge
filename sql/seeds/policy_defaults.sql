-- Seed Data: Default Policy Categories
-- ComplianceForge GRC Platform
--
-- System-default policy categories aligned with common compliance frameworks.
-- Available to all organizations (organization_id = NULL).

BEGIN;

INSERT INTO policy_categories (id, organization_id, name, code, description, parent_category_id, sort_order, is_system_default) VALUES

('b0000000-0000-0000-0000-000000000001', NULL, 'Information Security', 'INFO_SECURITY',
 'Policies governing the protection of information assets, covering confidentiality, integrity, and availability. Core to ISO 27001 ISMS requirements.',
 NULL, 1, true),

('b0000000-0000-0000-0000-000000000002', NULL, 'Data Protection & Privacy', 'DATA_PRIVACY',
 'Policies for handling personal data in compliance with GDPR, UK GDPR, and other privacy regulations. Includes data processing, retention, and subject rights.',
 NULL, 2, true),

('b0000000-0000-0000-0000-000000000003', NULL, 'Acceptable Use', 'ACCEPTABLE_USE',
 'Policies defining acceptable use of organizational IT resources, systems, email, internet, mobile devices, and social media.',
 NULL, 3, true),

('b0000000-0000-0000-0000-000000000004', NULL, 'Access Control', 'ACCESS_CONTROL',
 'Policies governing logical and physical access to systems, networks, and data. Covers authentication, authorization, privileged access, and remote access.',
 NULL, 4, true),

('b0000000-0000-0000-0000-000000000005', NULL, 'Business Continuity', 'BUSINESS_CONTINUITY',
 'Policies for business continuity planning, disaster recovery, crisis management, and operational resilience. Aligned with ISO 22301.',
 NULL, 5, true),

('b0000000-0000-0000-0000-000000000006', NULL, 'Incident Management', 'INCIDENT_MGMT',
 'Policies for information security incident detection, response, escalation, and post-incident review. Covers GDPR breach notification requirements.',
 NULL, 6, true),

('b0000000-0000-0000-0000-000000000007', NULL, 'Risk Management', 'RISK_MGMT',
 'Policies defining the organization''s approach to risk identification, assessment, treatment, and monitoring. Aligned with ISO 31000.',
 NULL, 7, true),

('b0000000-0000-0000-0000-000000000008', NULL, 'Third-Party & Vendor Management', 'THIRD_PARTY',
 'Policies for managing risks from suppliers, vendors, and outsourced service providers. Covers due diligence, contractual requirements, and ongoing monitoring.',
 NULL, 8, true),

('b0000000-0000-0000-0000-000000000009', NULL, 'HR & Employment', 'HR_EMPLOYMENT',
 'Policies covering pre-employment screening, security responsibilities in employment, and termination/change of employment processes.',
 NULL, 9, true),

('b0000000-0000-0000-0000-000000000010', NULL, 'Physical Security', 'PHYSICAL_SECURITY',
 'Policies governing physical access controls, secure areas, equipment security, and environmental protections.',
 NULL, 10, true),

('b0000000-0000-0000-0000-000000000011', NULL, 'Change Management', 'CHANGE_MGMT',
 'Policies for managing changes to IT systems, applications, infrastructure, and processes. Covers change approval, testing, and rollback procedures.',
 NULL, 11, true),

('b0000000-0000-0000-0000-000000000012', NULL, 'Compliance', 'COMPLIANCE',
 'Overarching compliance policies including regulatory compliance obligations, compliance monitoring, and reporting requirements.',
 NULL, 12, true),

('b0000000-0000-0000-0000-000000000013', NULL, 'Code of Conduct', 'CODE_OF_CONDUCT',
 'Organizational code of conduct, ethics policies, and behavioral expectations for all personnel.',
 NULL, 13, true),

('b0000000-0000-0000-0000-000000000014', NULL, 'Anti-Bribery & Corruption', 'ANTI_BRIBERY',
 'Policies addressing the UK Bribery Act 2010, FCPA, and other anti-corruption regulations. Covers gifts, hospitality, and facilitation payments.',
 NULL, 14, true),

('b0000000-0000-0000-0000-000000000015', NULL, 'Whistleblowing', 'WHISTLEBLOWING',
 'Policies providing mechanisms for reporting concerns about wrongdoing, fraud, or malpractice. Compliant with EU Whistleblower Protection Directive.',
 NULL, 15, true),

('b0000000-0000-0000-0000-000000000016', NULL, 'Environmental & Sustainability', 'ENVIRONMENTAL',
 'Policies addressing environmental responsibilities, sustainability goals, and ESG commitments.',
 NULL, 16, true);

-- Sub-categories for Data Protection & Privacy
INSERT INTO policy_categories (id, organization_id, name, code, description, parent_category_id, sort_order, is_system_default) VALUES
('b0000000-0000-0000-0000-000000000021', NULL, 'Data Retention & Disposal', 'DATA_RETENTION',
 'Policies defining data retention schedules, archival procedures, and secure data disposal/destruction methods.',
 'b0000000-0000-0000-0000-000000000002', 1, true),
('b0000000-0000-0000-0000-000000000022', NULL, 'Data Subject Rights', 'DATA_SUBJECT_RIGHTS',
 'Procedures for handling data subject access requests (DSARs), right to erasure, data portability, and other GDPR individual rights.',
 'b0000000-0000-0000-0000-000000000002', 2, true),
('b0000000-0000-0000-0000-000000000023', NULL, 'Data Classification', 'DATA_CLASSIFICATION',
 'Policies for classifying information by sensitivity level and defining handling requirements for each classification.',
 'b0000000-0000-0000-0000-000000000002', 3, true),
('b0000000-0000-0000-0000-000000000024', NULL, 'Data Transfer & Sharing', 'DATA_TRANSFER',
 'Policies governing the transfer of personal and sensitive data, including international transfers, adequacy decisions, and standard contractual clauses.',
 'b0000000-0000-0000-0000-000000000002', 4, true);

-- Sub-categories for Information Security
INSERT INTO policy_categories (id, organization_id, name, code, description, parent_category_id, sort_order, is_system_default) VALUES
('b0000000-0000-0000-0000-000000000031', NULL, 'Cryptography & Key Management', 'CRYPTOGRAPHY',
 'Policies for the use of cryptographic controls, encryption standards, and key lifecycle management.',
 'b0000000-0000-0000-0000-000000000001', 1, true),
('b0000000-0000-0000-0000-000000000032', NULL, 'Network Security', 'NETWORK_SECURITY',
 'Policies governing network architecture security, segmentation, firewall management, and wireless security.',
 'b0000000-0000-0000-0000-000000000001', 2, true),
('b0000000-0000-0000-0000-000000000033', NULL, 'Secure Development', 'SECURE_DEV',
 'Policies for secure software development lifecycle (SSDLC), code review, vulnerability management, and DevSecOps practices.',
 'b0000000-0000-0000-0000-000000000001', 3, true),
('b0000000-0000-0000-0000-000000000034', NULL, 'Logging & Monitoring', 'LOGGING_MONITORING',
 'Policies for security event logging, log retention, SIEM management, and continuous security monitoring.',
 'b0000000-0000-0000-0000-000000000001', 4, true),
('b0000000-0000-0000-0000-000000000035', NULL, 'Vulnerability Management', 'VULN_MGMT',
 'Policies for vulnerability scanning, patch management, penetration testing, and remediation timelines.',
 'b0000000-0000-0000-0000-000000000001', 5, true);

COMMIT;
