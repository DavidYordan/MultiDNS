package internal

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/txthinking/socks5"
)

var httpClientsCN map[string]*http.Client

type DNSServer struct {
	Config           ServerConfig
	Cache            *DNSCache
	UpstreamCN       []string
	UpstreamNonCN    []string
	CNDomains        *Trie
	CacheName        string
	SocksPort        int
	httpClientsNonCN map[string]*http.Client
}

func NewDNSServer(cfg ServerConfig, cache *DNSCache, upstreamCN []string, upstreamNonCN []string, cnDomains *Trie, cacheName string, socksPort int) *DNSServer {
	if httpClientsCN == nil {
		httpClientsCN = make(map[string]*http.Client)
		initHTTPClientsCN(upstreamCN)
	}
	server := &DNSServer{
		Config:           cfg,
		Cache:            cache,
		UpstreamCN:       upstreamCN,
		UpstreamNonCN:    upstreamNonCN,
		CNDomains:        cnDomains,
		CacheName:        cacheName,
		SocksPort:        socksPort,
		httpClientsNonCN: make(map[string]*http.Client),
	}
	server.initHTTPClientsNonCN()
	return server
}

func initHTTPClientsCN(upstreamCN []string) {
	for _, upstream := range upstreamCN {
		httpClientsCN[upstream] = &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:          50,
				MaxIdleConnsPerHost:   20,
				MaxConnsPerHost:       50,
				IdleConnTimeout:       1 * time.Hour,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 0,
			},
			Timeout: 10 * time.Second,
		}
	}
}

func (s *DNSServer) initHTTPClientsNonCN() {
	for _, upstream := range s.UpstreamNonCN {
		socksDialer, err := socks5.NewClient(fmt.Sprintf("127.0.0.1:%d", s.SocksPort), "", "", 5, 5)
		if err != nil {
			fmt.Printf("Failed to create SOCKS5 client for %s: %v\n", upstream, err)
			continue
		}
		s.httpClientsNonCN[upstream] = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return socksDialer.Dial("tcp", addr)
				},
				MaxIdleConns:          20,
				MaxIdleConnsPerHost:   5,
				MaxConnsPerHost:       20,
				IdleConnTimeout:       1 * time.Hour,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			Timeout: 10 * time.Second,
		}
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
