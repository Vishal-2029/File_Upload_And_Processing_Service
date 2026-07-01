-- Runs once on first container start (only when the data volume is empty).
-- Schema is managed by GORM AutoMigrate in development.

SELECT 'CREATE DATABASE fileservice' WHERE NOT EXISTS (
  SELECT FROM pg_database WHERE datname = 'fileservice'
)\gexec

\c fileservice

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
