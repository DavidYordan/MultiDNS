package internal

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/txthinking/socks5"
)

func QueryUpstreamServer(s *DNSServer, dnsMsg *dns.Msg, isCN bool) ([]byte, string, error) {
	upstreamServers := s.UpstreamNonCN
	if isCN {
		upstreamServers = s.UpstreamCN
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultChan := make(chan struct {
		response []byte
		server   string
		err      error
	}, len(upstreamServers))
	var wg sync.WaitGroup

	queryFunc := queryDirectUDP
	if !isCN {
		queryFunc = querySocksMethods
	}

	for _, upstreamAddr := range upstreamServers {
		wg.Add(1)
		go queryFunc(ctx, dnsMsg, upstreamAddr, s.socksDialer, resultChan, &wg)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for res := range resultChan {
		if res.err == nil {
			cancel()
			return res.response, fmt.Sprintf("%d_%s", s.SocksPort, res.server), nil
		}
	}

	return nil, "", fmt.Errorf("failed to get a response from any upstream servers")
}

func queryDirectUDP(ctx context.Context, dnsMsg *dns.Msg, upstreamAddr string, dialer *socks5.Client, resultChan chan struct {
	response []byte
	server   string
	err      error
}, wg *sync.WaitGroup) {
	defer wg.Done()

	address := upstreamAddr + ":53"
	conn, err := net.Dial("udp", address)
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}

	_, err = conn.Write(msg)
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}

	response := make([]byte, 512)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	select {
	case <-ctx.Done():
		return
	default:
		n, err := conn.Read(response)
		sendResult(upstreamAddr, response[:n], err, resultChan)
	}
}

func querySocksMethods(ctx context.Context, dnsMsg *dns.Msg, upstreamAddr string, dialer *socks5.Client, resultChan chan struct {
	response []byte
	server   string
	err      error
}, wg *sync.WaitGroup) {
	defer wg.Done()

	response, err := queryThroughSocks5UDP(dnsMsg, dialer, upstreamAddr)
	if err == nil {
		sendResult(fmt.Sprintf("%s", upstreamAddr), response, nil, resultChan)
	} else {
		sendResult(fmt.Sprintf("%s", upstreamAddr), nil, err, resultChan)
	}
}

func queryThroughSocks5UDP(dnsMsg *dns.Msg, dialer *socks5.Client, upstreamAddr string) ([]byte, error) {
	conn, err := dialer.Dial("udp", upstreamAddr+":53")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(msg)
	if err != nil {
		return nil, err
	}

	response := make([]byte, 512)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	return response[:n], nil
}

func sendResult(server string, response []byte, err error, resultChan chan struct {
	response []byte
	server   string
	err      error
}) {
	resultChan <- struct {
		response []byte
		server   string
		err      error
	}{response: response, server: server, err: err}
}
