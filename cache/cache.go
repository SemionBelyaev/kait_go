package cache

import (
	"sync"
	"time"
)

type Cache struct {
	data map[string]CacheItem
	mu   sync.RWMutex
}

type CacheItem struct {
	Value      interface{}
	Expiration time.Time
}

func NewCache() *Cache {
	return &Cache{data: make(map[string]CacheItem)}
}

func (c *Cache) Set(key string, value interface{}, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = CacheItem{Value: value, Expiration: time.Now().Add(duration)}
}

func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, found := c.data[key]
	if !found || time.Now().After(item.Expiration) {
		return nil, false
	}
	return item.Value, true
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]CacheItem)
}
