package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Storage defines the storage interface
type Storage interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	Delete(key string) error
	Exists(key string) (bool, error)
}

// RedisStorage implements Storage using Redis
type RedisStorage struct {
	client *redis.Client
}

// NewRedisStorage creates a new Redis storage
func NewRedisStorage(addr string) *RedisStorage {
	return &RedisStorage{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

// Get retrieves a value from Redis
func (r *RedisStorage) Get(key string) ([]byte, error) {
	ctx := context.Background()
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, err
	}
	return []byte(val), nil
}

// Set stores a value in Redis with TTL
func (r *RedisStorage) Set(key string, value []byte, ttl time.Duration) error {
	ctx := context.Background()
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes a value from Redis
func (r *RedisStorage) Delete(key string) error {
	ctx := context.Background()
	return r.client.Del(ctx, key).Err()
}

// Exists checks if a key exists in Redis
func (r *RedisStorage) Exists(key string) (bool, error) {
	ctx := context.Background()
	result, err := r.client.Exists(ctx, key).Result()
	return result > 0, err
}