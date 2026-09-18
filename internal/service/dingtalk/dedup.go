package dingtalk

import (
	"sync"
	"time"
)

type msgDeduper struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	ttl    time.Duration
	lastGC time.Time
}

func newMsgDeduper(ttl time.Duration) *msgDeduper {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &msgDeduper{seen: map[string]time.Time{}, ttl: ttl}
}

func (d *msgDeduper) First(id string) bool {
	if id == "" {
		return true
	}
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.seen[id]; ok && now.Sub(t) < d.ttl {
		return false
	}
	d.seen[id] = now
	if now.Sub(d.lastGC) > d.ttl {
		for k, t := range d.seen {
			if now.Sub(t) >= d.ttl {
				delete(d.seen, k)
			}
		}
		d.lastGC = now
	}
	return true
}
