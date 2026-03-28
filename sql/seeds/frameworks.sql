-- ComplianceForge: Seed data for common compliance frameworks.
-- Replace the placeholder organization_id with a real UUID after creating your organization.
-- Example: SET app.current_tenant = '<your-org-uuid>';

-- Placeholder organization ID (replace with actual value)
-- organization_id: '00000000-0000-0000-0000-000000000001'

INSERT INTO compliance_frameworks (organization_id, name, version, description, authority, website_url, status) VALUES
(
    '00000000-0000-0000-0000-000000000001',
    'ISO/IEC 27001:2022',
    '2022',
    'Information security, cybersecurity and privacy protection — Information security management systems — Requirements.',
    'International Organization for Standardization (ISO)',
    'https://www.iso.org/standard/27001',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'UK GDPR',
    '2018',
    'The UK General Data Protection Regulation, governing the processing of personal data of individuals in the United Kingdom.',
    'Information Commissioner''s Office (ICO)',
    'https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'NCSC CAF',
    '3.2',
    'The NCSC Cyber Assessment Framework provides systematic guidance for organisations responsible for vitally important services and activities.',
    'National Cyber Security Centre (NCSC)',
    'https://www.ncsc.gov.uk/collection/caf',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'Cyber Essentials',
    '3.1',
    'A UK Government-backed scheme to help organisations protect against the most common cyber attacks.',
    'National Cyber Security Centre (NCSC)',
    'https://www.ncsc.gov.uk/cyberessentials/overview',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'NIST SP 800-53 Rev 5',
    'Rev 5',
    'Security and Privacy Controls for Information Systems and Organizations.',
    'National Institute of Standards and Technology (NIST)',
    'https://csrc.nist.gov/publications/detail/sp/800-53/rev-5/final',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'NIST CSF 2.0',
    '2.0',
    'The NIST Cybersecurity Framework helps organisations manage and reduce cybersecurity risk.',
    'National Institute of Standards and Technology (NIST)',
    'https://www.nist.gov/cyberframework',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'PCI DSS v4.0',
    '4.0',
    'Payment Card Industry Data Security Standard — requirements for entities that store, process, or transmit cardholder data.',
    'PCI Security Standards Council',
    'https://www.pcisecuritystandards.org/document_library/',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'ITIL 4',
    '4',
    'IT Infrastructure Library — a set of practices for IT service management that focuses on aligning IT services with business needs.',
    'Axelos / PeopleCert',
    'https://www.axelos.com/certifications/itil-service-management',
    'active'
),
(
    '00000000-0000-0000-0000-000000000001',
    'COBIT 2019',
    '2019',
    'A framework for the governance and management of enterprise information and technology.',
    'ISACA',
    'https://www.isaca.org/resources/cobit',
    'active'
);
