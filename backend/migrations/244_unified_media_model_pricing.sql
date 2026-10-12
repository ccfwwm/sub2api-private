-- Media prices live in the existing channel/group model pricing. Task snapshots remain immutable.
ALTER TABLE channel_model_pricing ADD COLUMN IF NOT EXISTS media JSONB;
