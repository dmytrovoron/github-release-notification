CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL,
    repository TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'active', 'unsubscribed')),
    confirm_token TEXT NOT NULL UNIQUE,
    unsubscribe_token TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS repository_states (
    repository TEXT PRIMARY KEY,
    last_seen_tag TEXT NOT NULL DEFAULT '',
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
