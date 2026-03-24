-- Runs once on first container start.
-- Schema is managed by GORM AutoMigrate in development,
-- but this file handles PostgreSQL extensions and constraints.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
