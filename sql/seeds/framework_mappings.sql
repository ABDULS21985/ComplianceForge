-- Seed Data: Cross-Framework Control Mappings
-- ComplianceForge GRC Platform
--
-- Maps controls across frameworks to enable "implement one, cover many" analysis.
-- Mapping types:
--   equivalent  = 1:1 direct correspondence (strength 0.90-1.00)
--   partial     = covers some but not all aspects (strength 0.40-0.80)
--   related     = thematic relationship (strength 0.20-0.50)
--   superset    = source control covers more than target
--   subset      = target control covers more than source
--
-- These mappings are based on published cross-reference documents from NIST,
-- ISO, and PCI SSC. In production, an AI-assisted mapping tool would generate
-- additional mappings.

BEGIN;

-- ============================================================================
-- ISO 27001 ↔ NIST CSF 2.0 Mappings
-- ============================================================================

INSERT INTO framework_control_mappings (source_control_id, target_control_id, mapping_type, mapping_strength, notes) VALUES

-- A.5.1 Policies → GV.PO-01 Policy established
('c0000000-0001-0000-0000-000000000001', 'c0000000-0006-0000-0000-000000000010', 'equivalent', 0.95, 'Both require establishment of information/cybersecurity policies'),

-- A.5.2 Roles & Responsibilities → GV.RR-02 Roles established
('c0000000-0001-0000-0000-000000000002', 'c0000000-0006-0000-0000-000000000009', 'equivalent', 0.90, 'Both require defined cybersecurity roles and responsibilities'),

-- A.5.7 Threat intelligence → ID.RA-02 CTI received
('c0000000-0001-0000-0000-000000000007', 'c0000000-0006-0000-0000-000000000016', 'equivalent', 0.85, 'Both require threat intelligence gathering and analysis'),

-- A.5.9 Asset inventory → ID.AM-01 HW inventory + ID.AM-02 SW inventory
('c0000000-0001-0000-0000-000000000009', 'c0000000-0006-0000-0000-000000000013', 'partial', 0.70, 'ISO covers all assets; NIST CSF ID.AM-01 focuses on hardware'),
('c0000000-0001-0000-0000-000000000009', 'c0000000-0006-0000-0000-000000000014', 'partial', 0.70, 'ISO covers all assets; NIST CSF ID.AM-02 focuses on software'),

-- A.5.15 Access control → PR.AA-05 Access permissions managed
('c0000000-0001-0000-0000-000000000015', 'c0000000-0006-0000-0000-000000000021', 'equivalent', 0.90, 'Both require access control based on business need'),

-- A.5.16 Identity management → PR.AA-01 Identities managed
('c0000000-0001-0000-0000-000000000016', 'c0000000-0006-0000-0000-000000000018', 'equivalent', 0.90, 'Both cover identity lifecycle management'),

-- A.5.17 Authentication → PR.AA-03 Authentication
('c0000000-0001-0000-0000-000000000017', 'c0000000-0006-0000-0000-000000000020', 'equivalent', 0.90, 'Both require secure authentication mechanisms'),

-- A.5.24 Incident management planning → RS.MA-01 IR plan executed
('c0000000-0001-0000-0000-000000000024', 'c0000000-0006-0000-0000-000000000030', 'partial', 0.75, 'ISO focuses on planning; NIST CSF on execution'),

-- A.5.26 Incident response → RS.MA-02 Incident reports triaged
('c0000000-0001-0000-0000-000000000026', 'c0000000-0006-0000-0000-000000000031', 'partial', 0.70, 'Both cover incident response procedures'),

-- A.5.27 Lessons learned → ID.IM-01 Improvements identified
('c0000000-0001-0000-0000-000000000027', 'c0000000-0006-0000-0000-000000000017', 'equivalent', 0.85, 'Both require learning from incidents to improve'),

-- A.5.29 BC during disruption → RC.RP-01 Recovery plan executed
('c0000000-0001-0000-0000-000000000029', 'c0000000-0006-0000-0000-000000000034', 'partial', 0.65, 'ISO focuses on continuity during disruption; NIST on recovery'),

-- A.5.31 Legal requirements → GV.OC-03 Legal/regulatory requirements understood
('c0000000-0001-0000-0000-000000000031', 'c0000000-0006-0000-0000-000000000003', 'equivalent', 0.90, 'Both require identification of legal/regulatory requirements'),

-- A.6.3 Awareness & training → PR.AT-01 Personnel trained
('c0000000-0001-0000-0000-000000000040', 'c0000000-0006-0000-0000-000000000022', 'equivalent', 0.95, 'Both require security awareness training for personnel'),

-- A.8.5 Secure authentication → PR.AA-03 Authentication
('c0000000-0001-0000-0000-000000000064', 'c0000000-0006-0000-0000-000000000020', 'equivalent', 0.90, 'Both require secure authentication technologies'),

-- A.8.7 Malware protection → PR.DS-01 Data-at-rest protected (partial)
('c0000000-0001-0000-0000-000000000066', 'c0000000-0006-0000-0000-000000000023', 'related', 0.40, 'Malware protection contributes to data protection at rest'),

-- A.8.8 Vulnerability management → ID.RA-01 Vulnerabilities identified
('c0000000-0001-0000-0000-000000000067', 'c0000000-0006-0000-0000-000000000015', 'equivalent', 0.90, 'Both require identification and management of vulnerabilities'),

-- A.8.9 Configuration management → PR.PS-01 Config management
('c0000000-0001-0000-0000-000000000068', 'c0000000-0006-0000-0000-000000000025', 'equivalent', 0.90, 'Both require configuration management practices'),

-- A.8.15 Logging → DE.CM-01 Networks monitored
('c0000000-0001-0000-0000-000000000074', 'c0000000-0006-0000-0000-000000000026', 'partial', 0.65, 'Logging supports but does not fully equal network monitoring'),

-- A.8.16 Monitoring → DE.CM-01 + DE.CM-03
('c0000000-0001-0000-0000-000000000075', 'c0000000-0006-0000-0000-000000000026', 'equivalent', 0.85, 'Both require monitoring for anomalous behaviour'),
('c0000000-0001-0000-0000-000000000075', 'c0000000-0006-0000-0000-000000000028', 'partial', 0.65, 'Monitoring overlaps with personnel activity monitoring'),

-- A.8.20 Network security → DE.CM-01 Networks monitored
('c0000000-0001-0000-0000-000000000079', 'c0000000-0006-0000-0000-000000000026', 'partial', 0.60, 'Network security enables network monitoring'),

-- A.8.24 Cryptography → PR.DS-01 Data-at-rest + PR.DS-02 Data-in-transit
('c0000000-0001-0000-0000-000000000083', 'c0000000-0006-0000-0000-000000000023', 'partial', 0.75, 'Cryptography is key mechanism for protecting data at rest'),
('c0000000-0001-0000-0000-000000000083', 'c0000000-0006-0000-0000-000000000024', 'partial', 0.80, 'Cryptography is key mechanism for protecting data in transit');

-- ============================================================================
-- ISO 27001 ↔ Cyber Essentials Mappings
-- ============================================================================

INSERT INTO framework_control_mappings (source_control_id, target_control_id, mapping_type, mapping_strength, notes) VALUES

-- A.8.20 Network security → CE1.1 Firewalls configured
('c0000000-0001-0000-0000-000000000079', 'c0000000-0004-0000-0000-000000000001', 'superset', 0.60, 'ISO network security is broader; CE firewalls is specific'),

-- A.8.22 Network segregation → CE1.4 Unapproved services blocked
('c0000000-0001-0000-0000-000000000081', 'c0000000-0004-0000-0000-000000000004', 'partial', 0.55, 'Network segregation supports blocking unapproved services'),

-- A.8.9 Configuration management → CE2.1 Unnecessary software removed
('c0000000-0001-0000-0000-000000000068', 'c0000000-0004-0000-0000-000000000005', 'superset', 0.65, 'Configuration management includes removing unnecessary software'),

-- A.5.17 Authentication → CE2.2 Default passwords changed
('c0000000-0001-0000-0000-000000000017', 'c0000000-0004-0000-0000-000000000006', 'superset', 0.50, 'Authentication management encompasses default password changes'),

-- A.5.15 Access control → CE3.1 User accounts managed
('c0000000-0001-0000-0000-000000000015', 'c0000000-0004-0000-0000-000000000008', 'superset', 0.65, 'ISO access control is broader than CE user account management'),

-- A.8.2 Privileged access → CE3.2 Admin accounts restricted
('c0000000-0001-0000-0000-000000000061', 'c0000000-0004-0000-0000-000000000009', 'equivalent', 0.85, 'Both restrict privileged/admin access to admin activities'),

-- A.8.5 Secure authentication → CE3.3 MFA used
('c0000000-0001-0000-0000-000000000064', 'c0000000-0004-0000-0000-000000000010', 'superset', 0.70, 'ISO secure auth is broader; CE specifically requires MFA'),

-- A.8.7 Malware protection → CE4.1 Anti-malware installed
('c0000000-0001-0000-0000-000000000066', 'c0000000-0004-0000-0000-000000000011', 'equivalent', 0.85, 'Both require anti-malware protection'),

-- A.8.8 Vulnerability management → CE5.2 Critical patches within 14 days
('c0000000-0001-0000-0000-000000000067', 'c0000000-0004-0000-0000-000000000015', 'superset', 0.70, 'ISO vuln management is broader; CE has specific 14-day patch SLA');

-- ============================================================================
-- NIST CSF 2.0 ↔ Cyber Essentials Mappings
-- ============================================================================

INSERT INTO framework_control_mappings (source_control_id, target_control_id, mapping_type, mapping_strength, notes) VALUES

-- PR.AA-01 Identities managed → CE3.1 User accounts managed
('c0000000-0006-0000-0000-000000000018', 'c0000000-0004-0000-0000-000000000008', 'superset', 0.65, 'NIST identity management is broader than CE user account management'),

-- PR.AA-03 Authentication → CE3.3 MFA
('c0000000-0006-0000-0000-000000000020', 'c0000000-0004-0000-0000-000000000010', 'superset', 0.70, 'NIST authentication covers more than CE MFA requirement'),

-- PR.AA-05 Access permissions → CE3.2 Admin accounts restricted
('c0000000-0006-0000-0000-000000000021', 'c0000000-0004-0000-0000-000000000009', 'superset', 0.65, 'NIST access permissions broader than CE admin account restriction'),

-- PR.PS-01 Configuration management → CE2.1 Unnecessary software removed
('c0000000-0006-0000-0000-000000000025', 'c0000000-0004-0000-0000-000000000005', 'superset', 0.60, 'NIST config management includes secure baseline configuration'),

-- DE.CM-01 Network monitoring → CE1.1 Firewalls configured
('c0000000-0006-0000-0000-000000000026', 'c0000000-0004-0000-0000-000000000001', 'related', 0.40, 'Network monitoring complements firewall configuration'),

-- ID.RA-01 Vulnerabilities identified → CE5.2 Critical patches
('c0000000-0006-0000-0000-000000000015', 'c0000000-0004-0000-0000-000000000015', 'partial', 0.65, 'Vulnerability identification leads to patching');

COMMIT;
