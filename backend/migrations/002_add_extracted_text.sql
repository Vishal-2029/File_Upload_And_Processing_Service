-- Add extracted_text column to store PDF text content for editing
ALTER TABLE files ADD COLUMN IF NOT EXISTS extracted_text TEXT NOT NULL DEFAULT '';
