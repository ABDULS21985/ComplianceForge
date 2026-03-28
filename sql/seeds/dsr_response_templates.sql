BEGIN;

INSERT INTO dsr_response_templates (id, organization_id, request_type, name, subject, body_html, body_text, is_system, language) VALUES

('f0000000-0001-0000-0000-000000000001', NULL, 'access', 'DSAR Response — Access Request',
 'Your Data Subject Access Request — Reference {{.RequestRef}}',
 '<p>Dear {{.DataSubjectName}},</p><p>Thank you for your data subject access request received on {{.ReceivedDate}} (Reference: <strong>{{.RequestRef}}</strong>).</p><p>In accordance with Article 15 of the UK GDPR / EU GDPR, please find attached a copy of the personal data we hold about you.</p><p>The data provided includes information from the following systems:</p><ul>{{range .Systems}}<li>{{.}}</li>{{end}}</ul><p>If you have any questions about the data provided, please contact our Data Protection Officer at {{.DPOEmail}}.</p><p>Yours sincerely,<br/>{{.OrganizationName}}<br/>Data Protection Team</p>',
 'Dear {{.DataSubjectName}},\n\nThank you for your data subject access request (Ref: {{.RequestRef}}).\n\nPlease find attached a copy of your personal data as required by GDPR Article 15.\n\nRegards,\n{{.OrganizationName}}',
 true, 'en'),

('f0000000-0001-0000-0000-000000000002', NULL, 'erasure', 'DSAR Response — Erasure Confirmation',
 'Confirmation of Data Erasure — Reference {{.RequestRef}}',
 '<p>Dear {{.DataSubjectName}},</p><p>Further to your erasure request received on {{.ReceivedDate}} (Reference: <strong>{{.RequestRef}}</strong>), we confirm that your personal data has been erased from our systems in accordance with Article 17 of the UK GDPR / EU GDPR.</p><p>The following actions have been taken:</p><ul>{{range .Actions}}<li>{{.}}</li>{{end}}</ul><p>We have also notified the following third parties who previously received your data:</p><ul>{{range .ThirdParties}}<li>{{.}}</li>{{end}}</ul><p>Please note that we may retain certain data where we have a legal obligation to do so (e.g., financial records for tax purposes).</p><p>Yours sincerely,<br/>{{.OrganizationName}}</p>',
 'Dear {{.DataSubjectName}},\n\nWe confirm your personal data has been erased per GDPR Article 17 (Ref: {{.RequestRef}}).\n\nRegards,\n{{.OrganizationName}}',
 true, 'en'),

('f0000000-0001-0000-0000-000000000003', NULL, 'rectification', 'DSAR Response — Rectification Confirmation',
 'Confirmation of Data Rectification — Reference {{.RequestRef}}',
 '<p>Dear {{.DataSubjectName}},</p><p>We confirm that your personal data has been corrected as requested (Reference: <strong>{{.RequestRef}}</strong>), in accordance with Article 16 of the UK GDPR / EU GDPR.</p><p>We have also notified relevant third parties of the correction per Article 19.</p><p>Yours sincerely,<br/>{{.OrganizationName}}</p>',
 'Dear {{.DataSubjectName}},\n\nYour data has been corrected per GDPR Article 16 (Ref: {{.RequestRef}}).\n\nRegards,\n{{.OrganizationName}}',
 true, 'en'),

('f0000000-0001-0000-0000-000000000004', NULL, 'portability', 'DSAR Response — Data Portability',
 'Your Data Portability Request — Reference {{.RequestRef}}',
 '<p>Dear {{.DataSubjectName}},</p><p>Please find attached your personal data in a structured, commonly used, and machine-readable format (JSON/CSV) as required by Article 20 of the UK GDPR / EU GDPR (Reference: <strong>{{.RequestRef}}</strong>).</p><p>Yours sincerely,<br/>{{.OrganizationName}}</p>',
 'Dear {{.DataSubjectName}},\n\nPlease find attached your data in machine-readable format per GDPR Article 20 (Ref: {{.RequestRef}}).\n\nRegards,\n{{.OrganizationName}}',
 true, 'en'),

('f0000000-0001-0000-0000-000000000005', NULL, 'access', 'DSAR Extension Notice',
 'Extension of Response Period — Reference {{.RequestRef}}',
 '<p>Dear {{.DataSubjectName}},</p><p>We are writing regarding your data subject request (Reference: <strong>{{.RequestRef}}</strong>) received on {{.ReceivedDate}}.</p><p>Due to the complexity of your request, we are exercising our right under Article 12(3) of the UK GDPR / EU GDPR to extend the response period by a further two months.</p><p><strong>Reason:</strong> {{.ExtensionReason}}</p><p><strong>New deadline:</strong> {{.ExtendedDeadline}}</p><p>We apologise for any inconvenience and will respond as soon as possible.</p><p>Yours sincerely,<br/>{{.OrganizationName}}</p>',
 'Extension notice for DSR {{.RequestRef}}.\nNew deadline: {{.ExtendedDeadline}}\nReason: {{.ExtensionReason}}',
 true, 'en'),

('f0000000-0001-0000-0000-000000000006', NULL, 'access', 'DSAR Rejection Notice',
 'Response to Your Data Request — Reference {{.RequestRef}}',
 '<p>Dear {{.DataSubjectName}},</p><p>We have reviewed your request (Reference: <strong>{{.RequestRef}}</strong>) and regret to inform you that we are unable to comply for the following reason:</p><p><strong>Legal basis for refusal:</strong> {{.RejectionLegalBasis}}</p><p><strong>Explanation:</strong> {{.RejectionReason}}</p><p>You have the right to lodge a complaint with the Information Commissioner''s Office (ICO) at ico.org.uk if you are dissatisfied with our response.</p><p>Yours sincerely,<br/>{{.OrganizationName}}</p>',
 'Your DSR {{.RequestRef}} has been declined.\nReason: {{.RejectionReason}}\nLegal basis: {{.RejectionLegalBasis}}\nYou may complain to the ICO.',
 true, 'en');

COMMIT;
