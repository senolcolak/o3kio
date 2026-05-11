-- Migration 069: Add protected column to images table
ALTER TABLE images ADD COLUMN IF NOT EXISTS protected INTEGER NOT NULL DEFAULT 0;
