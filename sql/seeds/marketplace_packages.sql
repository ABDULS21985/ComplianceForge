BEGIN;

-- Official ComplianceForge publisher
INSERT INTO marketplace_publishers (id, organization_id, publisher_name, publisher_slug, description, website, is_verified, is_official, verification_date, contact_email)
VALUES ('p0000000-0000-0000-0000-000000000001', NULL, 'ComplianceForge Official', 'complianceforge',
  'Official compliance packages curated by the ComplianceForge team. Quality-assured and regularly updated.',
  'https://complianceforge.io', true, true, '2026-01-01', 'marketplace@complianceforge.io');

-- Package 1: UK Financial Services Compliance Pack
INSERT INTO marketplace_packages (id, publisher_id, package_slug, name, description, long_description, package_type, category, applicable_frameworks, applicable_regions, applicable_industries, tags, current_version, pricing_model, price_eur, contents_summary, status, published_at, license)
VALUES ('pkg00000-0000-0000-0000-000000000001', 'p0000000-0000-0000-0000-000000000001',
  'uk-financial-services-pack', 'UK Financial Services Compliance Pack',
  'Comprehensive compliance package for UK-regulated financial services firms covering FCA, PRA, and Bank of England requirements.',
  '## UK Financial Services Compliance Pack\n\nDesigned for UK-regulated financial services firms including banks, investment firms, insurance companies, and payment institutions.\n\n### What''s Included\n- 25 additional controls specific to FCA/PRA requirements\n- Cross-mappings to ISO 27001, PCI DSS, and UK GDPR\n- 5 policy templates (AML, KYC, Fraud Prevention, Data Handling, Outsourcing)\n- Evidence collection templates for FCA audits\n\n### Regulatory Coverage\n- FCA SYSC (Systems and Controls)\n- PRA Operational Resilience\n- SM&CR (Senior Managers & Certification Regime)\n- UK Operational Resilience Framework',
  'compliance_playbook', 'financial_services', '{ISO27001,PCI_DSS_4,UK_GDPR}', '{UK}', '{financial_services}',
  '{FCA,PRA,banking,insurance,payments,AML,KYC}', '1.0.0', 'free', 0,
  '{"controls": 25, "mappings": 75, "policies": 5, "evidence_templates": 12}',
  'published', NOW(), 'CC-BY-4.0');

INSERT INTO marketplace_package_versions (id, package_id, version, release_notes, package_data, package_hash, file_size_bytes, published_at)
VALUES (gen_random_uuid(), 'pkg00000-0000-0000-0000-000000000001', '1.0.0', 'Initial release with 25 FCA/PRA controls, 75 cross-framework mappings, and 5 policy templates.',
  '{"controls": [{"code": "FCA-SYSC-3.1", "title": "Management responsibility for compliance", "description": "Senior management must take reasonable care to ensure the firm has adequate compliance arrangements.", "control_type": "directive", "implementation_type": "administrative"}], "mappings": [], "policies": [{"title": "Anti-Money Laundering Policy", "category": "COMPLIANCE"}]}',
  'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2', 45000, NOW());

-- Package 2: Healthcare GDPR Data Protection Pack
INSERT INTO marketplace_packages (id, publisher_id, package_slug, name, description, long_description, package_type, category, applicable_frameworks, applicable_regions, applicable_industries, tags, current_version, pricing_model, price_eur, contents_summary, status, published_at, license)
VALUES ('pkg00000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000001',
  'healthcare-gdpr-pack', 'Healthcare GDPR Data Protection Pack',
  'Specialised compliance package for healthcare organisations processing health data under GDPR Article 9.',
  '## Healthcare GDPR Data Protection Pack\n\nDesigned for hospitals, clinics, health insurers, pharma companies, and health tech providers processing special category health data.\n\n### What''s Included\n- 20 controls for health data processing per GDPR Article 9\n- Special category data handling procedures\n- DPIA template for health data processing\n- Patient consent management framework\n- Health data breach response procedures\n\n### Regulatory Coverage\n- GDPR Article 9 (Special Categories)\n- GDPR Article 35 (DPIA)\n- National health data regulations',
  'compliance_playbook', 'healthcare', '{UK_GDPR,ISO27001}', '{EU,UK}', '{healthcare}',
  '{GDPR,health_data,Article_9,DPIA,patient_data,special_category}', '1.0.0', 'free', 0,
  '{"controls": 20, "mappings": 40, "policies": 4, "evidence_templates": 8}',
  'published', NOW(), 'CC-BY-4.0');

INSERT INTO marketplace_package_versions (id, package_id, version, release_notes, package_data, package_hash, file_size_bytes, published_at)
VALUES (gen_random_uuid(), 'pkg00000-0000-0000-0000-000000000002', '1.0.0', 'Initial release with 20 healthcare-specific GDPR controls and 4 policy templates.',
  '{"controls": [{"code": "HC-GDPR-01", "title": "Lawful basis for health data processing", "description": "Establish and document explicit consent or other lawful basis per GDPR Article 9(2) for all health data processing activities.", "control_type": "directive", "implementation_type": "administrative"}], "mappings": [], "policies": [{"title": "Health Data Processing Policy", "category": "DATA_PRIVACY"}]}',
  'b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3', 38000, NOW());

-- Package 3: Cloud Security Controls Pack
INSERT INTO marketplace_packages (id, publisher_id, package_slug, name, description, long_description, package_type, category, applicable_frameworks, applicable_regions, applicable_industries, tags, current_version, pricing_model, price_eur, contents_summary, status, published_at, license)
VALUES ('pkg00000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000001',
  'cloud-security-controls', 'Cloud Security Controls Pack',
  'Comprehensive cloud security control library for organisations using AWS, Azure, and GCP cloud services.',
  '## Cloud Security Controls Pack\n\nFor organisations running workloads in public cloud (AWS, Azure, GCP) or hybrid environments.\n\n### What''s Included\n- 30 cloud-specific security controls\n- Mappings to ISO 27001, NIST 800-53, and CSA CCM\n- Cloud evidence collection templates\n- Cloud security architecture review checklist\n- Shared responsibility model documentation\n\n### Coverage\n- Identity and access management\n- Data encryption at rest and in transit\n- Network security and segmentation\n- Logging and monitoring\n- Incident response in cloud\n- Container and serverless security',
  'control_library', 'technology', '{ISO27001,NIST_800_53,NIST_CSF_2}', '{Global}', '{technology,all}',
  '{cloud,AWS,Azure,GCP,containers,serverless,IaC,DevSecOps}', '1.0.0', 'free', 0,
  '{"controls": 30, "mappings": 90, "policies": 3, "evidence_templates": 15}',
  'published', NOW(), 'CC-BY-4.0');

INSERT INTO marketplace_package_versions (id, package_id, version, release_notes, package_data, package_hash, file_size_bytes, published_at)
VALUES (gen_random_uuid(), 'pkg00000-0000-0000-0000-000000000003', '1.0.0', 'Initial release with 30 cloud security controls covering AWS, Azure, and GCP.',
  '{"controls": [{"code": "CLOUD-IAM-01", "title": "Cloud identity federation and SSO", "description": "Implement identity federation between corporate IdP and cloud providers using SAML 2.0 or OIDC.", "control_type": "preventive", "implementation_type": "technical"}], "mappings": [], "policies": [{"title": "Cloud Security Policy", "category": "INFO_SECURITY"}]}',
  'c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4', 52000, NOW());

COMMIT;
