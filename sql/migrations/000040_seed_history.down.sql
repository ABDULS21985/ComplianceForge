-- Rollback Migration 040: compatibility boundary
--
-- The exception-trigger repair is intentionally retained: migrations 014 and
-- 030 now own and safely remove their respective functions during later downs.
-- bootstrap_seed_history is foundational bootstrap metadata owned by migration
-- 001. It is retained across ordinary application-schema rollbacks so applied
-- seed checksums are not lost, and migration 001 removes it at version zero.

SELECT 1;
