package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"carecompanion/internal/database"
)

func TestSessionCache_LookupValidRevokedMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewSessionCache(&database.Redis{Client: rdb})
	ctx := context.Background()

	sid := uuid.New()
	if got := cache.Lookup(ctx, sid); got != "miss" {
		t.Fatalf("Lookup empty = %q, want miss", got)
	}
	cache.MarkValid(ctx, sid)
	if got := cache.Lookup(ctx, sid); got != "valid" {
		t.Fatalf("Lookup after MarkValid = %q, want valid", got)
	}
	cache.MarkRevoked(ctx, sid)
	if got := cache.Lookup(ctx, sid); got != "revoked" {
		t.Fatalf("Lookup after MarkRevoked = %q, want revoked", got)
	}

	// Expiry: fast-forward miniredis past the valid TTL and confirm fadeout.
	sid2 := uuid.New()
	cache.MarkValid(ctx, sid2)
	mr.FastForward(2 * time.Minute)
	if got := cache.Lookup(ctx, sid2); got != "miss" {
		t.Fatalf("Lookup after TTL = %q, want miss", got)
	}
}
