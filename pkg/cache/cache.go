package cache

import (
	"container/list"
	"sync"
	"time"
)

type CacheEntry struct {
	key      string
	response []byte
	expireAt time.Time
}

type DNSCache struct {
	capacity int
	mu       sync.Mutex
	cache    map[string]*list.Element
	order    *list.List
	size     int
}

func NewDNSCache(capacity int) *DNSCache {
	return &DNSCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

func (c *DNSCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.cache[key]
	if !found {
		return nil, false
	}

	entry := elem.Value.(*CacheEntry)
	if time.Now().After(entry.expireAt) {
		c.removeElement(elem)
		return nil, false
	}

	c.order.MoveToFront(elem)
	return entry.response, true
}

func (c *DNSCache) Set(key string, response []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.cache[key]; found {
		c.order.MoveToFront(elem)
		elem.Value.(*CacheEntry).response = response
		elem.Value.(*CacheEntry).expireAt = time.Now().Add(ttl)
		return
	}

	if c.size >= c.capacity {
		c.removeOldest()
	}

	entry := &CacheEntry{
		key:      key,
		response: response,
		expireAt: time.Now().Add(ttl),
	}
	elem := c.order.PushFront(entry)
	c.cache[key] = elem
	c.size++
}

func (c *DNSCache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	entry := elem.Value.(*CacheEntry)
	delete(c.cache, entry.key)
	c.size--
}

func (c *DNSCache) removeOldest() {
	if elem := c.order.Back(); elem != nil {
		c.removeElement(elem)
	}
}
