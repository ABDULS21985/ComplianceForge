BEGIN;

-- The 10 NIS2 Article 21 minimum cybersecurity risk-management measures.
-- These are seeded as templates; the application creates org-specific copies during NIS2 onboarding.
-- organization_id is a placeholder — in production these are created per-org.

-- NOTE: This is reference data showing the measure definitions and their ISO 27001 mappings.
-- The actual nis2_security_measures records are created per-organization.

-- We document the measures and their ISO 27001 control mappings here:

/*
NIS2 Article 21 Measures → ISO 27001:2022 Control Mappings:

a) Risk analysis & IS security policies → A.5.1 (Policies), A.5.2 (Roles)
b) Incident handling → A.5.24 (Incident planning), A.5.25 (Assessment), A.5.26 (Response)
c) Business continuity, backup, DR, crisis management → A.5.29 (BC during disruption), A.5.30 (ICT readiness), A.8.13 (Backup)
d) Supply chain security → A.5.19 (Supplier relationships), A.5.20 (Supplier agreements), A.5.21 (ICT supply chain)
e) Security in acquisition, development, maintenance + vulnerability handling → A.8.8 (Vuln management), A.8.25 (Secure SDLC), A.8.9 (Config management)
f) Policies for assessing cybersecurity effectiveness → A.5.35 (Independent review), A.5.36 (Compliance review)
g) Cyber hygiene & training → A.6.3 (Awareness training), A.5.10 (Acceptable use)
h) Cryptography & encryption → A.8.24 (Cryptography)
i) HR security, access control, asset management → A.5.15 (Access control), A.5.16 (Identity), A.5.9 (Asset inventory), A.6.1 (Screening)
j) MFA, secured communications, emergency communications → A.8.5 (Secure authentication), A.8.20 (Network security), A.8.22 (Network segregation)
*/

-- Insert the 10 measures as a documentation/reference table
-- (Application will copy these to nis2_security_measures per-org during onboarding)

CREATE TABLE IF NOT EXISTS nis2_measure_definitions (
    measure_code VARCHAR(20) PRIMARY KEY,
    measure_title VARCHAR(500) NOT NULL,
    measure_description TEXT NOT NULL,
    article_reference VARCHAR(50) NOT NULL,
    iso27001_control_codes TEXT[] NOT NULL
);

INSERT INTO nis2_measure_definitions (measure_code, measure_title, measure_description, article_reference, iso27001_control_codes) VALUES
('NIS2-Art21-a', 'Policies on risk analysis and information system security',
 'Organisations must establish and maintain policies for risk analysis and information system security, including risk assessment methodologies and security governance frameworks.',
 'Article 21(2)(a)', '{A.5.1,A.5.2,A.5.7,A.5.31}'),

('NIS2-Art21-b', 'Incident handling',
 'Processes and procedures for preventing, detecting, and responding to cybersecurity incidents, including incident classification, escalation, and reporting procedures.',
 'Article 21(2)(b)', '{A.5.24,A.5.25,A.5.26,A.5.27,A.5.28,A.6.8}'),

('NIS2-Art21-c', 'Business continuity and crisis management',
 'Business continuity management including backup management, disaster recovery, and crisis management to ensure resilience of essential services.',
 'Article 21(2)(c)', '{A.5.29,A.5.30,A.8.13,A.8.14}'),

('NIS2-Art21-d', 'Supply chain security',
 'Security measures for relationships with direct suppliers and service providers, including security requirements in agreements and ongoing monitoring of supply chain risks.',
 'Article 21(2)(d)', '{A.5.19,A.5.20,A.5.21,A.5.22,A.5.23}'),

('NIS2-Art21-e', 'Security in network and information system acquisition, development and maintenance',
 'Security in the acquisition, development and maintenance of network and information systems, including vulnerability handling and disclosure.',
 'Article 21(2)(e)', '{A.8.8,A.8.9,A.8.25,A.8.26,A.8.27,A.8.28,A.8.29}'),

('NIS2-Art21-f', 'Policies and procedures for assessing effectiveness',
 'Policies and procedures to assess the effectiveness of cybersecurity risk-management measures, including regular testing, auditing, and review.',
 'Article 21(2)(f)', '{A.5.35,A.5.36,A.8.34}'),

('NIS2-Art21-g', 'Basic cyber hygiene practices and cybersecurity training',
 'Implementation of basic cyber hygiene practices and regular cybersecurity training for all staff and management bodies.',
 'Article 21(2)(g)', '{A.6.3,A.5.10,A.5.4}'),

('NIS2-Art21-h', 'Policies and procedures regarding the use of cryptography and encryption',
 'Policies governing the use of cryptographic controls and encryption to protect the confidentiality, authenticity and integrity of data.',
 'Article 21(2)(h)', '{A.8.24}'),

('NIS2-Art21-i', 'Human resources security, access control policies and asset management',
 'Human resources security measures, access control policies and procedures, and management of information assets.',
 'Article 21(2)(i)', '{A.5.9,A.5.10,A.5.15,A.5.16,A.5.17,A.5.18,A.6.1,A.6.2,A.6.5,A.8.2,A.8.3}'),

('NIS2-Art21-j', 'Use of multi-factor authentication, secured communications, and emergency communications',
 'Use of multi-factor authentication or continuous authentication solutions, secured voice, video and text communications, and secured emergency communication systems within the entity.',
 'Article 21(2)(j)', '{A.8.5,A.8.20,A.8.21,A.8.22}');

COMMIT;
