DROP FUNCTION IF EXISTS incident_due_tenants(INTEGER);
DROP FUNCTION IF EXISTS severity_ord(TEXT);
DROP TABLE IF EXISTS incident_assignments;
DROP TABLE IF EXISTS incident_events;
DROP TABLE IF EXISTS incidents;
DROP TABLE IF EXISTS incident_reference_sequences;
DROP INDEX IF EXISTS uq_users_org_id_incident_fk;
