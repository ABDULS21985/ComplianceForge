-- Questionnaire Template Seed Data for ComplianceForge
-- Seeds 4 system questionnaire templates with sections and questions
-- Generated: 2026-03-28

BEGIN;

-- ============================================================================
-- Template 1: ComplianceForge Standard Security Assessment
-- ============================================================================
INSERT INTO questionnaire_templates (id, name, description, version, category, framework_mappings, is_system_template, is_active, created_at, updated_at)
VALUES (
    'q0000000-0000-0000-0000-000000000001',
    'ComplianceForge Standard Security Assessment',
    'Comprehensive security assessment questionnaire covering governance, access control, data protection, incident response, and vulnerability management. Suitable for evaluating third-party vendors and internal security posture.',
    '1.0',
    'security_assessment',
    '["ISO 27001", "NIST CSF", "SOC 2"]'::jsonb,
    true,
    true,
    NOW(),
    NOW()
);

-- Section 1: Governance & Policies
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000001',
    'q0000000-0000-0000-0000-000000000001',
    'Governance & Policies',
    'Assess the organization''s security governance structure, policies, and oversight mechanisms.',
    1,
    20,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000001',
    'qs000000-0000-0000-0000-000000000001',
    'Does the organization have a formally documented information security policy?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'A formal information security policy should be approved by management, published, and communicated to all employees and relevant external parties. It should define the organization''s approach to managing information security objectives.',
    '["ISO 27001 A.5.1", "NIST CSF ID.GV-1", "SOC 2 CC1.1"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000002',
    'qs000000-0000-0000-0000-000000000001',
    'Is the information security policy reviewed and updated at least annually?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Policies should be reviewed at planned intervals or when significant changes occur to ensure their continuing suitability, adequacy, and effectiveness.',
    '["ISO 27001 A.5.1.2", "NIST CSF ID.GV-4"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000003',
    'qs000000-0000-0000-0000-000000000001',
    'Are information security roles and responsibilities clearly defined and assigned?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'All information security responsibilities should be defined and allocated. This includes designating a CISO or equivalent role, defining responsibilities for asset owners, and establishing clear accountability.',
    '["ISO 27001 A.6.1.1", "NIST CSF ID.AM-6", "SOC 2 CC1.3"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000004',
    'qs000000-0000-0000-0000-000000000001',
    'Is a formal risk assessment conducted at least annually?',
    'yes_no_na',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}, "na": {"score": null, "label": "Not Applicable"}}'::jsonb,
    5,
    'critical',
    true,
    'A structured risk assessment process should identify, analyze, and evaluate information security risks. This should be conducted at planned intervals or when significant changes are proposed or occur.',
    '["ISO 27001 A.8.2", "NIST CSF ID.RA-3", "SOC 2 CC3.2"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000005',
    'qs000000-0000-0000-0000-000000000001',
    'Does the organization maintain a formal compliance program covering applicable laws and regulations?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'The organization should identify all applicable legislative, regulatory, and contractual requirements and document its approach to meeting these requirements.',
    '["ISO 27001 A.18.1.1", "NIST CSF ID.GV-3", "SOC 2 CC2.2"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 2: Access Control
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000002',
    'q0000000-0000-0000-0000-000000000001',
    'Access Control',
    'Evaluate the organization''s access control mechanisms, authentication practices, and user access management.',
    2,
    20,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000006',
    'qs000000-0000-0000-0000-000000000002',
    'Is multi-factor authentication (MFA) enforced for all user accounts?',
    'single_choice',
    '{"choices": [{"value": "all_users", "label": "Yes, for all users", "score": 100}, {"value": "privileged_only", "label": "Only for privileged accounts", "score": 60}, {"value": "optional", "label": "Available but optional", "score": 20}, {"value": "not_implemented", "label": "Not implemented", "score": 0}]}'::jsonb,
    5,
    'critical',
    true,
    'MFA adds a critical layer of security beyond passwords. It should be enforced for all users, especially for remote access, privileged accounts, and access to sensitive systems.',
    '["ISO 27001 A.9.4.2", "NIST CSF PR.AC-7", "SOC 2 CC6.1"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000007',
    'qs000000-0000-0000-0000-000000000002',
    'Is privileged access restricted and monitored?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Privileged access (admin, root, superuser) should be tightly controlled, logged, and monitored. Use of privileged access management (PAM) solutions is recommended.',
    '["ISO 27001 A.9.2.3", "NIST CSF PR.AC-4", "SOC 2 CC6.3"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000008',
    'qs000000-0000-0000-0000-000000000002',
    'Are user access reviews conducted at least quarterly?',
    'single_choice',
    '{"choices": [{"value": "quarterly", "label": "Quarterly or more frequently", "score": 100}, {"value": "semi_annually", "label": "Semi-annually", "score": 70}, {"value": "annually", "label": "Annually", "score": 40}, {"value": "never", "label": "Not conducted", "score": 0}]}'::jsonb,
    4,
    'high',
    true,
    'Regular access reviews ensure that user access rights are appropriate and that terminated or transferred employees no longer have access to systems they should not.',
    '["ISO 27001 A.9.2.5", "NIST CSF PR.AC-1", "SOC 2 CC6.2"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000009',
    'qs000000-0000-0000-0000-000000000002',
    'Does the organization enforce a strong password policy?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    3,
    'medium',
    true,
    'A password policy should enforce minimum length (12+ characters recommended), complexity requirements, prohibition of common passwords, and regular rotation or breach-based rotation.',
    '["ISO 27001 A.9.4.3", "NIST CSF PR.AC-1", "SOC 2 CC6.1"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000010',
    'qs000000-0000-0000-0000-000000000002',
    'Is remote access secured via VPN or zero-trust architecture?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'All remote access should be authenticated, encrypted, and logged. Modern approaches include zero-trust network access (ZTNA) where every access request is verified regardless of location.',
    '["ISO 27001 A.6.2.2", "NIST CSF PR.AC-3", "SOC 2 CC6.6"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 3: Data Protection
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000003',
    'q0000000-0000-0000-0000-000000000001',
    'Data Protection',
    'Assess the organization''s data protection practices including classification, encryption, retention, and third-party data processing.',
    3,
    20,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000011',
    'qs000000-0000-0000-0000-000000000003',
    'Does the organization have a data classification scheme?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Data should be classified according to its sensitivity and criticality (e.g., public, internal, confidential, restricted). Classification drives the application of appropriate protection measures.',
    '["ISO 27001 A.8.2.1", "NIST CSF ID.AM-5", "SOC 2 CC6.7"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000012',
    'qs000000-0000-0000-0000-000000000003',
    'Is encryption applied to sensitive data at rest?',
    'single_choice',
    '{"choices": [{"value": "aes256", "label": "Yes, AES-256 or equivalent", "score": 100}, {"value": "aes128", "label": "Yes, AES-128 or equivalent", "score": 80}, {"value": "partial", "label": "Partially encrypted", "score": 40}, {"value": "none", "label": "No encryption at rest", "score": 0}]}'::jsonb,
    5,
    'critical',
    true,
    'Sensitive data should be encrypted at rest using industry-standard algorithms (AES-256 recommended). This includes databases, file storage, backups, and removable media.',
    '["ISO 27001 A.10.1.1", "NIST CSF PR.DS-1", "SOC 2 CC6.7"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000013',
    'qs000000-0000-0000-0000-000000000003',
    'Is encryption applied to data in transit (TLS 1.2+)?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'All data transmitted over networks should be encrypted using TLS 1.2 or higher. Older protocols (SSL, TLS 1.0, TLS 1.1) should be disabled.',
    '["ISO 27001 A.10.1.1", "NIST CSF PR.DS-2", "SOC 2 CC6.7"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000014',
    'qs000000-0000-0000-0000-000000000003',
    'Does the organization have a documented data retention and disposal policy?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    3,
    'medium',
    true,
    'A data retention policy should define retention periods for different data categories, disposal methods, and legal hold procedures. Data should be securely destroyed when no longer needed.',
    '["ISO 27001 A.8.3.2", "NIST CSF PR.IP-6", "SOC 2 CC6.5"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000015',
    'qs000000-0000-0000-0000-000000000003',
    'Are Data Processing Agreements (DPAs) in place with all third-party data processors?',
    'yes_no_na',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}, "na": {"score": null, "label": "Not Applicable"}}'::jsonb,
    4,
    'high',
    true,
    'DPAs should be executed with all third parties that process personal or sensitive data on behalf of the organization, ensuring contractual obligations for data protection.',
    '["GDPR Art. 28", "ISO 27001 A.15.1.2", "SOC 2 CC9.2"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 4: Incident Response
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000004',
    'q0000000-0000-0000-0000-000000000001',
    'Incident Response',
    'Evaluate the organization''s ability to detect, respond to, and recover from security incidents.',
    4,
    20,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000016',
    'qs000000-0000-0000-0000-000000000004',
    'Does the organization have a documented incident response plan?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'An incident response plan should define roles, communication procedures, escalation paths, containment strategies, eradication steps, and recovery procedures for various incident types.',
    '["ISO 27001 A.16.1.1", "NIST CSF RS.RP-1", "SOC 2 CC7.3"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000017',
    'qs000000-0000-0000-0000-000000000004',
    'Is the incident response plan tested at least annually (tabletop exercises or simulations)?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Regular testing through tabletop exercises, functional drills, or full-scale simulations ensures the IR plan remains effective and that team members understand their roles.',
    '["ISO 27001 A.17.1.3", "NIST CSF RS.RP-1", "SOC 2 CC7.4"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000018',
    'qs000000-0000-0000-0000-000000000004',
    'Is there a defined breach notification process meeting regulatory timelines?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Breach notification processes should comply with applicable regulations (e.g., GDPR 72-hour requirement, state breach notification laws) and include templates and contact lists.',
    '["GDPR Art. 33", "NIST CSF RS.CO-2", "SOC 2 CC7.3"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000019',
    'qs000000-0000-0000-0000-000000000004',
    'Are post-incident lessons learned documented and used to improve controls?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    3,
    'medium',
    true,
    'After each incident, a lessons-learned review should be conducted to identify root causes, evaluate the effectiveness of the response, and implement improvements to prevent recurrence.',
    '["ISO 27001 A.16.1.6", "NIST CSF RS.IM-1", "SOC 2 CC7.5"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000020',
    'qs000000-0000-0000-0000-000000000004',
    'Does the organization maintain 24/7 security monitoring capabilities?',
    'single_choice',
    '{"choices": [{"value": "internal_247", "label": "Yes, internal 24/7 SOC", "score": 100}, {"value": "managed_soc", "label": "Yes, managed SOC/MDR provider", "score": 90}, {"value": "business_hours", "label": "Business hours only with on-call", "score": 50}, {"value": "none", "label": "No continuous monitoring", "score": 0}]}'::jsonb,
    4,
    'high',
    true,
    'Continuous security monitoring through a Security Operations Center (SOC) or Managed Detection and Response (MDR) service enables timely detection and response to security events.',
    '["ISO 27001 A.12.4.1", "NIST CSF DE.CM-1", "SOC 2 CC7.2"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 5: Vulnerability Management
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000005',
    'q0000000-0000-0000-0000-000000000001',
    'Vulnerability Management',
    'Assess the organization''s approach to identifying, prioritizing, and remediating security vulnerabilities.',
    5,
    20,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000021',
    'qs000000-0000-0000-0000-000000000005',
    'How frequently are vulnerability scans conducted?',
    'single_choice',
    '{"choices": [{"value": "continuous", "label": "Continuous / real-time", "score": 100}, {"value": "weekly", "label": "Weekly", "score": 85}, {"value": "monthly", "label": "Monthly", "score": 60}, {"value": "quarterly", "label": "Quarterly", "score": 30}, {"value": "never", "label": "Not conducted", "score": 0}]}'::jsonb,
    5,
    'critical',
    true,
    'Regular vulnerability scanning of all systems, networks, and applications is essential. Continuous or weekly scanning is recommended for critical assets; monthly at minimum.',
    '["ISO 27001 A.12.6.1", "NIST CSF DE.CM-8", "SOC 2 CC7.1"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000022',
    'qs000000-0000-0000-0000-000000000005',
    'Is penetration testing conducted at least annually by qualified professionals?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Annual penetration testing by qualified internal or external professionals helps identify vulnerabilities that automated scanning may miss. Testing should cover network, application, and social engineering vectors.',
    '["ISO 27001 A.18.2.3", "NIST CSF DE.CM-8", "SOC 2 CC4.1"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000023',
    'qs000000-0000-0000-0000-000000000005',
    'Are critical and high-severity patches applied within defined SLAs?',
    'single_choice',
    '{"choices": [{"value": "24h_critical", "label": "Critical: 24h, High: 7 days", "score": 100}, {"value": "7d_critical", "label": "Critical: 7 days, High: 30 days", "score": 70}, {"value": "30d_critical", "label": "Critical: 30 days, High: 90 days", "score": 30}, {"value": "no_sla", "label": "No defined SLAs", "score": 0}]}'::jsonb,
    5,
    'critical',
    true,
    'Patch management SLAs should be defined based on vulnerability severity. Recommended: critical patches within 24-48 hours, high within 7 days, medium within 30 days, low within 90 days.',
    '["ISO 27001 A.12.6.1", "NIST CSF PR.IP-12", "SOC 2 CC7.1"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000024',
    'qs000000-0000-0000-0000-000000000005',
    'Is there a formal change management process for production systems?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'A formal change management process should include risk assessment, approval workflows, testing requirements, rollback procedures, and post-implementation review for all production changes.',
    '["ISO 27001 A.12.1.2", "NIST CSF PR.IP-3", "SOC 2 CC8.1"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000025',
    'qs000000-0000-0000-0000-000000000005',
    'Does the organization follow secure software development practices (SSDLC)?',
    'yes_no_na',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}, "na": {"score": null, "label": "Not Applicable"}}'::jsonb,
    4,
    'high',
    true,
    'Secure development practices should include security requirements analysis, threat modeling, secure coding standards, code review, SAST/DAST testing, and security testing in CI/CD pipelines.',
    '["ISO 27001 A.14.2.1", "NIST CSF PR.IP-2", "SOC 2 CC8.1"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- ============================================================================
-- Template 2: GDPR Article 28 Processor Assessment
-- ============================================================================
INSERT INTO questionnaire_templates (id, name, description, version, category, framework_mappings, is_system_template, is_active, created_at, updated_at)
VALUES (
    'q0000000-0000-0000-0000-000000000002',
    'GDPR Article 28 Processor Assessment',
    'Assessment questionnaire for evaluating data processors under GDPR Article 28 requirements. Covers data processing activities, data subject rights, and technical/organizational security measures.',
    '1.0',
    'gdpr_assessment',
    '["GDPR"]'::jsonb,
    true,
    true,
    NOW(),
    NOW()
);

-- Section 1: Data Processing
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000006',
    'q0000000-0000-0000-0000-000000000002',
    'Data Processing',
    'Evaluate data processing activities, lawful basis, and processing agreements.',
    1,
    40,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000026',
    'qs000000-0000-0000-0000-000000000006',
    'Does the processor maintain a record of all processing activities (Article 30)?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Under GDPR Article 30, processors must maintain a record of all categories of processing activities carried out on behalf of a controller.',
    '["GDPR Art. 30"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000027',
    'qs000000-0000-0000-0000-000000000006',
    'Does the processor only process personal data on documented instructions from the controller?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'GDPR Article 28(3)(a) requires the processor to process personal data only on documented instructions from the controller, unless required by EU or Member State law.',
    '["GDPR Art. 28(3)(a)"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000028',
    'qs000000-0000-0000-0000-000000000006',
    'Has the processor appointed a Data Protection Officer (DPO) where required?',
    'yes_no_na',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}, "na": {"score": null, "label": "Not Required"}}'::jsonb,
    4,
    'high',
    true,
    'A DPO must be appointed where the core activities require regular and systematic monitoring of data subjects on a large scale, or consist of large-scale processing of special categories of data.',
    '["GDPR Art. 37"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000029',
    'qs000000-0000-0000-0000-000000000006',
    'Are sub-processors engaged only with prior written authorization of the controller?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Under Article 28(2), the processor shall not engage another processor without prior specific or general written authorization of the controller.',
    '["GDPR Art. 28(2)"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000030',
    'qs000000-0000-0000-0000-000000000006',
    'Are data transfers outside the EEA conducted with appropriate safeguards?',
    'yes_no_na',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}, "na": {"score": null, "label": "No transfers outside EEA"}}'::jsonb,
    5,
    'critical',
    true,
    'Transfers to third countries require appropriate safeguards such as Standard Contractual Clauses (SCCs), Binding Corporate Rules (BCRs), or adequacy decisions.',
    '["GDPR Art. 44-49"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 2: Data Subject Rights
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000007',
    'q0000000-0000-0000-0000-000000000002',
    'Data Subject Rights',
    'Assess the processor''s ability to support the controller in fulfilling data subject rights requests.',
    2,
    30,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000031',
    'qs000000-0000-0000-0000-000000000007',
    'Can the processor assist the controller in responding to data subject access requests (DSARs)?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'The processor must assist the controller by appropriate technical and organizational measures to fulfil the controller''s obligation to respond to requests for exercising data subject rights.',
    '["GDPR Art. 28(3)(e)"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000032',
    'qs000000-0000-0000-0000-000000000007',
    'Can the processor support data portability requests by providing data in a structured, machine-readable format?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Data subjects have the right to receive their personal data in a structured, commonly used, and machine-readable format (Article 20).',
    '["GDPR Art. 20"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000033',
    'qs000000-0000-0000-0000-000000000007',
    'Can the processor support the right to erasure by deleting personal data upon request?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'The processor must be able to delete personal data when the controller instructs, supporting the right to erasure (Article 17).',
    '["GDPR Art. 17"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000034',
    'qs000000-0000-0000-0000-000000000007',
    'Does the processor delete or return all personal data at the end of the service contract?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'At the end of the provision of services, the processor shall delete or return all personal data to the controller and delete existing copies, per Article 28(3)(g).',
    '["GDPR Art. 28(3)(g)"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000035',
    'qs000000-0000-0000-0000-000000000007',
    'Does the processor have processes to notify the controller of a data breach without undue delay (within 72 hours)?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'The processor shall notify the controller without undue delay after becoming aware of a personal data breach, per Article 33(2).',
    '["GDPR Art. 33(2)"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 3: Security Measures
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000008',
    'q0000000-0000-0000-0000-000000000002',
    'Security Measures',
    'Evaluate the technical and organizational measures implemented by the processor to ensure the security of processing.',
    3,
    30,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000036',
    'qs000000-0000-0000-0000-000000000008',
    'Does the processor implement pseudonymization and encryption of personal data?',
    'single_choice',
    '{"choices": [{"value": "both", "label": "Both pseudonymization and encryption", "score": 100}, {"value": "encryption_only", "label": "Encryption only", "score": 70}, {"value": "pseudonymization_only", "label": "Pseudonymization only", "score": 50}, {"value": "neither", "label": "Neither", "score": 0}]}'::jsonb,
    5,
    'critical',
    true,
    'Article 32(1)(a) specifically references pseudonymization and encryption as appropriate technical measures to ensure security of processing.',
    '["GDPR Art. 32(1)(a)"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000037',
    'qs000000-0000-0000-0000-000000000008',
    'Does the processor ensure ongoing confidentiality, integrity, availability, and resilience of processing systems?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Article 32(1)(b) requires the ability to ensure the ongoing confidentiality, integrity, availability, and resilience of processing systems and services.',
    '["GDPR Art. 32(1)(b)"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000038',
    'qs000000-0000-0000-0000-000000000008',
    'Does the processor have the ability to restore availability and access to personal data in a timely manner following an incident?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Article 32(1)(c) requires the ability to restore the availability and access to personal data in a timely manner in the event of a physical or technical incident.',
    '["GDPR Art. 32(1)(c)"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000039',
    'qs000000-0000-0000-0000-000000000008',
    'Does the processor regularly test and evaluate the effectiveness of security measures?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Article 32(1)(d) requires a process for regularly testing, assessing, and evaluating the effectiveness of technical and organizational measures for ensuring the security of the processing.',
    '["GDPR Art. 32(1)(d)"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000040',
    'qs000000-0000-0000-0000-000000000008',
    'Does the processor ensure that persons authorized to process personal data are bound by confidentiality obligations?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Article 28(3)(b) requires that persons authorized to process personal data have committed themselves to confidentiality or are under an appropriate statutory obligation of confidentiality.',
    '["GDPR Art. 28(3)(b)"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- ============================================================================
-- Template 3: NIS2 Supply Chain Security Assessment
-- ============================================================================
INSERT INTO questionnaire_templates (id, name, description, version, category, framework_mappings, is_system_template, is_active, created_at, updated_at)
VALUES (
    'q0000000-0000-0000-0000-000000000003',
    'NIS2 Supply Chain Security Assessment',
    'Assessment questionnaire aligned with NIS2 Directive requirements for evaluating supply chain cybersecurity. Covers cybersecurity risk management measures and incident reporting obligations.',
    '1.0',
    'nis2_assessment',
    '["NIS2"]'::jsonb,
    true,
    true,
    NOW(),
    NOW()
);

-- Section 1: Cyber Security Measures
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000009',
    'q0000000-0000-0000-0000-000000000003',
    'Cyber Security Measures',
    'Evaluate the cybersecurity risk management measures required under NIS2 Article 21.',
    1,
    60,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000041',
    'qs000000-0000-0000-0000-000000000009',
    'Does the organization have a risk analysis and information system security policy?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'NIS2 Article 21(2)(a) requires policies on risk analysis and information system security as a minimum cybersecurity risk-management measure.',
    '["NIS2 Art. 21(2)(a)"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000042',
    'qs000000-0000-0000-0000-000000000009',
    'Does the organization have business continuity and crisis management procedures?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'NIS2 Article 21(2)(c) requires business continuity, such as backup management and disaster recovery, and crisis management.',
    '["NIS2 Art. 21(2)(c)"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000043',
    'qs000000-0000-0000-0000-000000000009',
    'Does the organization address supply chain security including security-related aspects of relationships with direct suppliers?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'NIS2 Article 21(2)(d) requires supply chain security, including security-related aspects concerning the relationships between each entity and its direct suppliers or service providers.',
    '["NIS2 Art. 21(2)(d)"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000044',
    'qs000000-0000-0000-0000-000000000009',
    'Does the organization implement security in network and information system acquisition, development, and maintenance?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'NIS2 Article 21(2)(e) requires security in network and information systems acquisition, development, and maintenance, including vulnerability handling and disclosure.',
    '["NIS2 Art. 21(2)(e)"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000045',
    'qs000000-0000-0000-0000-000000000009',
    'Does the organization use cryptography and encryption where appropriate?',
    'single_choice',
    '{"choices": [{"value": "comprehensive", "label": "Comprehensive cryptography program", "score": 100}, {"value": "partial", "label": "Partially implemented", "score": 50}, {"value": "none", "label": "Not implemented", "score": 0}]}'::jsonb,
    4,
    'high',
    true,
    'NIS2 Article 21(2)(h) requires policies and procedures regarding the use of cryptography and, where appropriate, encryption.',
    '["NIS2 Art. 21(2)(h)"]'::jsonb,
    5,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000046',
    'qs000000-0000-0000-0000-000000000009',
    'Does the organization use multi-factor authentication and secured communications?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'NIS2 Article 21(2)(j) requires the use of multi-factor authentication or continuous authentication solutions, secured voice, video, and text communications.',
    '["NIS2 Art. 21(2)(j)"]'::jsonb,
    6,
    NOW(),
    NOW()
);

-- Section 2: Incident Reporting
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000010',
    'q0000000-0000-0000-0000-000000000003',
    'Incident Reporting',
    'Evaluate the organization''s incident reporting capabilities as required under NIS2 Article 23.',
    2,
    40,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000047',
    'qs000000-0000-0000-0000-000000000010',
    'Can the organization provide an early warning within 24 hours of becoming aware of a significant incident?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'NIS2 Article 23(4)(a) requires an early warning to the CSIRT or competent authority within 24 hours of becoming aware of a significant incident.',
    '["NIS2 Art. 23(4)(a)"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000048',
    'qs000000-0000-0000-0000-000000000010',
    'Can the organization provide an incident notification within 72 hours with initial assessment?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'NIS2 Article 23(4)(b) requires an incident notification within 72 hours, including an initial assessment of the incident severity and impact.',
    '["NIS2 Art. 23(4)(b)"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000049',
    'qs000000-0000-0000-0000-000000000010',
    'Can the organization provide a final incident report within one month?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'NIS2 Article 23(4)(d) requires a final report within one month of the incident notification, including a detailed description, root cause, mitigation measures, and cross-border impact.',
    '["NIS2 Art. 23(4)(d)"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000050',
    'qs000000-0000-0000-0000-000000000010',
    'Does the organization have processes to determine if an incident is significant based on NIS2 criteria?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Organizations must be able to determine if an incident is significant, i.e., has caused or is capable of causing severe operational disruption or financial loss, or has affected other persons by causing considerable damage.',
    '["NIS2 Art. 23(3)"]'::jsonb,
    4,
    NOW(),
    NOW()
);

-- ============================================================================
-- Template 4: Quick Security Assessment
-- ============================================================================
INSERT INTO questionnaire_templates (id, name, description, version, category, framework_mappings, is_system_template, is_active, created_at, updated_at)
VALUES (
    'q0000000-0000-0000-0000-000000000004',
    'Quick Security Assessment',
    'A streamlined security assessment for rapid evaluation of essential security controls and data protection basics. Suitable for low-risk vendors or initial screening.',
    '1.0',
    'quick_assessment',
    '["ISO 27001", "NIST CSF"]'::jsonb,
    true,
    true,
    NOW(),
    NOW()
);

-- Section 1: Essential Controls
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000011',
    'q0000000-0000-0000-0000-000000000004',
    'Essential Controls',
    'Evaluate the most critical security controls that every organization should have in place.',
    1,
    50,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000051',
    'qs000000-0000-0000-0000-000000000011',
    'Does the organization have an information security policy?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'A foundational security policy demonstrates management commitment to information security.',
    '["ISO 27001 A.5.1", "NIST CSF ID.GV-1"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000052',
    'qs000000-0000-0000-0000-000000000011',
    'Is multi-factor authentication enforced for all remote access?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'MFA for remote access is considered a baseline security control.',
    '["ISO 27001 A.9.4.2", "NIST CSF PR.AC-7"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000053',
    'qs000000-0000-0000-0000-000000000011',
    'Are systems and software kept up to date with security patches?',
    'single_choice',
    '{"choices": [{"value": "automated", "label": "Yes, automated patching", "score": 100}, {"value": "manual_timely", "label": "Yes, manual but timely", "score": 70}, {"value": "irregular", "label": "Irregular patching", "score": 20}, {"value": "no", "label": "No patch management", "score": 0}]}'::jsonb,
    5,
    'critical',
    true,
    'Regular patching is one of the most effective controls against known vulnerabilities.',
    '["ISO 27001 A.12.6.1", "NIST CSF PR.IP-12"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000054',
    'qs000000-0000-0000-0000-000000000011',
    'Does the organization have endpoint protection (antivirus/EDR) on all devices?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Endpoint protection including antivirus or EDR solutions should be deployed on all endpoints to detect and prevent malware.',
    '["ISO 27001 A.12.2.1", "NIST CSF DE.CM-4"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000055',
    'qs000000-0000-0000-0000-000000000011',
    'Are regular backups performed and tested for restorability?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'Regular backups with verified restore capability are essential for business continuity and ransomware resilience.',
    '["ISO 27001 A.12.3.1", "NIST CSF PR.IP-4"]'::jsonb,
    5,
    NOW(),
    NOW()
);

-- Section 2: Data Protection Basics
INSERT INTO questionnaire_sections (id, questionnaire_template_id, title, description, sort_order, weight, created_at, updated_at)
VALUES (
    'qs000000-0000-0000-0000-000000000012',
    'q0000000-0000-0000-0000-000000000004',
    'Data Protection Basics',
    'Assess fundamental data protection measures.',
    2,
    50,
    NOW(),
    NOW()
);

INSERT INTO questionnaire_questions (id, section_id, question_text, question_type, options, weight, risk_impact, is_required, guidance_text, mapped_control_codes, sort_order, created_at, updated_at)
VALUES
(
    'qq000000-0000-0000-0000-000000000056',
    'qs000000-0000-0000-0000-000000000012',
    'Is all data encrypted in transit using TLS 1.2 or higher?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Encryption in transit protects data from interception. TLS 1.2 is the minimum acceptable standard.',
    '["ISO 27001 A.10.1.1", "NIST CSF PR.DS-2"]'::jsonb,
    1,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000057',
    'qs000000-0000-0000-0000-000000000012',
    'Is sensitive data encrypted at rest?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    5,
    'critical',
    true,
    'Encryption at rest protects data stored in databases, file systems, and backups from unauthorized access.',
    '["ISO 27001 A.10.1.1", "NIST CSF PR.DS-1"]'::jsonb,
    2,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000058',
    'qs000000-0000-0000-0000-000000000012',
    'Does the organization have an incident response plan?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    4,
    'high',
    true,
    'An incident response plan is essential for timely and effective response to security incidents.',
    '["ISO 27001 A.16.1.1", "NIST CSF RS.RP-1"]'::jsonb,
    3,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000059',
    'qs000000-0000-0000-0000-000000000012',
    'Are employees required to complete security awareness training?',
    'single_choice',
    '{"choices": [{"value": "annual_plus", "label": "Yes, annual with ongoing phishing simulations", "score": 100}, {"value": "annual", "label": "Yes, annual training only", "score": 70}, {"value": "onboarding", "label": "Only during onboarding", "score": 30}, {"value": "none", "label": "No training program", "score": 0}]}'::jsonb,
    3,
    'medium',
    true,
    'Security awareness training helps employees recognize and respond to security threats such as phishing, social engineering, and data handling.',
    '["ISO 27001 A.7.2.2", "NIST CSF PR.AT-1"]'::jsonb,
    4,
    NOW(),
    NOW()
),
(
    'qq000000-0000-0000-0000-000000000060',
    'qs000000-0000-0000-0000-000000000012',
    'Does the organization have a process for securely disposing of data and equipment?',
    'yes_no',
    '{"yes": {"score": 100, "label": "Yes"}, "no": {"score": 0, "label": "No"}}'::jsonb,
    3,
    'medium',
    true,
    'Secure disposal ensures that sensitive data cannot be recovered from decommissioned hardware or deleted storage.',
    '["ISO 27001 A.8.3.2", "NIST CSF PR.DS-3"]'::jsonb,
    5,
    NOW(),
    NOW()
);

COMMIT;
