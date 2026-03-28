-- Rollback Migration 003: Drop users, MFA, sessions, and password reset tables.

DROP TABLE IF EXISTS password_reset_tokens CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;

DROP TRIGGER IF EXISTS trg_user_mfa_updated_at ON user_mfa;
DROP TABLE IF EXISTS user_mfa CASCADE;

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP TABLE IF EXISTS users CASCADE;
