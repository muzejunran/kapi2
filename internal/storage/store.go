package storage

import "time"

// KVStore is the interface for key-value storage backends.
type KVStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	Delete(key string) error
}
