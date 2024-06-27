package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/miekg/dns"
)

func QueryUpstreamServer(s *DNSServer, dnsMsg *dns.Msg, isCN bool) ([]byte, string, error) {
	upstreamServers := s.UpstreamNonCN
	clients := s.httpClientsNonCN
	if isCN {
		upstreamServers = s.UpstreamCN
		clients = httpClientsCN
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultChan := make(chan struct {
		response []byte
		server   string
		err      error
	}, len(upstreamServers))
	var wg sync.WaitGroup

	for _, upstreamAddr := range upstreamServers {
		wg.Add(1)
		go queryDoH(ctx, clients[upstreamAddr], dnsMsg, upstreamAddr, resultChan, &wg)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for res := range resultChan {
		if res.err == nil {
			cancel()
			if isCN {
				return res.response, fmt.Sprintf("%s", res.server), nil
			} else {
				return res.response, fmt.Sprintf("%s_%d", res.server, s.SocksPort), nil
			}
		}
	}

	return nil, "", fmt.Errorf("failed to get a response from any upstream servers")
}

func queryDoH(ctx context.Context, client *http.Client, dnsMsg *dns.Msg, upstreamAddr string, resultChan chan struct {
	response []byte
	server   string
	err      error
}, wg *sync.WaitGroup) {
	defer wg.Done()

	msg, err := dnsMsg.Pack()
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}

	dohURL := fmt.Sprintf("https://%s/dns-query", upstreamAddr)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dohURL, bytes.NewReader(msg))
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}
	req.Header.Set("Content-Type", "application/dns-message")

	resp, err := client.Do(req)
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}
	defer resp.Body.Close()

	response, err := io.ReadAll(resp.Body)
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}

	sendResult(upstreamAddr, response, nil, resultChan)
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
