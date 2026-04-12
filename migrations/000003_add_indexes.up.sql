-- Index for ExistsActiveOrPending: WHERE email = $1 AND repository = $2 AND status IN ('pending', 'active')
CREATE INDEX IF NOT EXISTS idx_subscriptions_email_repository ON subscriptions (email, repository);

-- Index for ListActiveByEmail: WHERE email = $1 AND status = 'active' ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_subscriptions_email_status_created_at ON subscriptions (email, status, created_at DESC);

-- Index for ListActive and ListPendingNotifications: WHERE status = 'active' with JOIN ON repository
CREATE INDEX IF NOT EXISTS idx_subscriptions_status_repository ON subscriptions (status, repository);
