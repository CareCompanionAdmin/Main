package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/database"
)

const sessionCacheTTL = 60 * time.Second

// SessionCache stores per-sid validation results in Redis with a short TTL.
// Hot path lookups stay sub-ms; revocation invalidates immediately so a
// killed session can't ride a cached "valid" entry past the kill point.
type SessionCache struct{ r *database.Redis }

func NewSessionCache(r *database.Redis) *SessionCache { return &SessionCache{r: r} }

// Lookup returns "valid", "revoked", or "miss".
func (c *SessionCache) Lookup(ctx context.Context, sid uuid.UUID) string {
	val, err := c.r.Get(ctx, sessionKey(sid)).Result()
	if err != nil {
		return "miss"
	}
	if val == "valid" || val == "revoked" {
		return val
	}
	return "miss"
}

func (c *SessionCache) MarkValid(ctx context.Context, sid uuid.UUID) {
	_ = c.r.Set(ctx, sessionKey(sid), "valid", sessionCacheTTL).Err()
}

// MarkRevoked uses a longer TTL than valid entries so a revoked session can't
// fall out of cache and silently re-validate against a stale DB read.
func (c *SessionCache) MarkRevoked(ctx context.Context, sid uuid.UUID) {
	_ = c.r.Set(ctx, sessionKey(sid), "revoked", 5*time.Minute).Err()
}

func sessionKey(sid uuid.UUID) string { return "session:" + sid.String() }
