package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"carecompanion/internal/config"
)

type Redis struct {
	*redis.Client
}

func NewRedis(cfg *config.RedisConfig) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Redis{client}, nil
}

func (r *Redis) Close() error {
	return r.Client.Close()
}

func (r *Redis) SetSession(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.Set(ctx, "session:"+key, value, expiration).Err()
}

func (r *Redis) GetSession(ctx context.Context, key string) (string, error) {
	return r.Get(ctx, "session:"+key).Result()
}

func (r *Redis) DeleteSession(ctx context.Context, key string) error {
	return r.Del(ctx, "session:"+key).Err()
}
