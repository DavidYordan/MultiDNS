package cache

import (
	"fmt"
	"strings"
	"time"

	"multidns/internal/upstream"

	"github.com/dgraph-io/ristretto"
	"github.com/miekg/dns"
)

// DNSCache represents the DNS cache using ristretto
type DNSCache struct {
	cache     *ristretto.Cache
	cacheName string
}

// CacheEntry represents a cache entry with response and expiry
type CacheEntry struct {
	Response []byte
	Expiry   int64
}

// NewDNSCache initializes a new DNSCache with ristretto
func NewDNSCache(maxCost int64, cacheName string) (*DNSCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     maxCost, // maximum cost of cache.
		BufferItems: 64,      // number of keys per Get buffer.
		Metrics:     true,    // enable metrics collection.
	})
	if err != nil {
		return nil, err
	}

	return &DNSCache{
		cache:     cache,
		cacheName: cacheName,
	}, nil
}

// Get retrieves a DNS response from the cache for the specified domain name and type
// and returns whether the response is expired and remaining TTL
func (c *DNSCache) Get(cacheName, qtype, domain string) ([]byte, bool, int64) {
	key := c.generateKey(cacheName, qtype, domain)
	if value, found := c.cache.Get(key); found {
		entry := value.(CacheEntry)
		expired := time.Now().Unix() > entry.Expiry
		remainingTTL := entry.Expiry - time.Now().Unix()
		if remainingTTL < 0 {
			remainingTTL = 1 // If expired, set TTL to 1 to indicate it's stale
		}
		c.printMetrics()
		return entry.Response, expired, remainingTTL
	}
	c.printMetrics()
	return nil, false, 0
}

// Set stores a DNS response in the cache for the specified domain name and type with the given TTL
func (c *DNSCache) Set(cacheName, qtype, domain string, response []byte, ttl int64) {
	key := c.generateKey(cacheName, qtype, domain)
	entry := CacheEntry{
		Response: response,
		Expiry:   time.Now().Unix() + ttl,
	}
	// Assuming each entry has a cost of 1. Adjust this if you have more complex cost calculation.
	c.cache.Set(key, entry, 1)
}

// GetOrUpdate retrieves a DNS response from the cache for the specified domain name and type
// If the response is not found or expired, it queries the upstream server and updates the cache
func (c *DNSCache) GetOrUpdate(dnsMsg *dns.Msg, cacheName string, isCN bool, upstreamCN, upstreamNonCN []string, socksPort int) ([]byte, int64, string, error) {
	domain := dnsMsg.Question[0].Name
	qtype := dns.TypeToString[dnsMsg.Question[0].Qtype]
	response, expired, remainingTTL := c.Get(cacheName, qtype, domain)
	if response != nil && !expired {
		return response, remainingTTL, cacheName, nil
	}

	if response != nil && expired {
		// 已过期的数据立即返回，同时异步更新缓存
		go func() {
			newResponse, _, err := c.updateCache(dnsMsg, isCN, upstreamCN, upstreamNonCN, socksPort)
			if err == nil {
				c.Set(cacheName, qtype, domain, newResponse, GetTTLFromResponse(newResponse))
			}
		}()
		return response, 1, cacheName, nil
	}

	// 没有找到数据，查询上游并更新缓存
	response, newServer, err := c.updateCache(dnsMsg, isCN, upstreamCN, upstreamNonCN, socksPort)
	if err != nil {
		return nil, 0, "", err
	}
	ttl := GetTTLFromResponse(response)
	c.Set(cacheName, qtype, domain, response, ttl)
	return response, ttl, newServer, nil
}

func (c *DNSCache) updateCache(dnsMsg *dns.Msg, isCN bool, upstreamCN, upstreamNonCN []string, socksPort int) ([]byte, string, error) {
	response, server, err := upstream.QueryUpstreamServer(dnsMsg, isCN, upstreamCN, upstreamNonCN, socksPort)
	if err != nil {
		return nil, "", err
	}
	return response, server, nil
}

// normalizeDomain ensures that the domain name is in a consistent format for caching
func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(domain, "."))
}

// generateKey creates a unique key for caching based on cacheName, qtype, and domain
func (c *DNSCache) generateKey(cacheName, qtype, domain string) string {
	return cacheName + ":" + qtype + ":" + normalizeDomain(domain)
}

// CacheName returns the name of the cache
func (c *DNSCache) CacheName() string {
	return c.cacheName
}

// Metrics returns the metrics collected by Ristretto
func (c *DNSCache) Metrics() *ristretto.Metrics {
	return c.cache.Metrics
}

// GetTTLFromResponse extracts the TTL from a DNS response
func GetTTLFromResponse(response []byte) int64 {
	var dnsMsg dns.Msg
	if err := dnsMsg.Unpack(response); err != nil {
		return 0
	}

	if len(dnsMsg.Answer) > 0 {
		return int64(dnsMsg.Answer[0].Header().Ttl)
	}

	return 0
}

// printMetrics prints the metrics collected by Ristretto
func (c *DNSCache) printMetrics() {
	metrics := c.Metrics()
	fmt.Printf("Metrics - Hits: %d, Misses: %d, Hit Ratio: %.2f, Cost Added: %d, Cost Evicted: %d\n",
		metrics.Hits(),
		metrics.Misses(),
		metrics.Ratio(),
		metrics.CostAdded(),
		metrics.CostEvicted(),
	)
}
