package cache

import (
	"RTTServer/internal/model"
	"sync"
	"time"
)

const TTL = time.Hour

type Store struct {
	mu   sync.RWMutex
	data map[string]model.RTTRecord
}

func New() *Store { return &Store{data: make(map[string]model.RTTRecord)} }

func (c *Store) Set(rec model.RTTRecord) {
	c.mu.Lock()
	c.data[rec.IP] = rec
	c.mu.Unlock()
}

func (c *Store) Get(ip string) (model.RTTRecord, bool) {
	c.mu.RLock()
	rec, ok := c.data[ip]
	c.mu.RUnlock()
	if !ok || time.Since(rec.UpdatedAt) > TTL {
		return model.RTTRecord{}, false
	}
	return rec, true
}

func (c *Store) AllFresh() []model.RTTRecord {
	now := time.Now()
	c.mu.RLock()
	out := make([]model.RTTRecord, 0, len(c.data))
	for _, r := range c.data {
		if now.Sub(r.UpdatedAt) <= TTL {
			out = append(out, r)
		}
	}
	c.mu.RUnlock()
	return out
}

func (c *Store) Janitor(cleanEvery time.Duration) {
	t := time.NewTicker(cleanEvery)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for k, v := range c.data {
			if now.Sub(v.UpdatedAt) > TTL {
				delete(c.data, k)
			}
		}
		c.mu.Unlock()
	}
}
