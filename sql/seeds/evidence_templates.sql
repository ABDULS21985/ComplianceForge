-- Seed Data: Evidence Templates
-- ComplianceForge GRC Platform
--
-- 55 system evidence templates covering the most commonly audited controls
-- across ISO 27001:2022 (primary), PCI DSS v4.0, and NIST 800-53 Rev 5.
--
-- Template UUIDs use pattern: et000000-XXXX-0000-0000-00000000YYYY
--   XXXX = framework (0001=ISO 27001, 0002=PCI DSS, 0003=NIST 800-53)
--   YYYY = sequential template number
--
-- All system templates: organization_id = NULL, is_system = true.
--
-- Depends on: 000031_evidence_templates.up.sql (table DDL)

BEGIN;

-- ============================================================================
-- ISO/IEC 27001:2022 — 35 Templates
-- ============================================================================

INSERT INTO evidence_templates (
    id, organization_id, framework_control_code, framework_code,
    name, description, evidence_category, collection_method,
    collection_instructions, collection_frequency,
    typical_collection_time_minutes, validation_rules,
    acceptance_criteria, auditor_priority, difficulty, is_system,
    common_rejection_reasons, tags
) VALUES

-- ---------------------------------------------------------------------------
-- A.5.1  Policies for information security
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000001', NULL, 'A.5.1', 'ISO27001',
 'Information Security Policy Document',
 'The master information security policy approved by senior management, establishing the organization''s approach to managing information security.',
 'policy_document', 'manual_upload',
 'Export the current board-approved information security policy as a PDF. Ensure the document includes the approval date, version number, management signature, and next review date. If the policy is maintained in a GRC or document management system, export the version-controlled copy with metadata intact.',
 'annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]},{"type":"contains_text","keywords":["approved","review date","version"]}]',
 'Document must be dated within the last 12 months, bear a management approval signature or attestation, include a scheduled review date, and cover scope, objectives, and roles.',
 'must_have', 'easy', true,
 ARRAY['Document is outdated (>12 months)', 'Missing management approval signature', 'No version number or review date', 'Draft watermark still present'],
 ARRAY['policy', 'governance', 'management-approval']),

-- ---------------------------------------------------------------------------
-- A.5.2  Information security roles and responsibilities
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000002', NULL, 'A.5.2', 'ISO27001',
 'RACI Matrix for Security Roles',
 'A responsibility assignment matrix mapping information security functions to named roles and individuals.',
 'policy_document', 'manual_upload',
 'Export the current RACI chart from the project/GRC tool or retrieve the approved spreadsheet from the document repository. Verify that every Annex A domain has at least one Responsible and one Accountable party assigned. Include the date of last review.',
 'annual', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","xlsx","docx"]}]',
 'Matrix must map all key security functions to named roles, include an accountable owner for each domain, and be dated within 12 months.',
 'should_have', 'easy', true,
 ARRAY['Roles not mapped to named individuals', 'Missing domains or functions', 'No review date'],
 ARRAY['governance', 'roles', 'responsibility']),

-- ---------------------------------------------------------------------------
-- A.5.7  Threat intelligence
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000003', NULL, 'A.5.7', 'ISO27001',
 'Threat Intelligence Subscription Report',
 'Monthly summary from the organization''s threat intelligence feed documenting relevant threats, indicators of compromise, and actions taken.',
 'audit_report', 'system_export',
 'Log in to the threat intelligence platform (e.g., Recorded Future, Mandiant, CISA alerts). Generate the monthly summary report covering the previous calendar month. Export as PDF. Ensure the report includes threat categories, relevance scoring, and any recommended actions.',
 'monthly', 20,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","html"]}]',
 'Report must cover the full prior month, list identified threats with relevance to the organization, and show evidence of review or triage actions.',
 'should_have', 'moderate', true,
 ARRAY['Report older than 45 days', 'No relevance analysis or triage notes', 'Generic feed output without org context'],
 ARRAY['threat-intelligence', 'monitoring', 'detection']),

-- ---------------------------------------------------------------------------
-- A.5.15  Access control — Policy
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000004', NULL, 'A.5.15', 'ISO27001',
 'Access Control Policy',
 'The documented policy governing logical and physical access to information assets, including role-based access principles and authorization procedures.',
 'policy_document', 'manual_upload',
 'Export the current access control policy from the document management system. Confirm it includes sections on role-based access control, least privilege, segregation of duties, access provisioning/deprovisioning, and remote access. Ensure management approval is visible.',
 'annual', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]},{"type":"contains_text","keywords":["least privilege","role-based","authorization"]}]',
 'Policy must define access control principles (RBAC, least privilege), cover provisioning and deprovisioning workflows, address remote access, and be approved within the last 12 months.',
 'must_have', 'easy', true,
 ARRAY['Missing least-privilege or RBAC language', 'No deprovisioning procedure', 'Outdated approval date'],
 ARRAY['access-control', 'policy', 'authorization']),

-- ---------------------------------------------------------------------------
-- A.5.15  Access control — Review
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000005', NULL, 'A.5.15', 'ISO27001',
 'User Access Review Report',
 'Quarterly review of user access rights across critical systems, confirming appropriateness and removal of unnecessary privileges.',
 'access_review', 'system_export',
 'Run the user access review report from the IAM platform or directory service. The report should list every user, their assigned roles, last login date, and the reviewer''s approval or revocation decision. Include the review completion date and reviewer name for each system in scope.',
 'quarterly', 60,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","xlsx","csv"]},{"type":"row_count_min","min":1}]',
 'Report must cover all in-scope systems, show reviewer sign-off for each user, flag and remediate any inappropriate access, and be completed within the quarter.',
 'must_have', 'moderate', true,
 ARRAY['Not all systems covered', 'Missing reviewer sign-off', 'Remediation actions not documented', 'Stale data (>100 days old)'],
 ARRAY['access-review', 'iam', 'user-access', 'quarterly-review']),

-- ---------------------------------------------------------------------------
-- A.5.24  Incident response — Plan
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000006', NULL, 'A.5.24', 'ISO27001',
 'Incident Response Plan',
 'The documented incident response plan covering detection, analysis, containment, eradication, recovery, and post-incident review procedures.',
 'procedure_document', 'manual_upload',
 'Export the current incident response plan from the document repository. Verify it includes defined severity levels, escalation paths with named contacts, communication templates, and post-incident review procedures. Ensure the document shows the most recent approval date and version.',
 'annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]},{"type":"contains_text","keywords":["containment","escalation","severity"]}]',
 'Plan must define incident severity classification, escalation procedures with contact information, containment and eradication steps, and include a post-incident review process. Must be reviewed within 12 months.',
 'must_have', 'moderate', true,
 ARRAY['Missing severity classification', 'No escalation contacts', 'Outdated plan (>12 months)', 'No post-incident review process'],
 ARRAY['incident-response', 'ir-plan', 'bcdr']),

-- ---------------------------------------------------------------------------
-- A.5.24  Incident response — Test results
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000007', NULL, 'A.5.24', 'ISO27001',
 'Incident Response Drill/Tabletop Results',
 'Results from the most recent incident response tabletop exercise or drill, including scenario description, participant roles, findings, and improvement actions.',
 'audit_report', 'manual_upload',
 'Collect the after-action report from the last IR tabletop or live drill. The report should include the scenario tested, participants and their roles, timeline of actions, identified gaps, and a remediation plan with owners and due dates. If lessons learned were published separately, include those as well.',
 'annual', 45,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx","pptx"]}]',
 'Report must describe the drill scenario, list all participants, document findings and gaps, and include remediation actions with assigned owners. Must be conducted within the last 12 months.',
 'should_have', 'moderate', true,
 ARRAY['No participant list', 'Missing findings or lessons learned', 'No remediation action plan', 'Drill older than 12 months'],
 ARRAY['incident-response', 'tabletop', 'drill', 'testing']),

-- ---------------------------------------------------------------------------
-- A.6.3  Security awareness training
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000008', NULL, 'A.6.3', 'ISO27001',
 'Security Awareness Training Completion Records',
 'Records demonstrating that all personnel have completed the required annual security awareness training program.',
 'training_record', 'system_export',
 'Export the training completion report from the LMS or security awareness platform (e.g., KnowBe4, Proofpoint). The report must show each employee name, completion date, pass/fail status, and the training module completed. Calculate and include the overall completion rate. Filter for the current compliance period.',
 'annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","xlsx","csv"]},{"type":"row_count_min","min":1}]',
 'Report must demonstrate at least 95% completion across all active employees, show individual pass/fail status, and be dated within the current compliance period.',
 'must_have', 'easy', true,
 ARRAY['Completion rate below 95%', 'Missing employees from the report', 'Training content not relevant to security', 'Report does not cover current period'],
 ARRAY['training', 'awareness', 'people-controls']),

-- ---------------------------------------------------------------------------
-- A.8.2  Privileged access rights
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000009', NULL, 'A.8.2', 'ISO27001',
 'Privileged Account Inventory',
 'Complete inventory of all privileged accounts across infrastructure, including account owner, justification, last review date, and MFA status.',
 'access_review', 'system_export',
 'Export the privileged account list from the PAM solution (e.g., CyberArk, BeyondTrust) or Active Directory. For each account include: account name, type (admin/service/break-glass), owner, business justification, creation date, last password rotation, and MFA enrollment status. Cross-reference with HR records to verify all owners are active employees.',
 'quarterly', 45,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","xlsx","csv"]},{"type":"row_count_min","min":1}]',
 'Inventory must list all privileged accounts with named owners, business justifications, and MFA status. Orphaned or unjustified accounts must be flagged for remediation.',
 'must_have', 'moderate', true,
 ARRAY['Accounts missing business justification', 'Orphaned accounts not flagged', 'MFA status not documented', 'Incomplete system coverage'],
 ARRAY['privileged-access', 'pam', 'iam', 'admin-accounts']),

-- ---------------------------------------------------------------------------
-- A.8.5  Secure authentication
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000010', NULL, 'A.8.5', 'ISO27001',
 'MFA Enforcement Configuration',
 'Screenshot or configuration export demonstrating multi-factor authentication is enforced for all remote access and privileged operations.',
 'configuration_screenshot', 'screenshot',
 'Navigate to the identity provider admin console (e.g., Azure AD Conditional Access, Okta Policies, Duo Admin Panel). Capture screenshots showing the MFA enforcement policy is enabled for all users, with conditional access rules requiring MFA for remote access and admin operations. Include the policy name, scope (all users), and enforcement status.',
 'quarterly', 20,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["png","jpg","pdf"]}]',
 'Evidence must show MFA is mandatory (not optional) for all users on remote access and all privileged operations. Conditional access policy must be in enforced (not report-only) mode.',
 'should_have', 'easy', true,
 ARRAY['MFA in report-only mode, not enforced', 'Policy scope excludes some users', 'Screenshot too blurry to read', 'Timestamp not visible'],
 ARRAY['mfa', 'authentication', 'identity', 'conditional-access']),

-- ---------------------------------------------------------------------------
-- A.8.7  Protection against malware
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000011', NULL, 'A.8.7', 'ISO27001',
 'Anti-Malware Deployment Report',
 'Report from the endpoint protection platform showing deployment coverage, signature update status, and scan results across all managed endpoints.',
 'vulnerability_scan', 'system_export',
 'Log in to the EDR/AV management console (e.g., CrowdStrike, Defender for Endpoint, Sophos Central). Generate the deployment and health status report showing all managed endpoints, agent version, last check-in time, signature update date, and any endpoints with outdated definitions or disabled protection. Export as PDF or CSV.',
 'monthly', 20,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Report must show 98%+ endpoint coverage with active protection, signature definitions updated within 48 hours, and no endpoints with disabled real-time protection.',
 'should_have', 'easy', true,
 ARRAY['Coverage below 98%', 'Stale signature definitions (>48h)', 'Endpoints with disabled protection not remediated', 'Report older than 45 days'],
 ARRAY['anti-malware', 'endpoint', 'edr', 'av']),

-- ---------------------------------------------------------------------------
-- A.8.8  Management of technical vulnerabilities
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000012', NULL, 'A.8.8', 'ISO27001',
 'Vulnerability Scan Report',
 'Results from automated vulnerability scanning of infrastructure and applications, showing identified vulnerabilities, severity ratings, and remediation status.',
 'vulnerability_scan', 'system_export',
 'Run a credentialed vulnerability scan using the approved scanner (e.g., Nessus, Qualys, Rapid7 InsightVM) covering all in-scope IP ranges and web applications. Export the executive summary and detailed findings report. Ensure the scan includes CVSS scores, remediation recommendations, and trend comparison against the prior scan period.',
 'monthly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","csv","html"]},{"type":"contains_text","keywords":["CVSS","critical","high"]}]',
 'Scan must cover all in-scope assets with credentialed checks, report CVSS-scored findings, and show remediation progress for critical/high vulnerabilities within SLA timelines.',
 'must_have', 'moderate', true,
 ARRAY['Uncredentialed scan only', 'Missing asset coverage', 'No remediation tracking', 'Critical vulnerabilities open beyond SLA'],
 ARRAY['vulnerability-management', 'scanning', 'patching', 'risk']),

-- ---------------------------------------------------------------------------
-- A.8.9  Configuration management — Baseline
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000013', NULL, 'A.8.9', 'ISO27001',
 'Configuration Baseline Documentation',
 'Documented secure configuration baselines (hardening standards) for servers, workstations, network devices, and cloud services.',
 'policy_document', 'manual_upload',
 'Export the approved configuration baseline documents for each technology tier (e.g., Windows Server, Linux, network switches, cloud accounts). Baselines should reference industry standards such as CIS Benchmarks. Include version, approval date, and the list of settings enforced.',
 'semi_annual', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","docx","xlsx"]}]',
 'Baselines must exist for all major technology categories, reference recognized hardening standards (CIS, DISA STIG), include version and approval date, and be reviewed at least semi-annually.',
 'should_have', 'moderate', true,
 ARRAY['Missing baselines for some platforms', 'No reference to CIS/DISA standards', 'Outdated (>6 months without review)', 'Settings list too vague'],
 ARRAY['configuration', 'hardening', 'baseline', 'cis-benchmark']),

-- ---------------------------------------------------------------------------
-- A.8.9  Configuration management — Compliance scan
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000014', NULL, 'A.8.9', 'ISO27001',
 'Configuration Compliance Scan Results',
 'Automated scan results comparing live system configurations against approved baselines, showing compliance percentage and deviations.',
 'vulnerability_scan', 'system_export',
 'Run a CIS benchmark or SCAP compliance scan against the target systems using tools such as Nessus Compliance, Qualys Policy Compliance, or AWS Config. Export the results showing each check, pass/fail status, and overall compliance score. Include the baseline profile used and scan date.',
 'quarterly', 35,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","html"]}]',
 'Scan results must show at least 85% compliance against the approved baseline, with deviations documented and risk-accepted or scheduled for remediation.',
 'should_have', 'complex', true,
 ARRAY['Compliance below 85%', 'Deviations not risk-accepted', 'Wrong baseline profile used', 'Scan does not cover all in-scope hosts'],
 ARRAY['configuration', 'compliance-scan', 'hardening', 'scap']),

-- ---------------------------------------------------------------------------
-- A.8.13  Information backup — Configuration
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000015', NULL, 'A.8.13', 'ISO27001',
 'Backup Configuration and Schedule',
 'Documentation or export of backup job configurations showing retention periods, schedules, encryption settings, and scope of systems backed up.',
 'configuration_screenshot', 'system_export',
 'Export backup job configurations from the backup platform (e.g., Veeam, AWS Backup, Commvault). For each job, capture the schedule (daily/weekly), retention period, encryption-at-rest status, target storage location, and the list of protected systems. If using a cloud-native service, export the backup policy or plan configuration.',
 'quarterly', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","png","csv","json"]}]',
 'Configuration must show all critical systems are included in backup scope, retention meets policy requirements (e.g., 30-day minimum), backups are encrypted, and schedules align with RPO targets.',
 'should_have', 'moderate', true,
 ARRAY['Critical systems missing from backup scope', 'Retention below policy minimum', 'Encryption not enabled', 'RPO not met by schedule'],
 ARRAY['backup', 'bcdr', 'data-protection', 'recovery']),

-- ---------------------------------------------------------------------------
-- A.8.13  Information backup — Restoration test
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000016', NULL, 'A.8.13', 'ISO27001',
 'Backup Restoration Test Results',
 'Results from periodic backup restoration tests demonstrating that data can be successfully recovered within defined RTO/RPO targets.',
 'audit_report', 'manual_upload',
 'Document the most recent backup restoration test. Record the system restored, backup date used, restoration start and end times (to validate RTO), data integrity verification method, and pass/fail result. If an automated restore test tool is used, export its report. Include the tester name and date.',
 'quarterly', 60,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","docx","xlsx"]}]',
 'Test must demonstrate successful data recovery, validate data integrity post-restore, confirm RTO was met, and cover at least one critical system per test cycle.',
 'should_have', 'moderate', true,
 ARRAY['RTO not measured or exceeded', 'No data integrity verification', 'Only non-critical systems tested', 'No tester sign-off'],
 ARRAY['backup', 'restoration', 'bcdr', 'testing']),

-- ---------------------------------------------------------------------------
-- A.8.15  Logging — Aggregation configuration
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000017', NULL, 'A.8.15', 'ISO27001',
 'Log Aggregation Configuration',
 'Configuration evidence showing that logs from critical systems are forwarded to a centralized log management or SIEM platform.',
 'configuration_screenshot', 'screenshot',
 'Capture screenshots or export configuration from the SIEM/log management platform (e.g., Splunk, Elastic, Sentinel) showing the list of log sources, ingestion status, and data retention settings. Also capture the syslog/agent forwarding configuration from at least two representative source systems to confirm active forwarding.',
 'quarterly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["png","pdf","json"]}]',
 'Evidence must show logs from all critical systems (firewalls, servers, identity providers, databases) are actively forwarded to the central platform with a retention period meeting policy requirements.',
 'should_have', 'moderate', true,
 ARRAY['Critical log sources missing', 'Retention below policy minimum', 'Log forwarding errors not addressed', 'No proof of active ingestion'],
 ARRAY['logging', 'siem', 'monitoring', 'log-aggregation']),

-- ---------------------------------------------------------------------------
-- A.8.15  Logging — Review records
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000018', NULL, 'A.8.15', 'ISO27001',
 'Log Review Records',
 'Evidence that security-relevant logs are reviewed on a regular basis, with anomalies investigated and documented.',
 'log_export', 'manual_upload',
 'Export log review tickets or records from the ticketing system or SIEM showing regular scheduled reviews. Each record should include the reviewer name, date, systems reviewed, findings (or confirmation of no anomalies), and any follow-up actions created. Alternatively, export SIEM saved-search history showing regular analyst activity.',
 'monthly', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","xlsx","csv"]}]',
 'Records must demonstrate regular review cadence (at minimum weekly), name the reviewer, cover all critical systems, and show investigation follow-up for any anomalies identified.',
 'should_have', 'moderate', true,
 ARRAY['Irregular review cadence', 'No named reviewer', 'Anomalies noted but not investigated', 'Only partial system coverage'],
 ARRAY['logging', 'log-review', 'monitoring', 'analyst']),

-- ---------------------------------------------------------------------------
-- A.8.16  Monitoring activities — SIEM dashboard
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000019', NULL, 'A.8.16', 'ISO27001',
 'SIEM Dashboard Screenshot',
 'Screenshot of the active SIEM dashboard showing real-time or near-real-time security monitoring status, active alerts, and key metrics.',
 'configuration_screenshot', 'screenshot',
 'Open the primary SIEM security operations dashboard. Capture a full-page screenshot showing the current alert queue, event volume trends, top triggered detection rules, and mean time to acknowledge/resolve metrics. Ensure the timestamp is visible in the screenshot. If multiple dashboards exist, capture the executive summary view.',
 'quarterly', 15,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["png","jpg","pdf"]}]',
 'Screenshot must clearly show the SIEM is operational with active data ingestion, display detection rules or use cases, and include a visible timestamp.',
 'nice_to_have', 'easy', true,
 ARRAY['Timestamp not visible', 'Dashboard shows no data or errors', 'Screenshot too low resolution', 'No detection rules visible'],
 ARRAY['siem', 'monitoring', 'soc', 'dashboard']),

-- ---------------------------------------------------------------------------
-- A.8.16  Monitoring activities — Alert configuration
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000020', NULL, 'A.8.16', 'ISO27001',
 'Monitoring Alert Rules Configuration',
 'Export of configured detection and alerting rules in the SIEM/monitoring platform, showing rule names, conditions, severity, and notification targets.',
 'configuration_screenshot', 'system_export',
 'Export the list of active alert/detection rules from the SIEM platform. For each rule include: rule name, description, trigger condition (query or logic), severity level, assigned notification channel (email, Slack, PagerDuty), and enabled/disabled status. Group rules by MITRE ATT&CK tactic where possible.',
 'quarterly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","json","xlsx"]}]',
 'Export must show a minimum set of detection rules covering authentication failures, privilege escalation, malware, and data exfiltration. Rules must have assigned notification channels and be in enabled state.',
 'should_have', 'moderate', true,
 ARRAY['Too few detection rules', 'Rules disabled without justification', 'No notification channels configured', 'Missing key threat categories'],
 ARRAY['siem', 'detection', 'alerting', 'monitoring-rules']),

-- ---------------------------------------------------------------------------
-- A.8.20  Network security — Firewall rules
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000021', NULL, 'A.8.20', 'ISO27001',
 'Firewall Rule Set Export',
 'Complete export of firewall rules from perimeter and internal segmentation firewalls, including rule descriptions, source/destination, ports, and actions.',
 'configuration_screenshot', 'system_export',
 'Export the full firewall rule set from each in-scope firewall (perimeter, internal segmentation, cloud security groups). Include rule number, name/description, source/destination zones or IPs, ports/protocols, action (allow/deny), hit count, and last-modified date. Flag any overly permissive rules (e.g., any-any-allow).',
 'semi_annual', 45,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","csv","xlsx","json"]}]',
 'Export must cover all in-scope firewalls, include rule descriptions and justifications, flag any-any rules, show deny-all default, and include the last review date.',
 'should_have', 'complex', true,
 ARRAY['Rules missing descriptions', 'Overly permissive any-any rules not justified', 'Not all firewalls included', 'No default deny rule'],
 ARRAY['firewall', 'network', 'segmentation', 'rule-review']),

-- ---------------------------------------------------------------------------
-- A.8.20  Network security — Network diagram
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000022', NULL, 'A.8.20', 'ISO27001',
 'Network Architecture Diagram',
 'Current network topology diagram showing security zones, segmentation boundaries, key infrastructure components, and data flow paths.',
 'policy_document', 'manual_upload',
 'Export the current network architecture diagram from the network documentation tool (e.g., Visio, Lucidchart, draw.io). The diagram should clearly show DMZ, internal zones, cloud VPCs, trust boundaries, firewall placement, and critical data flow paths. Include a legend and version/date stamp.',
 'semi_annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","png","vsdx"]}]',
 'Diagram must show current network topology with labeled security zones, firewall placement, segmentation boundaries, and a version date within the last 6 months.',
 'should_have', 'moderate', true,
 ARRAY['Diagram outdated (>6 months)', 'Missing security zones or trust boundaries', 'No legend or labeling', 'Cloud environment not depicted'],
 ARRAY['network', 'architecture', 'diagram', 'segmentation']),

-- ---------------------------------------------------------------------------
-- A.8.24  Use of cryptography — Encryption config
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000023', NULL, 'A.8.24', 'ISO27001',
 'Encryption Configuration Report',
 'Evidence that data at rest and in transit is encrypted using approved algorithms and key lengths across critical systems.',
 'configuration_screenshot', 'system_export',
 'For data at rest: export disk/volume encryption status (e.g., BitLocker, LUKS, AWS EBS encryption). For data in transit: capture TLS configuration from web servers and load balancers showing minimum TLS version and cipher suites. For databases: export transparent data encryption (TDE) status. Compile findings into a single report.',
 'quarterly', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx","json"]}]',
 'Report must demonstrate encryption at rest and in transit for all critical systems, using approved algorithms (AES-256, TLS 1.2+), with no weak cipher suites or deprecated protocols.',
 'should_have', 'complex', true,
 ARRAY['Weak cipher suites (e.g., RC4, DES)', 'TLS 1.0/1.1 still enabled', 'Unencrypted volumes found', 'Key lengths below minimum (e.g., RSA <2048)'],
 ARRAY['encryption', 'cryptography', 'tls', 'data-protection']),

-- ---------------------------------------------------------------------------
-- A.8.24  Use of cryptography — Certificate inventory
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000024', NULL, 'A.8.24', 'ISO27001',
 'TLS/SSL Certificate Inventory',
 'Inventory of all TLS/SSL certificates in use, including issuer, expiration date, key length, associated domain, and renewal status.',
 'audit_report', 'system_export',
 'Export the certificate inventory from the certificate management tool (e.g., Venafi, DigiCert CertCentral, AWS Certificate Manager). Include certificate subject, issuer, key algorithm and length, expiration date, associated hostname/domain, and auto-renewal status. Flag any certificates expiring within 30 days or using deprecated algorithms.',
 'quarterly', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Inventory must list all production certificates, flag those expiring within 30 days, show no certificates using SHA-1 or key lengths below 2048 bits, and indicate renewal management status.',
 'nice_to_have', 'moderate', true,
 ARRAY['Certificates expiring soon without renewal plan', 'SHA-1 certificates in use', 'Incomplete inventory', 'Self-signed certificates on public services'],
 ARRAY['certificates', 'tls', 'pki', 'cryptography']),

-- ---------------------------------------------------------------------------
-- A.8.25  Secure development lifecycle — Policy
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000025', NULL, 'A.8.25', 'ISO27001',
 'Secure Development Policy',
 'Documented secure software development lifecycle (SSDLC) policy covering secure coding standards, code review requirements, and security testing gates.',
 'policy_document', 'manual_upload',
 'Export the approved secure development lifecycle policy or standards document. Ensure it covers: secure coding standards (e.g., OWASP Top 10 awareness), mandatory code review requirements, static/dynamic application security testing gates, dependency vulnerability scanning, and secure deployment procedures.',
 'annual', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]}]',
 'Policy must define secure coding standards, mandatory code review process, SAST/DAST requirements, dependency scanning, and be approved within the last 12 months.',
 'should_have', 'moderate', true,
 ARRAY['No OWASP or secure coding reference', 'Missing SAST/DAST requirements', 'No code review mandate', 'Outdated policy'],
 ARRAY['sdlc', 'secure-development', 'appsec', 'code-review']),

-- ---------------------------------------------------------------------------
-- A.8.25  Secure development lifecycle — Code review records
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000026', NULL, 'A.8.25', 'ISO27001',
 'Code Review and SAST Records',
 'Sample of code review approvals and static application security testing results from the most recent quarter.',
 'audit_report', 'system_export',
 'Export a sample of at least 10 merged pull requests from the main repository showing code review approvals (e.g., GitHub PR reviews, GitLab merge request approvals). Also export the most recent SAST scan summary from the CI/CD pipeline (e.g., SonarQube, Checkmarx, Snyk Code). Ensure the sample spans different teams and repositories.',
 'quarterly', 35,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx","html"]}]',
 'Evidence must show code review is enforced (no self-approvals, at least one reviewer), SAST scans run on every build, and critical/high findings are resolved before merge.',
 'should_have', 'moderate', true,
 ARRAY['Self-approved pull requests found', 'SAST not integrated into CI/CD', 'Critical findings merged without resolution', 'Sample size too small'],
 ARRAY['code-review', 'sast', 'appsec', 'ci-cd']),

-- ---------------------------------------------------------------------------
-- A.8.29  Security testing — Penetration test
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000027', NULL, 'A.8.29', 'ISO27001',
 'Penetration Test Report',
 'Report from an independent penetration test covering external and internal network, web applications, and social engineering vectors as applicable.',
 'penetration_test', 'manual_upload',
 'Obtain the final penetration test report from the contracted testing firm. The report should include scope and methodology, executive summary, detailed findings with CVSS scores, proof-of-concept evidence, risk ratings, and remediation recommendations. Confirm the testing period and that retesting of critical findings was conducted.',
 'annual', 15,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf"]},{"type":"contains_text","keywords":["scope","methodology","finding","remediation"]}]',
 'Report must be from a qualified independent firm, dated within 12 months, cover agreed scope, include CVSS-rated findings with remediation guidance, and show evidence of retest for critical findings.',
 'must_have', 'easy', true,
 ARRAY['Test older than 12 months', 'Scope too narrow or missing key assets', 'No remediation recommendations', 'Critical findings not retested', 'Internal-only test without external testing'],
 ARRAY['pentest', 'penetration-test', 'offensive-security', 'appsec']),

-- ---------------------------------------------------------------------------
-- A.5.9  Asset inventory
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000028', NULL, 'A.5.9', 'ISO27001',
 'Information Asset Inventory',
 'Comprehensive inventory of information assets and associated assets including classification, owner, and location.',
 'audit_report', 'system_export',
 'Export the asset inventory from the CMDB or asset management tool (e.g., ServiceNow, Snipe-IT, Lansweeper). Include asset name, type, classification level, owner, location, and criticality rating. Cross-reference with network discovery scans to identify any unmanaged assets.',
 'quarterly', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Inventory must cover hardware, software, data, and cloud assets. Each asset must have a named owner, classification level, and criticality rating. Unmanaged assets must be flagged.',
 'should_have', 'moderate', true,
 ARRAY['Assets without owners', 'Missing classification levels', 'Cloud assets not included', 'Stale inventory data'],
 ARRAY['asset-management', 'cmdb', 'inventory', 'classification']),

-- ---------------------------------------------------------------------------
-- A.5.12  Information classification
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000029', NULL, 'A.5.12', 'ISO27001',
 'Information Classification Scheme',
 'Documented information classification policy defining classification levels, handling rules, and labeling requirements.',
 'policy_document', 'manual_upload',
 'Export the approved information classification policy. The document should define classification levels (e.g., Public, Internal, Confidential, Restricted), handling procedures for each level, labeling requirements, declassification rules, and responsibilities of information owners and custodians.',
 'annual', 20,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]}]',
 'Document must define at least three classification levels with clear handling procedures, labeling standards, and be approved by management within the last 12 months.',
 'should_have', 'easy', true,
 ARRAY['Fewer than 3 classification levels', 'No handling rules per level', 'Missing management approval', 'No labeling requirements'],
 ARRAY['classification', 'data-handling', 'labeling', 'policy']),

-- ---------------------------------------------------------------------------
-- A.5.17  Authentication information management
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000030', NULL, 'A.5.17', 'ISO27001',
 'Password Policy Configuration',
 'Configuration export from the identity provider or directory service demonstrating password complexity, rotation, and lockout settings.',
 'configuration_screenshot', 'system_export',
 'Export the password policy settings from Active Directory (Group Policy), Azure AD, or the primary identity provider. Capture minimum length, complexity requirements, history enforcement, maximum age, lockout threshold, and lockout duration. If multiple directories exist, export settings from each.',
 'quarterly', 15,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["png","pdf","json","csv"]}]',
 'Password policy must enforce minimum 12 characters, complexity rules, account lockout after 5 failed attempts, and password history of at least 12 previous passwords.',
 'should_have', 'easy', true,
 ARRAY['Minimum length below 12 characters', 'No account lockout configured', 'Password history too short', 'No complexity requirements'],
 ARRAY['password-policy', 'authentication', 'identity', 'directory']),

-- ---------------------------------------------------------------------------
-- A.5.29  Business continuity
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000031', NULL, 'A.5.29', 'ISO27001',
 'Business Continuity Plan',
 'The documented business continuity plan covering critical business functions, recovery strategies, and communication procedures during disruptions.',
 'procedure_document', 'manual_upload',
 'Export the current business continuity plan from the document repository. Ensure it includes a business impact analysis summary, recovery time objectives per critical function, recovery strategies, crisis communication procedures, and testing schedule. Verify management approval and current version.',
 'annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]},{"type":"contains_text","keywords":["RTO","recovery","communication"]}]',
 'Plan must include BIA-driven RTOs, defined recovery strategies, crisis communication plan, and be approved within 12 months.',
 'should_have', 'moderate', true,
 ARRAY['No BIA or RTO definitions', 'Missing communication procedures', 'Outdated plan', 'No testing schedule'],
 ARRAY['bcp', 'business-continuity', 'bcdr', 'resilience']),

-- ---------------------------------------------------------------------------
-- A.5.30  ICT readiness for business continuity
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000032', NULL, 'A.5.30', 'ISO27001',
 'Disaster Recovery Test Results',
 'Results from the most recent DR failover test including recovery time achieved, data integrity verification, and lessons learned.',
 'audit_report', 'manual_upload',
 'Collect the DR test report from the most recent failover exercise. Document the systems tested, target RTO/RPO vs. actual recovery time, data integrity checks performed, any issues encountered, and corrective actions. Include participant list and sign-off from the DR coordinator.',
 'annual', 60,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]}]',
 'Test must cover critical systems, measure actual RTO/RPO against targets, verify data integrity post-failover, and include lessons learned with action items.',
 'should_have', 'complex', true,
 ARRAY['RTO/RPO targets not met', 'Data integrity not verified', 'Only tabletop, no actual failover', 'No lessons learned or action items'],
 ARRAY['dr-test', 'disaster-recovery', 'bcdr', 'failover']),

-- ---------------------------------------------------------------------------
-- A.8.1  User endpoint devices
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000033', NULL, 'A.8.1', 'ISO27001',
 'Endpoint Hardening Compliance Report',
 'Report showing endpoint devices comply with the organization''s hardening standards including disk encryption, EDR agent, and OS patch level.',
 'vulnerability_scan', 'system_export',
 'Export the endpoint compliance report from the MDM/UEM platform (e.g., Intune, Jamf, SCCM). The report should show each device''s compliance status against the hardening policy, including disk encryption status, OS version, EDR agent status, and last check-in date. Summarize the overall compliance percentage.',
 'quarterly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Report must show 95%+ endpoint compliance with hardening standards, disk encryption enabled, EDR agent active, and OS patched within SLA.',
 'should_have', 'moderate', true,
 ARRAY['Compliance below 95%', 'Devices without encryption', 'Missing EDR agents', 'Outdated OS versions'],
 ARRAY['endpoint', 'hardening', 'mdm', 'device-compliance']),

-- ---------------------------------------------------------------------------
-- A.5.31  Legal and regulatory requirements
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000034', NULL, 'A.5.31', 'ISO27001',
 'Regulatory Compliance Register',
 'Register of applicable legal, regulatory, and contractual requirements relevant to information security, with compliance status.',
 'audit_report', 'manual_upload',
 'Export the compliance obligations register from the GRC platform or retrieve the maintained spreadsheet. Each entry should list the regulation/standard, applicable requirements, compliance status, responsible owner, and last assessment date. Include entries for GDPR, industry-specific regulations, and key contractual obligations.',
 'semi_annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","xlsx","csv"]}]',
 'Register must cover all applicable regulations and key contracts, assign owners, include compliance status, and be reviewed within the last 6 months.',
 'nice_to_have', 'moderate', true,
 ARRAY['Missing key regulations', 'No compliance status indicated', 'Unassigned obligations', 'Outdated register'],
 ARRAY['compliance', 'regulatory', 'legal', 'obligations']),

-- ---------------------------------------------------------------------------
-- A.5.35  Independent review of information security
-- ---------------------------------------------------------------------------
('et000000-0001-0000-0000-000000000035', NULL, 'A.5.35', 'ISO27001',
 'Internal Audit Report for ISMS',
 'Report from the most recent internal audit of the ISMS covering scope, methodology, findings, nonconformities, and corrective action plans.',
 'audit_report', 'manual_upload',
 'Obtain the final internal audit report from the internal audit team or contracted auditors. The report should include audit scope, methodology, sampling approach, findings categorized as major/minor nonconformities or observations, and corrective action plans with owners and target dates. Include the audit program schedule for context.',
 'annual', 20,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]}]',
 'Report must cover the full ISMS scope, categorize findings, assign corrective actions with owners and due dates, and be completed within the last 12 months.',
 'must_have', 'easy', true,
 ARRAY['Audit scope too narrow', 'No corrective action plan', 'Findings not categorized', 'Older than 12 months'],
 ARRAY['internal-audit', 'isms', 'nonconformity', 'corrective-action']),


-- ============================================================================
-- PCI DSS v4.0 — 12 Templates
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1.2.1  Firewall rule review
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000001', NULL, '1.2.1', 'PCI-DSS',
 'Firewall and Router Rule Review Report',
 'Documented review of all firewall and router rule sets confirming business justification for each allow rule and verifying a default-deny posture.',
 'configuration_screenshot', 'system_export',
 'Export the complete firewall rule set and conduct a rule-by-rule review. For each allow rule, document the business justification, requesting party, and approval date. Flag any rules without justification, overly broad rules, or rules allowing traffic from untrusted networks to the CDE. Export review results as a signed-off report.',
 'semi_annual', 60,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","xlsx","csv"]}]',
 'Every allow rule must have documented business justification. Default deny must be in place. No unauthorized connections to the CDE. Review must be completed semi-annually.',
 'must_have', 'complex', true,
 ARRAY['Rules without business justification', 'No default deny', 'Unauthorized CDE access paths', 'Review overdue'],
 ARRAY['pci', 'firewall', 'rule-review', 'cde']),

-- ---------------------------------------------------------------------------
-- 2.2.1  Configuration standards
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000002', NULL, '2.2.1', 'PCI-DSS',
 'System Configuration Standards',
 'Documented configuration standards for all system components in the cardholder data environment, aligned with industry-accepted hardening guides.',
 'policy_document', 'manual_upload',
 'Export the configuration hardening standards applicable to CDE systems (servers, databases, network devices, POS terminals). Standards must reference CIS Benchmarks or vendor guides. Include the standard version, approval date, and the system types covered.',
 'annual', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]}]',
 'Standards must exist for each system type in the CDE, reference industry hardening guides (CIS/NIST), address removal of default credentials, and be approved within 12 months.',
 'should_have', 'moderate', true,
 ARRAY['Missing system types', 'No industry standard reference', 'Default credentials not addressed', 'Outdated standards'],
 ARRAY['pci', 'hardening', 'configuration', 'cde']),

-- ---------------------------------------------------------------------------
-- 6.3.3  Patch management
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000003', NULL, '6.3.3', 'PCI-DSS',
 'Patch Management Report',
 'Report showing critical and high-severity security patches are applied to CDE systems within defined SLAs (critical within 30 days per PCI DSS).',
 'vulnerability_scan', 'system_export',
 'Export the patch compliance report from the patch management tool (e.g., WSUS, SCCM, Ivanti, AWS Systems Manager). Filter to CDE-scoped systems. Show each system, outstanding patches, patch severity, days since release, and installation date for applied patches. Highlight any critical patches exceeding the 30-day SLA.',
 'monthly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'All critical/high patches must be applied within 30 days of release. Report must cover all CDE systems with zero critical patches outstanding beyond SLA.',
 'must_have', 'moderate', true,
 ARRAY['Critical patches beyond 30-day SLA', 'CDE systems missing from scope', 'No severity classification', 'Patch status unknown for some systems'],
 ARRAY['pci', 'patching', 'vulnerability-management', 'cde']),

-- ---------------------------------------------------------------------------
-- 8.3.6  Password complexity
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000004', NULL, '8.3.6', 'PCI-DSS',
 'Password/Authentication Policy Configuration',
 'Configuration evidence showing password complexity and authentication requirements for CDE access meet PCI DSS requirements.',
 'configuration_screenshot', 'system_export',
 'Export password policy settings from all authentication systems controlling CDE access (AD, application-level, database). Capture minimum length (12+ characters per PCI DSS v4.0), complexity, rotation period, history, and lockout settings. Include MFA configuration where applicable.',
 'quarterly', 20,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["png","pdf","json"]}]',
 'Password minimum length must be 12+ characters with numeric and alphabetic characters. Account lockout after no more than 10 failed attempts. Password history of at least 4.',
 'must_have', 'easy', true,
 ARRAY['Minimum length below 12 characters', 'No lockout policy', 'History fewer than 4 passwords', 'Not all CDE auth systems covered'],
 ARRAY['pci', 'password', 'authentication', 'cde']),

-- ---------------------------------------------------------------------------
-- 8.4.1  MFA for administrative access
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000005', NULL, '8.4.1', 'PCI-DSS',
 'MFA Configuration Evidence for CDE',
 'Evidence demonstrating multi-factor authentication is enforced for all administrative access to the cardholder data environment.',
 'configuration_screenshot', 'screenshot',
 'Capture screenshots of MFA enforcement settings from each system providing administrative access to the CDE: VPN gateway, jump/bastion hosts, cloud console, and application admin panels. Show that MFA is required (not optional) and cannot be bypassed. Include screenshots of the MFA provider configuration and conditional access policies.',
 'quarterly', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["png","jpg","pdf"]}]',
 'MFA must be enforced (not optional) for all administrative CDE access. No bypass mechanisms or exceptions unless documented and compensating controls exist.',
 'must_have', 'easy', true,
 ARRAY['MFA optional, not enforced', 'Bypass mechanism exists', 'Not all CDE entry points covered', 'MFA not on all admin access paths'],
 ARRAY['pci', 'mfa', 'authentication', 'admin-access']),

-- ---------------------------------------------------------------------------
-- 10.2.1  Audit log configuration
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000006', NULL, '10.2.1', 'PCI-DSS',
 'Audit Log Configuration Evidence',
 'Configuration evidence showing audit logging is enabled for all CDE systems, capturing required event types per PCI DSS Requirement 10.',
 'configuration_screenshot', 'system_export',
 'Export audit log configuration from each CDE system type: OS audit policy (auditd/Windows Security Policy), database audit settings, application logging configuration, and network device logging. For each, show that the required events are captured: user access, privilege use, access to cardholder data, actions by root/admin, log tampering attempts, and authentication events.',
 'quarterly', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","png","json","csv"]}]',
 'Configuration must capture all PCI DSS Req 10.2 event types: individual user access, privilege escalation, access to audit logs, invalid access attempts, use of identification and authentication mechanisms, initialization of audit logs, and creation/deletion of system-level objects.',
 'must_have', 'complex', true,
 ARRAY['Missing required event types', 'Not all CDE systems configured', 'Log forwarding not confirmed', 'No tamper protection on logs'],
 ARRAY['pci', 'logging', 'audit', 'cde']),

-- ---------------------------------------------------------------------------
-- 10.4.1  Audit log review
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000007', NULL, '10.4.1', 'PCI-DSS',
 'Daily Audit Log Review Records',
 'Records demonstrating that security events and logs from CDE systems are reviewed at least daily, either manually or through automated mechanisms.',
 'log_export', 'system_export',
 'Export evidence of daily log review from the SIEM or log management platform. This can be automated alert acknowledgment records, analyst review tickets, or SIEM correlation rule trigger and triage history. Show that reviews occur at least daily and cover all CDE log sources. Include the review date, analyst name, and disposition for each review period.',
 'monthly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Evidence must demonstrate daily review cadence for all CDE log sources, with analyst acknowledgment and documented triage of anomalies.',
 'should_have', 'moderate', true,
 ARRAY['Gaps in daily review coverage', 'Not all CDE sources included', 'No analyst sign-off', 'Anomalies not triaged'],
 ARRAY['pci', 'log-review', 'siem', 'daily-review']),

-- ---------------------------------------------------------------------------
-- 11.3.1  Internal vulnerability scan
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000008', NULL, '11.3.1', 'PCI-DSS',
 'Internal Vulnerability Scan Report (PCI)',
 'Quarterly internal vulnerability scan results for all CDE systems, showing that high-risk vulnerabilities are resolved and rescanned.',
 'vulnerability_scan', 'system_export',
 'Run a credentialed internal vulnerability scan of all CDE IP addresses and system components using an approved scanning tool. Generate the report showing all findings with CVSS scores. For any critical/high findings, document the remediation action and retest results. The quarterly scan must achieve a passing result (no critical/high unresolved vulnerabilities).',
 'quarterly', 35,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","html"]},{"type":"contains_text","keywords":["CVSS","critical","high"]}]',
 'Scan must cover all CDE components with credentialed checks, produce a passing result (no unresolved high/critical findings), and include retest evidence for remediated items.',
 'must_have', 'moderate', true,
 ARRAY['Unresolved critical/high vulnerabilities', 'Incomplete CDE coverage', 'Uncredentialed scan', 'No retest after remediation'],
 ARRAY['pci', 'vulnerability-scan', 'internal-scan', 'quarterly']),

-- ---------------------------------------------------------------------------
-- 11.3.2  External vulnerability scan (ASV)
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000009', NULL, '11.3.2', 'PCI-DSS',
 'External ASV Scan Report',
 'Quarterly external vulnerability scan performed by a PCI SSC Approved Scanning Vendor (ASV) with a passing result.',
 'vulnerability_scan', 'system_export',
 'Obtain the most recent quarterly ASV scan report from the approved scanning vendor (e.g., Qualys, Trustwave, SecurityMetrics). Confirm the scan covers all external-facing CDE IP addresses and URLs. The report must show a passing status. If it was initially failing, include evidence of remediation and the passing rescan.',
 'quarterly', 15,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf"]},{"type":"contains_text","keywords":["ASV","pass"]}]',
 'Report must be from a PCI SSC listed ASV, cover all external CDE IP addresses, and show a passing scan status for the quarter.',
 'must_have', 'easy', true,
 ARRAY['Failing scan status', 'Not from a PCI-listed ASV', 'Missing external IPs', 'Scan older than 90 days'],
 ARRAY['pci', 'asv', 'external-scan', 'quarterly']),

-- ---------------------------------------------------------------------------
-- 11.4.1  Penetration test
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000010', NULL, '11.4.1', 'PCI-DSS',
 'PCI Penetration Test Report',
 'Annual penetration test report covering network-layer and application-layer testing of the CDE and segmentation controls.',
 'penetration_test', 'manual_upload',
 'Obtain the final penetration test report from a qualified internal or external testing team. Verify the test covers both network-layer and application-layer testing of the CDE, plus segmentation validation. The report must include methodology (e.g., aligned with PCI Penetration Testing Guidance), scope, detailed findings with risk ratings, and remediation status.',
 'annual', 15,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf"]},{"type":"contains_text","keywords":["segmentation","network","application","finding"]}]',
 'Test must cover network-layer and application-layer testing of the CDE, validate segmentation controls, follow PCI penetration testing methodology, and include remediation of critical/high findings.',
 'must_have', 'easy', true,
 ARRAY['Segmentation testing missing', 'Application-layer testing not performed', 'Critical findings not remediated', 'Test older than 12 months'],
 ARRAY['pci', 'pentest', 'segmentation', 'cde']),

-- ---------------------------------------------------------------------------
-- 12.6.1  Security awareness training
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000011', NULL, '12.6.1', 'PCI-DSS',
 'PCI Security Awareness Training Records',
 'Records showing all CDE personnel completed security awareness training upon hire and at least annually thereafter.',
 'training_record', 'system_export',
 'Export the training completion report from the LMS filtered to personnel with CDE access. Show each employee name, role, hire date, training completion date, and acknowledgment of the acceptable use policy. New hires must complete training within 30 days. Include the training curriculum topics to confirm PCI-relevant content.',
 'annual', 25,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","xlsx","csv"]}]',
 'All CDE personnel must show training completion within the compliance period. New hires must complete within 30 days. Training content must cover PCI-specific topics.',
 'should_have', 'easy', true,
 ARRAY['Incomplete coverage of CDE personnel', 'New hires not trained within 30 days', 'Training content not PCI-specific', 'Missing acknowledgment signatures'],
 ARRAY['pci', 'training', 'awareness', 'cde-personnel']),

-- ---------------------------------------------------------------------------
-- 3.5.1  Encryption key management
-- ---------------------------------------------------------------------------
('et000000-0002-0000-0000-000000000012', NULL, '3.5.1', 'PCI-DSS',
 'Encryption Key Management Procedures and Evidence',
 'Documented key management procedures and evidence of proper key lifecycle management for encryption of stored cardholder data.',
 'procedure_document', 'manual_upload',
 'Export the encryption key management procedures document and supplement with evidence of key lifecycle operations: key generation records, custodian assignments, rotation schedule and last rotation date, split knowledge/dual control evidence, and key destruction records for retired keys. Include key custodian acknowledgment forms.',
 'annual', 45,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx","xlsx"]}]',
 'Procedures must cover full key lifecycle (generation, distribution, storage, rotation, destruction). Evidence must show keys are rotated per policy, custodians are assigned, and split knowledge/dual control is enforced.',
 'must_have', 'complex', true,
 ARRAY['Key rotation overdue', 'No split knowledge evidence', 'Missing key custodian assignments', 'Key destruction not documented'],
 ARRAY['pci', 'encryption', 'key-management', 'cryptography']),


-- ============================================================================
-- NIST 800-53 Rev 5 — 10 Templates
-- ============================================================================

-- ---------------------------------------------------------------------------
-- AC-2  Account management
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000001', NULL, 'AC-2', 'NIST800-53',
 'Account Inventory and Review Report',
 'Comprehensive inventory of all system accounts with periodic review evidence showing inactive, unauthorized, and orphaned accounts are identified and remediated.',
 'access_review', 'system_export',
 'Export the full account listing from each in-scope system (Active Directory, cloud IAM, application databases). For each account include: username, account type (user/service/admin), status (active/disabled), last login date, and assigned roles. Flag accounts inactive for 90+ days, accounts belonging to terminated employees, and service accounts without owners. Include the reviewer''s sign-off and remediation actions taken.',
 'quarterly', 50,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx"]},{"type":"row_count_min","min":1}]',
 'Report must cover all in-scope systems, flag inactive accounts (90+ days), identify orphaned accounts from terminated employees, and document remediation actions. Reviewer must sign off.',
 'must_have', 'moderate', true,
 ARRAY['Inactive accounts not flagged', 'Orphaned accounts from terminated employees', 'Missing system coverage', 'No reviewer sign-off'],
 ARRAY['nist', 'account-management', 'access-review', 'iam']),

-- ---------------------------------------------------------------------------
-- AU-2  Audit events
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000002', NULL, 'AU-2', 'NIST800-53',
 'Audit Event Configuration Documentation',
 'Documentation defining the auditable events selected for each system type, the rationale for selection, and configuration evidence showing events are captured.',
 'configuration_screenshot', 'system_export',
 'Document the organization''s selected auditable events for each system category (servers, network devices, databases, applications). For each event type, provide the rationale for inclusion based on risk assessment. Export audit policy configurations from representative systems (e.g., Windows Advanced Audit Policy, Linux auditd rules, cloud trail settings) as corroborating evidence.',
 'semi_annual', 35,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","docx","json","csv"]}]',
 'Documentation must specify auditable events per system type with risk-based rationale. Configuration exports must confirm the documented events are actually enabled on production systems.',
 'should_have', 'moderate', true,
 ARRAY['Events not risk-justified', 'Configuration does not match documentation', 'Missing system categories', 'No corroborating config export'],
 ARRAY['nist', 'audit', 'logging', 'event-selection']),

-- ---------------------------------------------------------------------------
-- CM-2  Baseline configuration
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000003', NULL, 'CM-2', 'NIST800-53',
 'Baseline Configuration Documentation',
 'Approved baseline configurations for all system types maintained under configuration control, including version history and change authorization records.',
 'policy_document', 'manual_upload',
 'Export the approved baseline configuration documents from the configuration management repository. For each system type include: baseline settings, version number, approval date, change history, and the change authorization record for the most recent update. Baselines should reference CIS Benchmarks or DISA STIGs where applicable.',
 'semi_annual', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","docx","xlsx"]}]',
 'Baselines must exist for all system types, be under version control, reference industry standards (CIS/STIG), include change authorization, and be reviewed within 6 months.',
 'must_have', 'moderate', true,
 ARRAY['Missing system types', 'No version control', 'Change not authorized', 'No industry standard reference'],
 ARRAY['nist', 'configuration', 'baseline', 'change-management']),

-- ---------------------------------------------------------------------------
-- IA-2  Identification and authentication
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000004', NULL, 'IA-2', 'NIST800-53',
 'Authentication Mechanism Configuration',
 'Configuration evidence demonstrating multi-factor authentication for privileged and network access, with cryptographic module validation.',
 'configuration_screenshot', 'system_export',
 'Export MFA configuration from the identity provider and remote access gateways. Show that MFA is enforced for all privileged access and remote network access. Include the authentication factors configured (something you know, have, are). If FIPS-validated cryptographic modules are required, include CMVP certificate numbers or validation evidence.',
 'quarterly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","png","json"]}]',
 'MFA must be enforced for all privileged and remote access. Authentication mechanisms should use FIPS-validated modules where required. Configuration must show factors used and enforcement mode.',
 'must_have', 'moderate', true,
 ARRAY['MFA not enforced for privileged access', 'Remote access without MFA', 'Non-FIPS modules where FIPS required', 'Single-factor authentication paths exist'],
 ARRAY['nist', 'authentication', 'mfa', 'fips']),

-- ---------------------------------------------------------------------------
-- IR-4  Incident handling
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000005', NULL, 'IR-4', 'NIST800-53',
 'Incident Handling Procedures and Capability Evidence',
 'Documented incident handling procedures with evidence of operational capability including tools, training, and recent incident records.',
 'procedure_document', 'manual_upload',
 'Export the incident handling procedures covering preparation, detection and analysis, containment, eradication and recovery, and post-incident activity (aligned with NIST SP 800-61). Supplement with evidence of operational capability: incident management tool configuration (e.g., SOAR platform), IR team training records, and a log of recent incidents handled with disposition.',
 'annual', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":365},{"type":"file_type","allowed":["pdf","docx"]},{"type":"contains_text","keywords":["detection","containment","recovery","post-incident"]}]',
 'Procedures must align with NIST SP 800-61, cover all phases of incident handling, and be supplemented with evidence of operational capability (tools, training, recent incident log).',
 'must_have', 'moderate', true,
 ARRAY['Missing incident phases', 'No operational capability evidence', 'Procedures not aligned with SP 800-61', 'No recent incident log'],
 ARRAY['nist', 'incident-response', 'ir', 'sp800-61']),

-- ---------------------------------------------------------------------------
-- RA-5  Vulnerability monitoring and scanning
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000006', NULL, 'RA-5', 'NIST800-53',
 'Vulnerability Monitoring and Scanning Report',
 'Results from vulnerability scanning across the information system, including analysis of findings, remediation timelines, and trend data.',
 'vulnerability_scan', 'system_export',
 'Export vulnerability scan results from the enterprise scanner covering all system components. Include executive summary with trend comparison to prior periods, detailed findings with CVSS scores and CVE identifiers, remediation timelines by severity, and any accepted risks with documented justification. Confirm scans use updated vulnerability databases.',
 'monthly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","csv","html"]}]',
 'Scans must cover all system components, use current vulnerability databases, report CVSS-scored findings, show trend data, and demonstrate remediation within defined timelines.',
 'must_have', 'moderate', true,
 ARRAY['Incomplete system coverage', 'Outdated vulnerability database', 'No trend analysis', 'Remediation timelines exceeded'],
 ARRAY['nist', 'vulnerability', 'scanning', 'risk-assessment']),

-- ---------------------------------------------------------------------------
-- SC-7  Boundary protection
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000007', NULL, 'SC-7', 'NIST800-53',
 'Network Boundary Protection Configuration',
 'Evidence of boundary protection controls at external and key internal boundaries including firewall rules, DMZ architecture, and traffic flow enforcement.',
 'configuration_screenshot', 'system_export',
 'Export firewall configurations at the external boundary and internal segmentation points. Include the network architecture diagram showing managed interfaces, DMZ topology, and traffic flow directions. Provide the access control list configuration demonstrating default deny for inbound traffic and controlled outbound traffic. Include IDS/IPS placement evidence.',
 'semi_annual', 45,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":200},{"type":"file_type","allowed":["pdf","csv","json","png"]}]',
 'Evidence must demonstrate default-deny at external boundaries, DMZ architecture for public-facing services, controlled internal segmentation, and IDS/IPS monitoring at key boundaries.',
 'should_have', 'complex', true,
 ARRAY['No default deny on external boundary', 'DMZ not implemented', 'Missing internal segmentation', 'No IDS/IPS evidence'],
 ARRAY['nist', 'boundary', 'firewall', 'network-security']),

-- ---------------------------------------------------------------------------
-- CP-9  System backup
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000008', NULL, 'CP-9', 'NIST800-53',
 'System Backup Evidence and Testing',
 'Evidence of system-level and user-level backup operations including backup logs, integrity verification, and periodic restoration testing.',
 'audit_report', 'system_export',
 'Export backup job completion logs from the backup platform for the most recent month, showing success/failure status for each job. Include backup integrity verification results (checksum validation, consistency checks). Supplement with the most recent restoration test report showing the test date, systems restored, time to restore, and data integrity verification.',
 'quarterly', 40,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Backup logs must show consistent successful completion. Integrity verification must be performed regularly. Restoration testing must demonstrate recovery within RTO and verified data integrity.',
 'should_have', 'moderate', true,
 ARRAY['Backup failures not remediated', 'No integrity verification', 'Restoration test not performed', 'RTO exceeded in test'],
 ARRAY['nist', 'backup', 'restoration', 'contingency']),

-- ---------------------------------------------------------------------------
-- SI-2  Flaw remediation
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000009', NULL, 'SI-2', 'NIST800-53',
 'Flaw Remediation and Patch Compliance Report',
 'Report tracking identification, prioritization, and remediation of software flaws across the information system, including patch deployment metrics.',
 'vulnerability_scan', 'system_export',
 'Export the patch compliance dashboard from the patch management or vulnerability management tool. Show the total number of outstanding patches by severity, mean time to remediate by severity level, patch deployment success rate, and systems out of compliance. Include trend data comparing current period to prior periods and highlight any patches failing to deploy.',
 'monthly', 30,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":45},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Report must show critical flaws remediated within 15 days, high within 30 days. Patch deployment success rate must exceed 95%. Systems out of compliance must have documented remediation plans.',
 'should_have', 'moderate', true,
 ARRAY['Critical patches beyond 15-day SLA', 'Deployment success rate below 95%', 'No trend analysis', 'Missing remediation plans for non-compliant systems'],
 ARRAY['nist', 'patching', 'flaw-remediation', 'compliance']),

-- ---------------------------------------------------------------------------
-- PE-3  Physical access control
-- ---------------------------------------------------------------------------
('et000000-0003-0000-0000-000000000010', NULL, 'PE-3', 'NIST800-53',
 'Physical Access Control Configuration and Logs',
 'Evidence of physical access controls at facility entry points and data center/server rooms, including access logs and authorized personnel lists.',
 'audit_report', 'system_export',
 'Export the physical access control system (PACS) configuration showing controlled entry points, badge reader placement, and access zones. Pull the authorized access list for data center and server room zones. Export a sample of physical access logs (past 30 days) showing badge-in events. Cross-reference with HR data to confirm only authorized personnel accessed restricted areas.',
 'quarterly', 35,
 '[{"type":"file_not_empty"},{"type":"date_within","max_age_days":100},{"type":"file_type","allowed":["pdf","csv","xlsx"]}]',
 'Evidence must show all restricted areas have badge-controlled entry, authorized personnel lists are current, access logs show no unauthorized entries, and terminated personnel badges are promptly deactivated.',
 'should_have', 'moderate', true,
 ARRAY['Unauthorized access detected', 'Terminated personnel still on access list', 'Missing badge reader at entry point', 'Access logs incomplete'],
 ARRAY['nist', 'physical-access', 'data-center', 'badge']);

COMMIT;
