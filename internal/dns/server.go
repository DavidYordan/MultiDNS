package dns

import (
	"log"
	"multidns/internal/config"
	"multidns/pkg/cache"
	"net"
	"sync"
)

type DNSServer struct {
	Config     config.ServerConfig
	Cache      *cache.DNSCache
	CacheCN    *cache.DNSCache
	UpstreamCN []string
	SocksPort  int
	CNDomains  map[string]struct{}
}

func NewDNSServer(cfg config.ServerConfig, cacheCN *cache.DNSCache, upstreamCN []string, socksPort int, cnDomains map[string]struct{}) *DNSServer {
	return &DNSServer{
		Config:     cfg,
		Cache:      cache.NewDNSCache(cfg.CacheCapacity),
		CacheCN:    cacheCN,
		UpstreamCN: upstreamCN,
		SocksPort:  socksPort,
		CNDomains:  cnDomains,
	}
}

func (s *DNSServer) StartTransparentUDP(port int) error {
	addr := &net.UDPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := createTransparentUDPSocket(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("Transparent DNS server started on port %d", port)

	var wg sync.WaitGroup
	sem := make(chan struct{}, 50) // 限制最大并发处理为50

	buffer := make([]byte, 512)
	for {
		n, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			log.Printf("Failed to read from connection: %v", err)
			continue
		}

		sem <- struct{}{} // 请求处理前获取信号量
		wg.Add(1)
		go func(b []byte, a net.Addr) {
			defer wg.Done()
			defer func() { <-sem }() // 处理完毕后释放信号量
			s.handleRequest(conn, a, b)
		}(buffer[:n], addr)
	}
}

func (s *DNSServer) handleRequest(conn net.PacketConn, addr net.Addr, msg []byte) {
	handleDNSRequest(s, conn, addr, msg)
}
