-- Versioned public image/video API documentation, editable without rebuilding app containers.
CREATE TABLE IF NOT EXISTS public_documents (
    slug TEXT PRIMARY KEY,
    published_version BIGINT NOT NULL DEFAULT 0 CHECK (published_version >= 0),
    published_content JSONB NOT NULL DEFAULT '[]'::jsonb,
    draft_version BIGINT NOT NULL DEFAULT 0 CHECK (draft_version >= 0),
    draft_content JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS public_document_versions (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL REFERENCES public_documents(slug) ON DELETE CASCADE,
    version BIGINT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('initial', 'draft', 'publish', 'rollback')),
    content JSONB NOT NULL,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (slug, version, action)
);

CREATE INDEX IF NOT EXISTS public_document_versions_history ON public_document_versions(slug, created_at DESC, id DESC);
