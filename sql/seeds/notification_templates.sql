BEGIN;

INSERT INTO notification_templates (id, organization_id, name, event_type, subject_template, body_html_template, body_text_template, in_app_title_template, in_app_body_template, variables, is_system) VALUES

-- GDPR Breach Alerts
('e0000000-0000-0000-0000-000000000001', NULL, 'GDPR Breach - 12 Hours Remaining', 'breach.deadline_approaching_12h',
 'URGENT: GDPR Breach Notification Deadline - 12 Hours Remaining — {{.IncidentRef}}',
 '<div style="background:#FEE2E2;border:2px solid #DC2626;padding:20px;border-radius:8px"><h2 style="color:#DC2626;margin:0">⚠️ GDPR 72-Hour Deadline Alert</h2><p style="margin:10px 0"><strong>Incident:</strong> {{.IncidentRef}} — {{.IncidentTitle}}</p><p><strong>Hours Remaining:</strong> {{.HoursRemaining}}</p><p><strong>Data Subjects Affected:</strong> {{.DataSubjectsAffected}}</p><p style="margin:10px 0">Under GDPR Article 33, you must notify the supervisory authority within 72 hours of becoming aware of a personal data breach.</p><a href="{{.IncidentURL}}" style="background:#DC2626;color:white;padding:10px 20px;border-radius:4px;text-decoration:none;display:inline-block">View Incident & Notify DPA</a></div>',
 'URGENT: GDPR Breach Notification Deadline - 12 Hours Remaining\n\nIncident: {{.IncidentRef}} — {{.IncidentTitle}}\nHours Remaining: {{.HoursRemaining}}\nData Subjects: {{.DataSubjectsAffected}}\n\nNotify DPA immediately: {{.IncidentURL}}',
 '🚨 GDPR Breach Alert — {{.HoursRemaining}}h remaining',
 'Incident {{.IncidentRef}}: {{.IncidentTitle}}. {{.DataSubjectsAffected}} data subjects affected. Notify DPA before deadline.',
 '{IncidentRef,IncidentTitle,HoursRemaining,DataSubjectsAffected,IncidentURL,DeadlineTime}', true),

('e0000000-0000-0000-0000-000000000002', NULL, 'GDPR Breach - 6 Hours Remaining', 'breach.deadline_approaching_6h',
 'CRITICAL: GDPR Breach Deadline - Only 6 Hours Remaining — {{.IncidentRef}}',
 '<div style="background:#FEE2E2;border:3px solid #DC2626;padding:20px;border-radius:8px"><h2 style="color:#DC2626">🚨 CRITICAL: 6 Hours Until GDPR Deadline</h2><p><strong>{{.IncidentRef}}</strong> — {{.IncidentTitle}}</p><p><strong>Deadline:</strong> {{.DeadlineTime}}</p><p><strong>Data Subjects:</strong> {{.DataSubjectsAffected}}</p><a href="{{.IncidentURL}}" style="background:#DC2626;color:white;padding:12px 24px;border-radius:4px;text-decoration:none;display:inline-block;font-weight:bold">NOTIFY DPA NOW</a></div>',
 'CRITICAL: Only 6 hours until GDPR breach notification deadline!\nIncident: {{.IncidentRef}} — {{.IncidentTitle}}\nDeadline: {{.DeadlineTime}}\nNotify DPA NOW: {{.IncidentURL}}',
 '🚨 CRITICAL: 6h to GDPR deadline — {{.IncidentRef}}',
 '{{.IncidentTitle}} — notify DPA before {{.DeadlineTime}}. {{.DataSubjectsAffected}} subjects affected.',
 '{IncidentRef,IncidentTitle,HoursRemaining,DataSubjectsAffected,IncidentURL,DeadlineTime}', true),

('e0000000-0000-0000-0000-000000000003', NULL, 'GDPR Breach - 1 Hour Remaining', 'breach.deadline_approaching_1h',
 'FINAL WARNING: GDPR Breach Deadline in 1 Hour — {{.IncidentRef}}',
 '<div style="background:#DC2626;color:white;padding:20px;border-radius:8px"><h2>🚨 FINAL WARNING: 1 HOUR TO GDPR DEADLINE</h2><p><strong>{{.IncidentRef}}</strong> — {{.IncidentTitle}}</p><p><strong>Deadline: {{.DeadlineTime}}</strong></p><p>Failure to notify the supervisory authority may result in fines up to €10M or 2% of annual turnover.</p><a href="{{.IncidentURL}}" style="background:white;color:#DC2626;padding:12px 24px;border-radius:4px;text-decoration:none;display:inline-block;font-weight:bold">NOTIFY DPA IMMEDIATELY</a></div>',
 'FINAL WARNING: 1 hour to GDPR breach notification deadline!\n{{.IncidentRef}} — {{.IncidentTitle}}\nDeadline: {{.DeadlineTime}}\nACT NOW: {{.IncidentURL}}',
 '🚨 FINAL: 1h to GDPR deadline — {{.IncidentRef}}',
 'IMMEDIATE ACTION REQUIRED. Notify DPA before {{.DeadlineTime}}.',
 '{IncidentRef,IncidentTitle,HoursRemaining,DataSubjectsAffected,IncidentURL,DeadlineTime}', true),

('e0000000-0000-0000-0000-000000000004', NULL, 'GDPR Breach Deadline Expired', 'breach.deadline_expired',
 'OVERDUE: GDPR Breach Notification Deadline Has Passed — {{.IncidentRef}}',
 '<div style="background:#7F1D1D;color:white;padding:20px;border-radius:8px"><h2>⛔ GDPR DEADLINE EXPIRED</h2><p>The 72-hour notification deadline for <strong>{{.IncidentRef}}</strong> has passed.</p><p>Notify the DPA immediately and document the reason for the delay as required by GDPR Article 33(1).</p><a href="{{.IncidentURL}}" style="background:white;color:#7F1D1D;padding:12px 24px;border-radius:4px;text-decoration:none;display:inline-block;font-weight:bold">Record Late Notification</a></div>',
 'OVERDUE: GDPR 72-hour deadline has expired for {{.IncidentRef}}.\nNotify DPA immediately and document the delay reason.\n{{.IncidentURL}}',
 '⛔ GDPR deadline EXPIRED — {{.IncidentRef}}',
 'The 72-hour notification deadline has passed. Notify DPA immediately.',
 '{IncidentRef,IncidentTitle,DataSubjectsAffected,IncidentURL,DeadlineTime}', true),

-- NIS2 Alert
('e0000000-0000-0000-0000-000000000005', NULL, 'NIS2 Early Warning Due', 'nis2.early_warning_due',
 'NIS2: Early Warning Required Within 24 Hours — {{.IncidentRef}}',
 '<div style="background:#FEF3C7;border:2px solid #F59E0B;padding:20px;border-radius:8px"><h2 style="color:#92400E">NIS2 Early Warning Required</h2><p>Incident <strong>{{.IncidentRef}}</strong> requires an early warning to the CSIRT within 24 hours under NIS2 Article 23.</p><p><strong>Deadline:</strong> {{.DeadlineTime}}</p><a href="{{.IncidentURL}}" style="background:#F59E0B;color:white;padding:10px 20px;border-radius:4px;text-decoration:none">Submit Early Warning</a></div>',
 'NIS2: Early warning required within 24 hours for {{.IncidentRef}}.\nDeadline: {{.DeadlineTime}}\nSubmit: {{.IncidentURL}}',
 'NIS2: Early warning due — {{.IncidentRef}}',
 'Submit early warning to CSIRT within 24 hours. Deadline: {{.DeadlineTime}}.',
 '{IncidentRef,IncidentTitle,DeadlineTime,IncidentURL}', true),

-- Incident Created
('e0000000-0000-0000-0000-000000000006', NULL, 'Incident Created', 'incident.created',
 'New {{.Severity}} Incident Reported — {{.IncidentRef}}',
 '<h2>New Incident Reported</h2><p><strong>Reference:</strong> {{.IncidentRef}}</p><p><strong>Title:</strong> {{.IncidentTitle}}</p><p><strong>Severity:</strong> <span style="color:{{.SeverityColor}}">{{.Severity}}</span></p><p><strong>Reported by:</strong> {{.ReporterName}}</p><a href="{{.IncidentURL}}" style="background:#4F46E5;color:white;padding:10px 20px;border-radius:4px;text-decoration:none">View Incident</a>',
 'New {{.Severity}} incident: {{.IncidentRef}} — {{.IncidentTitle}}\nReported by: {{.ReporterName}}\nView: {{.IncidentURL}}',
 'New {{.Severity}} incident — {{.IncidentRef}}',
 '{{.IncidentTitle}} reported by {{.ReporterName}}.',
 '{IncidentRef,IncidentTitle,Severity,SeverityColor,ReporterName,IncidentURL}', true),

-- Control Status Changed
('e0000000-0000-0000-0000-000000000007', NULL, 'Control Status Changed', 'control.status_changed',
 'Control Status Updated — {{.ControlCode}}: {{.OldStatus}} → {{.NewStatus}}',
 '<h2>Control Implementation Status Changed</h2><p><strong>Control:</strong> {{.ControlCode}} — {{.ControlTitle}}</p><p><strong>Framework:</strong> {{.FrameworkName}}</p><p><strong>Status:</strong> {{.OldStatus}} → <strong>{{.NewStatus}}</strong></p><p><strong>Changed by:</strong> {{.ChangedBy}}</p><a href="{{.ControlURL}}">View Control</a>',
 'Control {{.ControlCode}} status changed: {{.OldStatus}} → {{.NewStatus}}\n{{.ControlTitle}} ({{.FrameworkName}})\nChanged by: {{.ChangedBy}}',
 'Control {{.ControlCode}}: {{.NewStatus}}',
 '{{.ControlTitle}} changed from {{.OldStatus}} to {{.NewStatus}}.',
 '{ControlCode,ControlTitle,FrameworkName,OldStatus,NewStatus,ChangedBy,ControlURL}', true),

-- Policy Review Due
('e0000000-0000-0000-0000-000000000008', NULL, 'Policy Review Due', 'policy.review_due',
 'Policy Review Due — {{.PolicyRef}}: {{.PolicyTitle}}',
 '<h2>Policy Review Due</h2><p>Policy <strong>{{.PolicyRef}}</strong> — {{.PolicyTitle}} is due for review.</p><p><strong>Review Deadline:</strong> {{.ReviewDate}}</p><p><strong>Owner:</strong> {{.OwnerName}}</p><a href="{{.PolicyURL}}">Review Policy</a>',
 'Policy {{.PolicyRef}} — {{.PolicyTitle}} is due for review by {{.ReviewDate}}.\nOwner: {{.OwnerName}}\nReview: {{.PolicyURL}}',
 'Policy review due — {{.PolicyRef}}',
 '{{.PolicyTitle}} needs review by {{.ReviewDate}}.',
 '{PolicyRef,PolicyTitle,ReviewDate,OwnerName,PolicyURL}', true),

-- Policy Review Overdue
('e0000000-0000-0000-0000-000000000009', NULL, 'Policy Review Overdue', 'policy.review_overdue',
 'OVERDUE: Policy Review — {{.PolicyRef}}: {{.PolicyTitle}}',
 '<div style="background:#FEF2F2;border:1px solid #DC2626;padding:16px;border-radius:8px"><h2 style="color:#DC2626">Policy Review Overdue</h2><p><strong>{{.PolicyRef}}</strong> — {{.PolicyTitle}}</p><p>This policy was due for review on <strong>{{.ReviewDate}}</strong> and is now {{.DaysOverdue}} days overdue.</p><a href="{{.PolicyURL}}" style="background:#DC2626;color:white;padding:10px 20px;border-radius:4px;text-decoration:none">Review Now</a></div>',
 'OVERDUE: Policy {{.PolicyRef}} was due for review on {{.ReviewDate}} ({{.DaysOverdue}} days ago).\n{{.PolicyURL}}',
 '⚠️ Policy overdue — {{.PolicyRef}}',
 '{{.PolicyTitle}} is {{.DaysOverdue}} days overdue for review.',
 '{PolicyRef,PolicyTitle,ReviewDate,DaysOverdue,OwnerName,PolicyURL}', true),

-- Attestation Required
('e0000000-0000-0000-0000-000000000010', NULL, 'Policy Attestation Required', 'policy.attestation_required',
 'Action Required: Please Acknowledge Policy — {{.PolicyTitle}}',
 '<h2>Policy Acknowledgement Required</h2><p>You are required to read and acknowledge the following policy:</p><p><strong>{{.PolicyRef}}</strong> — {{.PolicyTitle}}</p><p><strong>Deadline:</strong> {{.DueDate}}</p><a href="{{.PolicyURL}}" style="background:#4F46E5;color:white;padding:10px 20px;border-radius:4px;text-decoration:none">Read & Acknowledge</a>',
 'Please read and acknowledge policy {{.PolicyRef}} — {{.PolicyTitle}} by {{.DueDate}}.\n{{.PolicyURL}}',
 'Acknowledge policy — {{.PolicyRef}}',
 'Please read and acknowledge {{.PolicyTitle}} by {{.DueDate}}.',
 '{PolicyRef,PolicyTitle,DueDate,PolicyURL}', true),

-- Audit Finding Created
('e0000000-0000-0000-0000-000000000011', NULL, 'Audit Finding Created', 'finding.created',
 'New {{.Severity}} Audit Finding — {{.FindingRef}}',
 '<h2>New Audit Finding</h2><p><strong>{{.FindingRef}}</strong> — {{.FindingTitle}}</p><p><strong>Audit:</strong> {{.AuditRef}} — {{.AuditTitle}}</p><p><strong>Severity:</strong> {{.Severity}}</p><p><strong>Due Date:</strong> {{.DueDate}}</p><a href="{{.FindingURL}}">View Finding</a>',
 'New {{.Severity}} finding: {{.FindingRef}} — {{.FindingTitle}}\nAudit: {{.AuditTitle}}\nDue: {{.DueDate}}',
 'New {{.Severity}} finding — {{.FindingRef}}',
 '{{.FindingTitle}} from audit {{.AuditRef}}. Due: {{.DueDate}}.',
 '{FindingRef,FindingTitle,AuditRef,AuditTitle,Severity,DueDate,FindingURL}', true),

-- Finding Overdue
('e0000000-0000-0000-0000-000000000012', NULL, 'Finding Remediation Overdue', 'finding.overdue',
 'OVERDUE: Audit Finding Remediation — {{.FindingRef}}',
 '<div style="background:#FEF2F2;border:1px solid #DC2626;padding:16px;border-radius:8px"><h2 style="color:#DC2626">Finding Remediation Overdue</h2><p><strong>{{.FindingRef}}</strong> — {{.FindingTitle}}</p><p>Due date was <strong>{{.DueDate}}</strong> ({{.DaysOverdue}} days ago).</p><a href="{{.FindingURL}}">View Finding</a></div>',
 'OVERDUE: Finding {{.FindingRef}} remediation was due {{.DueDate}} ({{.DaysOverdue}} days ago).',
 '⚠️ Finding overdue — {{.FindingRef}}',
 '{{.FindingTitle}} is {{.DaysOverdue}} days past remediation deadline.',
 '{FindingRef,FindingTitle,DueDate,DaysOverdue,FindingURL}', true),

-- Vendor Assessment Due
('e0000000-0000-0000-0000-000000000013', NULL, 'Vendor Assessment Due', 'vendor.assessment_due',
 'Vendor Risk Assessment Due — {{.VendorName}}',
 '<h2>Vendor Assessment Due</h2><p>Vendor <strong>{{.VendorName}}</strong> is due for a risk assessment.</p><p><strong>Risk Tier:</strong> {{.RiskTier}}</p><p><strong>Due Date:</strong> {{.DueDate}}</p><a href="{{.VendorURL}}">Start Assessment</a>',
 'Vendor {{.VendorName}} risk assessment due by {{.DueDate}}.\nRisk Tier: {{.RiskTier}}\n{{.VendorURL}}',
 'Vendor assessment due — {{.VendorName}}',
 '{{.VendorName}} ({{.RiskTier}} risk) needs assessment by {{.DueDate}}.',
 '{VendorName,RiskTier,DueDate,VendorURL}', true),

-- Risk Threshold Exceeded
('e0000000-0000-0000-0000-000000000014', NULL, 'Risk Threshold Exceeded', 'risk.threshold_exceeded',
 'Risk Alert: {{.RiskLevel}} Risk Detected — {{.RiskRef}}',
 '<h2>Risk Threshold Exceeded</h2><p><strong>{{.RiskRef}}</strong> — {{.RiskTitle}}</p><p>Residual risk score <strong>{{.RiskScore}}</strong> exceeds the {{.RiskLevel}} threshold for category {{.CategoryName}}.</p><a href="{{.RiskURL}}">View Risk</a>',
 'Risk {{.RiskRef}} ({{.RiskTitle}}) has exceeded {{.RiskLevel}} threshold with score {{.RiskScore}}.',
 '⚠️ Risk threshold exceeded — {{.RiskRef}}',
 '{{.RiskTitle}} — residual score {{.RiskScore}} exceeds {{.RiskLevel}} threshold.',
 '{RiskRef,RiskTitle,RiskScore,RiskLevel,CategoryName,RiskURL}', true),

-- Compliance Score Dropped
('e0000000-0000-0000-0000-000000000015', NULL, 'Compliance Score Dropped', 'compliance.score_dropped',
 'Compliance Alert: Score Dropped Below Threshold — {{.FrameworkName}}',
 '<h2>Compliance Score Alert</h2><p>The compliance score for <strong>{{.FrameworkName}}</strong> has dropped from {{.PreviousScore}}% to <strong>{{.CurrentScore}}%</strong>, falling below the {{.Threshold}}% threshold.</p><a href="{{.FrameworkURL}}">View Framework</a>',
 'Compliance score for {{.FrameworkName}} dropped from {{.PreviousScore}}% to {{.CurrentScore}}% (threshold: {{.Threshold}}%).',
 '📉 Score dropped — {{.FrameworkName}}',
 '{{.FrameworkName}} score dropped to {{.CurrentScore}}% (was {{.PreviousScore}}%).',
 '{FrameworkName,PreviousScore,CurrentScore,Threshold,FrameworkURL}', true),

-- DSR Received
('e0000000-0000-0000-0000-000000000016', NULL, 'DSR Request Received', 'dsr.received',
 'New Data Subject Request Received — {{.RequestRef}}',
 '<h2>New Data Subject Request</h2><p><strong>Reference:</strong> {{.RequestRef}}</p><p><strong>Type:</strong> {{.RequestType}}</p><p><strong>Received:</strong> {{.ReceivedDate}}</p><p><strong>Response Deadline:</strong> {{.Deadline}}</p><a href="{{.DSRURL}}">Process Request</a>',
 'New {{.RequestType}} DSR received: {{.RequestRef}}\nDeadline: {{.Deadline}}\nProcess: {{.DSRURL}}',
 'New DSR — {{.RequestRef}}',
 '{{.RequestType}} request received. Deadline: {{.Deadline}}.',
 '{RequestRef,RequestType,ReceivedDate,Deadline,DSRURL}', true),

-- Welcome Email
('e0000000-0000-0000-0000-000000000017', NULL, 'Welcome Email', 'user.welcome',
 'Welcome to ComplianceForge — {{.OrganizationName}}',
 '<h2>Welcome to ComplianceForge</h2><p>Hello {{.FirstName}},</p><p>Your account has been created for <strong>{{.OrganizationName}}</strong>.</p><p><strong>Email:</strong> {{.Email}}</p><p>Please log in and change your password.</p><a href="{{.LoginURL}}" style="background:#4F46E5;color:white;padding:10px 20px;border-radius:4px;text-decoration:none">Log In</a>',
 'Welcome to ComplianceForge, {{.FirstName}}!\nYour account for {{.OrganizationName}} is ready.\nLog in: {{.LoginURL}}',
 'Welcome to ComplianceForge',
 'Your account is ready. Log in to get started.',
 '{FirstName,LastName,Email,OrganizationName,LoginURL}', true),

-- Password Reset
('e0000000-0000-0000-0000-000000000018', NULL, 'Password Reset', 'user.password_reset',
 'Password Reset Request — ComplianceForge',
 '<h2>Password Reset</h2><p>Hello {{.FirstName}},</p><p>We received a request to reset your password. Click the button below to set a new password.</p><a href="{{.ResetURL}}" style="background:#4F46E5;color:white;padding:10px 20px;border-radius:4px;text-decoration:none">Reset Password</a><p style="color:#6B7280;font-size:12px;margin-top:16px">This link expires in {{.ExpiryHours}} hours. If you did not request this, please ignore this email.</p>',
 'Password reset for ComplianceForge.\nReset: {{.ResetURL}}\nExpires in {{.ExpiryHours}} hours.',
 'Password reset requested',
 'Click to reset your password. Link expires in {{.ExpiryHours}} hours.',
 '{FirstName,Email,ResetURL,ExpiryHours}', true);

COMMIT;
