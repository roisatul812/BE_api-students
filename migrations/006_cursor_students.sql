-- Migration 006: Index untuk keyset pagination pada entity students
-- Urutan column dan arah DESC pada index match persis dengan query:
-- ORDER BY created_at DESC, id DESC

CREATE INDEX IF NOT EXISTS idx_students_created_at_id_desc
ON students (created_at DESC, id DESC);
