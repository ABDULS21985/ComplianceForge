-- Rollback Migration 043: durable asynchronous delivery and coordination

DROP TABLE IF EXISTS scheduler_leases;
DROP TABLE IF EXISTS queue_inbox;
DROP TABLE IF EXISTS queue_outbox;
