-- Migration 069 rollback: Remove protected column from images table
ALTER TABLE images DROP COLUMN IF EXISTS protected;
