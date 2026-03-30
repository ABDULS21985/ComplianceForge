-- Seed: Default data classification levels and data categories
-- ComplianceForge GRC Platform — Batch 6
--
-- Provides 5 classification levels (Public through Top Secret) and 30+ data
-- categories covering personal data, GDPR special categories, financial data,
-- employment data, and technical data. All rows are system-level (organization_id
-- IS NULL) so they serve as templates copied into each tenant on onboarding.

BEGIN;

-- ============================================================================
-- DATA CLASSIFICATION LEVELS
-- ============================================================================

INSERT INTO data_classifications (id, organization_id, name, level, description, handling_requirements, encryption_required, access_restriction_required, data_masking_required, disposal_method, color_hex, is_system, sort_order) VALUES
(gen_random_uuid(), NULL, 'Public', 0, 'Information intended for public disclosure. No restrictions on access or distribution.', 'No special handling required.', false, false, false, 'standard_delete', '#22C55E', true, 0),
(gen_random_uuid(), NULL, 'Internal', 1, 'Information for internal use only. Not intended for external distribution.', 'Share only within the organisation. Use standard access controls.', false, true, false, 'standard_delete', '#3B82F6', true, 1),
(gen_random_uuid(), NULL, 'Confidential', 2, 'Sensitive information requiring protection. Unauthorised disclosure could cause harm.', 'Encryption required for storage and transmission. Access restricted to authorised personnel.', true, true, false, 'secure_wipe', '#F59E0B', true, 2),
(gen_random_uuid(), NULL, 'Restricted', 3, 'Highly sensitive information. Unauthorised access could cause significant harm.', 'Strong encryption required. Need-to-know access only. Audit all access.', true, true, true, 'secure_wipe', '#EF4444', true, 3),
(gen_random_uuid(), NULL, 'Top Secret', 4, 'Most sensitive information. Unauthorised disclosure could cause severe damage.', 'Highest level encryption. Physical security controls. Multi-person access required.', true, true, true, 'physical_destruction', '#7F1D1D', true, 4);

-- ============================================================================
-- DATA CATEGORIES — Personal Data
-- ============================================================================

INSERT INTO data_categories (id, organization_id, name, category_type, gdpr_special_category, description, examples, is_system) VALUES
(gen_random_uuid(), NULL, 'Full Name', 'personal_data', false, 'Individual''s full name including first name, last name, and middle names.', '{first name,last name,maiden name,middle name}', true),
(gen_random_uuid(), NULL, 'Email Address', 'personal_data', false, 'Personal or work email addresses.', '{personal email,work email}', true),
(gen_random_uuid(), NULL, 'Phone Number', 'personal_data', false, 'Mobile, landline, or work phone numbers.', '{mobile number,landline,work phone}', true),
(gen_random_uuid(), NULL, 'Postal Address', 'personal_data', false, 'Physical mailing address including street, city, postcode, country.', '{home address,work address,billing address}', true),
(gen_random_uuid(), NULL, 'Date of Birth', 'personal_data', false, 'Individual''s date of birth.', '{date of birth,age}', true),
(gen_random_uuid(), NULL, 'National ID Number', 'personal_data', false, 'Government-issued identification numbers.', '{national insurance number,social security number,personal ID}', true),
(gen_random_uuid(), NULL, 'Passport Number', 'personal_data', false, 'Passport identification number.', '{passport number,travel document number}', true),
(gen_random_uuid(), NULL, 'IP Address', 'personal_data', false, 'Internet Protocol address of devices used by individuals.', '{IPv4 address,IPv6 address}', true),
(gen_random_uuid(), NULL, 'Cookie Data', 'personal_data', false, 'Browser cookies and tracking identifiers.', '{session cookies,tracking cookies,advertising ID}', true),
(gen_random_uuid(), NULL, 'Location Data', 'personal_data', false, 'Geographic location information derived from devices or records.', '{GPS coordinates,cell tower data,WiFi triangulation,check-in data}', true),
(gen_random_uuid(), NULL, 'Photograph', 'personal_data', false, 'Photographs of identifiable individuals.', '{profile photo,ID photo,CCTV image}', true),
(gen_random_uuid(), NULL, 'Online Identifier', 'personal_data', false, 'Usernames, account IDs, and other online identifiers.', '{username,account ID,social media handle}', true),

-- ============================================================================
-- DATA CATEGORIES — GDPR Special Categories (Article 9)
-- ============================================================================

(gen_random_uuid(), NULL, 'Health Data', 'special_category', true, 'Data concerning physical or mental health, including healthcare service provision.', '{medical records,prescriptions,health insurance,disability status}', true),
(gen_random_uuid(), NULL, 'Genetic Data', 'special_category', true, 'Personal data relating to inherited or acquired genetic characteristics.', '{DNA profile,genome data,genetic test results}', true),
(gen_random_uuid(), NULL, 'Biometric Data', 'special_category', true, 'Data resulting from specific technical processing relating to physical, physiological or behavioural characteristics for identification.', '{fingerprint,facial recognition,iris scan,voice print}', true),
(gen_random_uuid(), NULL, 'Racial or Ethnic Origin', 'special_category', true, 'Data revealing racial or ethnic origin.', '{race,ethnicity,national origin}', true),
(gen_random_uuid(), NULL, 'Political Opinions', 'special_category', true, 'Data revealing political opinions or affiliations.', '{political party membership,voting record,political donations}', true),
(gen_random_uuid(), NULL, 'Religious or Philosophical Beliefs', 'special_category', true, 'Data revealing religious or philosophical beliefs.', '{religion,denomination,philosophical beliefs}', true),
(gen_random_uuid(), NULL, 'Trade Union Membership', 'special_category', true, 'Data revealing trade union membership.', '{union membership,union activities}', true),
(gen_random_uuid(), NULL, 'Sexual Orientation', 'special_category', true, 'Data concerning sexual orientation or sex life.', '{sexual orientation,relationship status}', true),
(gen_random_uuid(), NULL, 'Criminal Records', 'special_category', true, 'Data relating to criminal convictions and offences.', '{criminal conviction,caution,arrest record,DBS check}', true),

-- ============================================================================
-- DATA CATEGORIES — Financial Data
-- ============================================================================

(gen_random_uuid(), NULL, 'Bank Account Details', 'financial', false, 'Banking and payment account information.', '{bank account number,sort code,IBAN,SWIFT/BIC}', true),
(gen_random_uuid(), NULL, 'Credit/Debit Card Number', 'financial', false, 'Payment card information subject to PCI DSS.', '{card number,CVV,expiry date,cardholder name}', true),
(gen_random_uuid(), NULL, 'Salary Information', 'financial', false, 'Employee compensation and payroll data.', '{salary,bonus,benefits,tax code}', true),
(gen_random_uuid(), NULL, 'Credit Score', 'financial', false, 'Credit rating and credit history information.', '{credit score,credit history,credit report}', true),
(gen_random_uuid(), NULL, 'Tax Records', 'financial', false, 'Tax identification and filing information.', '{tax ID,tax returns,VAT number}', true),

-- ============================================================================
-- DATA CATEGORIES — Employment Data
-- ============================================================================

(gen_random_uuid(), NULL, 'Employment History', 'personal_data', false, 'Past and current employment records.', '{employer name,job title,dates of employment,reason for leaving}', true),
(gen_random_uuid(), NULL, 'Performance Reviews', 'personal_data', false, 'Employee performance evaluation records.', '{appraisal scores,objectives,feedback,development plans}', true),
(gen_random_uuid(), NULL, 'Disciplinary Records', 'personal_data', false, 'Employee disciplinary and grievance records.', '{warnings,disciplinary outcomes,grievance records}', true),
(gen_random_uuid(), NULL, 'Training Records', 'personal_data', false, 'Employee training and certification records.', '{courses completed,certifications,CPD records}', true),

-- ============================================================================
-- DATA CATEGORIES — Technical Data
-- ============================================================================

(gen_random_uuid(), NULL, 'System Access Logs', 'technical', false, 'Records of user access to IT systems.', '{login timestamps,access logs,audit trails}', true),
(gen_random_uuid(), NULL, 'Device Information', 'technical', false, 'Information about devices used.', '{device ID,MAC address,IMEI,operating system}', true);

COMMIT;
