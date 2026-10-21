-- Additive: old task snapshots and existing prices remain immutable.
ALTER TABLE media_config_versions DROP CONSTRAINT IF EXISTS media_config_versions_kind_check;
ALTER TABLE media_config_versions ADD CONSTRAINT media_config_versions_kind_check
 CHECK(kind IN ('capability','price','template','binding'));
ALTER TABLE media_artifacts ADD COLUMN IF NOT EXISTS filename text NOT NULL DEFAULT '';
