package internal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/miekg/dns"
)

func QueryUpstreamServer(s *DNSServer, dnsMsg *dns.Msg, isCN bool) ([]byte, string, error) {
	upstreamServers := s.UpstreamNonCN
	clients := s.httpClientsNonCN
	if isCN {
		upstreamServers = s.UpstreamCN
		clients = httpClientsCN
	}

	resultChan := make(chan struct {
		response []byte
		server   string
		err      error
	}, len(upstreamServers))
	var wg sync.WaitGroup

	for _, upstreamAddr := range upstreamServers {
		wg.Add(1)
		go func(upstreamAddr string, client *http.Client) {
			defer wg.Done()
			queryDoH(client, dnsMsg, upstreamAddr, resultChan)
		}(upstreamAddr, clients[upstreamAddr])
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for res := range resultChan {
		if res.err == nil {
			return res.response, res.server, nil
		}
	}

	return nil, "", fmt.Errorf("failed to get a response from any upstream servers")
}

func queryDoH(client *http.Client, dnsMsg *dns.Msg, upstreamAddr string, resultChan chan struct {
	response []byte
	server   string
	err      error
}) {
	msg, err := dnsMsg.Pack()
	if err != nil {
		sendResult(upstreamAddr, nil, err, resultChan)
		return
	}

	dohURL := fmt.Sprintf("https://%s/dns-query", upstreamAddr)

	req, err := http.NewRequest(http.MethodPost, dohURL, bytes.NewReader(msg))
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
	select {
	case resultChan <- struct {
		response []byte
		server   string
		err      error
	}{response: response, server: server, err: err}:
	case <-time.After(1 * time.Second):
	}
}
