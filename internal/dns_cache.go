package internal

import (
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/miekg/dns"
)

type DNSCache struct {
	cache *ristretto.Cache
}

type CacheEntry struct {
	Response []byte
	Expiry   int64
}

func NewDNSCache(maxCost int64) (*DNSCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     maxCost,
		BufferItems: 64,
		Metrics:     false,
	})
	if err != nil {
		return nil, err
	}

	return &DNSCache{cache: cache}, nil
}

func (c *DNSCache) Get(key string) ([]byte, int64) {
	if value, found := c.cache.Get(key); found {
		entry := value.(CacheEntry)
		remainingTTL := entry.Expiry - time.Now().Unix()
		if remainingTTL < 0 {
			c.cache.Del(key)
			return nil, 0
		}
		return entry.Response, remainingTTL
	}
	return nil, 0
}

func (c *DNSCache) Set(key string, response []byte, ttl int64) {
	entry := CacheEntry{
		Response: response,
		Expiry:   time.Now().Unix() + ttl,
	}
	c.cache.Set(key, entry, 1)
}

func (c *DNSCache) GetOrUpdate(s *DNSServer, dnsMsg *dns.Msg, isCN bool, key string) ([]byte, int64, string, error) {
	response, remainingTTL := c.Get(key)
	if response != nil && remainingTTL > 0 {
		return response, remainingTTL, s.CacheName, nil
	}

	response, newServer, err := c.updateCache(s, dnsMsg, isCN)
	if err != nil {
		return nil, 0, "", err
	}
	ttl := GetTTLFromResponse(response)
	if ttl > 0 {
		c.Set(key, response, ttl)
	}
	return response, 0, newServer, nil
}

func (c *DNSCache) updateCache(s *DNSServer, dnsMsg *dns.Msg, isCN bool) ([]byte, string, error) {
	response, server, err := QueryUpstreamServer(s, dnsMsg, isCN)
	if err != nil {
		return nil, "", err
	}
	return response, server, nil
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(domain, "."))
}

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
