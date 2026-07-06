package memcache

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrMiss = errors.New("memcache: miss")

const defaultMaxEntries = 4096

type entry struct {
	value     []byte
	expiresAt time.Time
}

type Cache struct {
	mu         sync.Mutex
	items      map[string]entry
	maxEntries int
	now        func() time.Time
}

func New(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}
	return &Cache{
		items:      make(map[string]entry),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (c *Cache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	if !ok {
		return nil, ErrMiss
	}
	if c.now().After(item.expiresAt) {
		delete(c.items, key)
		return nil, ErrMiss
	}
	out := make([]byte, len(item.value))
	copy(out, item.value)
	return out, nil
}

func (c *Cache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Minute
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.makeRoomLocked(key)
	c.items[key] = entry{value: cloneBytes(value), expiresAt: c.now().Add(ttl)}
	return nil
}

func (c *Cache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.items, key)
	}
	return nil
}

func (c *Cache) SetNX(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = time.Minute
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, ok := c.items[key]; ok && c.now().Before(item.expiresAt) {
		return false, nil
	}
	c.makeRoomLocked(key)
	c.items[key] = entry{value: cloneBytes(value), expiresAt: c.now().Add(ttl)}
	return true, nil
}

func (c *Cache) makeRoomLocked(incoming string) {
	if _, exists := c.items[incoming]; exists || len(c.items) < c.maxEntries {
		return
	}
	nowTime := c.now()
	for key, item := range c.items {
		if nowTime.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
	for key := range c.items {
		if len(c.items) < c.maxEntries {
			break
		}
		delete(c.items, key)
	}
}

func cloneBytes(value []byte) []byte {
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
