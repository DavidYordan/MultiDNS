package internal

import (
	"fmt"
	"net"
	"sync"

	"github.com/txthinking/socks5"
)

type DNSServer struct {
	Config        ServerConfig
	Cache         *DNSCache
	UpstreamCN    []string
	UpstreamNonCN []string
	CNDomains     *Trie
	CacheName     string
	SocksPort     int
	socksDialer   *socks5.Client
}

func NewDNSServer(cfg ServerConfig, cache *DNSCache, upstreamCN []string, upstreamNonCN []string, cnDomains *Trie, cacheName string, socksPort int) *DNSServer {
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
	client, err := socks5.NewClient(fmt.Sprintf("127.0.0.1:%d", s.SocksPort), "", "", 5, 5)
	if err != nil {
		fmt.Printf("Failed to create SOCKS5 client: %v\n", err)
		return
	}
	s.socksDialer = client
	fmt.Printf("SOCKS5 %s client created\n", client.Server)
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

	buffer := make([]byte, 4096)
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
			handleDNSRequest(s, conn, a, b)
		}(buffer[:n], addr)
	}
}
