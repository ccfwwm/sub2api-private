-- Add display-only availability for media groups. Existing groups default to available.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS media_availability VARCHAR(20) NOT NULL DEFAULT 'available'
    CHECK (media_availability IN ('available', 'unavailable'));
