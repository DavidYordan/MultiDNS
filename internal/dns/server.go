package dns

import (
	"fmt"
	"multidns/internal/config"
	"multidns/internal/utils"
	"multidns/pkg/cache"
	"net"
	"strconv"
	"sync"

	"golang.org/x/net/proxy"
)

type DNSServer struct {
	Config        config.ServerConfig
	Cache         *cache.DNSCache
	UpstreamCN    []string
	UpstreamNonCN []string
	CNDomains     *utils.Trie
	CacheName     string
	SocksPort     int
	socksDialer   proxy.Dialer
}

func NewDNSServer(cfg config.ServerConfig, cache *cache.DNSCache, upstreamCN []string, upstreamNonCN []string, cnDomains *utils.Trie, cacheName string, socksPort int) *DNSServer {
	server := &DNSServer{
		Config:        cfg,
		Cache:         cache,
		UpstreamCN:    upstreamCN,
		UpstreamNonCN: upstreamNonCN,
		CNDomains:     cnDomains,
		CacheName:     cacheName,
		SocksPort:     socksPort,
	}
	server.initSocksDialer()
	return server
}

func (s *DNSServer) initSocksDialer() {
	var err error
	s.socksDialer, err = proxy.SOCKS5("tcp", "127.0.0.1:"+strconv.Itoa(s.SocksPort), nil, proxy.Direct)
	if err != nil {
		fmt.Printf("Failed to create SOCKS5 dialer: %v\n", err)
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
	fmt.Printf("Transparent DNS server started on port %d\n", port)

	var wg sync.WaitGroup
	sem := make(chan struct{}, 100)

	buffer := make([]byte, 512)
	for {
		n, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			fmt.Printf("Failed to read from connection: %v\n", err)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(b []byte, a net.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			s.handleRequest(conn, a, b)
		}(buffer[:n], addr)
	}
}

func (s *DNSServer) handleRequest(conn net.PacketConn, addr net.Addr, msg []byte) {
	handleDNSRequest(s, conn, addr, msg)
}
