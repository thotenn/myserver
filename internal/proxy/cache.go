package proxy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// Cache provides a TTL-based cache for proxy responses.
type Cache struct {
	cache *ttlcache.Cache[string, []byte]
}

// NewCache creates a new proxy cache with the default TTL.
func NewCache(defaultTTL time.Duration) *Cache {
	c := ttlcache.New[string, []byte](
		ttlcache.WithTTL[string, []byte](defaultTTL),
		ttlcache.WithCapacity[string, []byte](500),
	)
	go c.Start()

	return &Cache{cache: c}
}

// Get retrieves a cached response.
func (c *Cache) Get(key string) ([]byte, bool) {
	item := c.cache.Get(key)
	if item == nil || item.IsExpired() {
		return nil, false
	}
	return item.Value(), true
}

// Set stores a response in the cache with the given TTL.
func (c *Cache) Set(key string, value []byte, ttl time.Duration) {
	c.cache.Set(key, value, ttl)
}

// Delete removes a cached response.
func (c *Cache) Delete(key string) {
	c.cache.Delete(key)
}

// DeleteAll removes all cached responses.
func (c *Cache) DeleteAll() {
	c.cache.DeleteAll()
}

// Stop stops the cache eviction goroutine.
func (c *Cache) Stop() {
	c.cache.Stop()
}

// CacheKey generates a cache key from proxy parameters.
func CacheKey(name, group, service string, index int) string {
	return fmt.Sprintf("%s.%s.%s.%d", name, group, service, index)
}

// CachedJSON retrieves cached data and unmarshals it as JSON.
func CachedJSON(cache *Cache, key string) (map[string]interface{}, bool) {
	data, ok := cache.Get(key)
	if !ok {
		return nil, false
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}
	return result, true
}

// SetJSON marshals data and stores it in the cache.
func SetJSON(cache *Cache, key string, data interface{}, ttl time.Duration) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	cache.Set(key, bytes, ttl)
	return nil
}
