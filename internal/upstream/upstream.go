package upstream

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/proxy"
)

// QueryUpstreamServer queries the upstream DNS servers
func QueryUpstreamServer(dnsMsg *dns.Msg, isCN bool, upstreamCN, upstreamNonCN []string, socksDialer proxy.Dialer, SocksPort int) ([]byte, string, error) {
	upstreamServers := upstreamNonCN
	if isCN {
		upstreamServers = upstreamCN
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultChan := make(chan struct {
		response []byte
		server   string
		err      error
	}, len(upstreamServers)*3)
	var wg sync.WaitGroup

	for _, upstreamAddr := range upstreamServers {
		wg.Add(1)
		go queryAllMethods(ctx, dnsMsg, upstreamAddr, resultChan, &wg, isCN, socksDialer, SocksPort)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for res := range resultChan {
		if res.err == nil {
			cancel() // Cancel other goroutines
			return res.response, res.server, nil
		} else {
			fmt.Printf("Failed to query upstream server %s: %v\n", res.server, res.err)
		}
	}

	return nil, "", fmt.Errorf("failed to get a response from any upstream servers")
}

func queryAllMethods(ctx context.Context, dnsMsg *dns.Msg, upstreamAddr string, resultChan chan struct {
	response []byte
	server   string
	err      error
}, wg *sync.WaitGroup, isCN bool, socksDialer proxy.Dialer, SocksPort int) {
	defer wg.Done()

	if response, server, err := queryUDP(ctx, dnsMsg, upstreamAddr, isCN, socksDialer, SocksPort); err == nil {
		select {
		case resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: server, err: nil}:
		case <-ctx.Done():
		}
		return
	} else {
	}

	if response, server, err := queryTCP(ctx, dnsMsg, upstreamAddr, isCN, socksDialer, SocksPort); err == nil {
		select {
		case resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: server, err: nil}:
		case <-ctx.Done():
		}
		return
	} else {
	}

	if response, server, err := queryTLS(ctx, dnsMsg, upstreamAddr, isCN, socksDialer, SocksPort); err == nil {
		select {
		case resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: server, err: nil}:
		case <-ctx.Done():
		}
		return
	} else {
	}
}

func queryUDP(ctx context.Context, dnsMsg *dns.Msg, upstreamAddr string, isCN bool, socksDialer proxy.Dialer, SocksPort int) ([]byte, string, error) {
	address := upstreamAddr + ":53"
	var conn net.Conn
	var err error

	if isCN {
		conn, err = net.Dial("udp", address)
	} else {
		conn, err = socksDialer.Dial("udp", address)
	}

	if err != nil {
		return nil, "", err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, "", err
	}
	_, err = conn.Write(msg)
	if err != nil {
		return nil, "", err
	}

	response := make([]byte, 4096)
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	default:
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(response)
		if err != nil {
			return nil, "", err
		}
		server := fmt.Sprintf("UDP %s", address)
		if !isCN {
			server = fmt.Sprintf("SOCKS5_%d UDP %s", SocksPort, address)
		}
		return response[:n], server, nil
	}
}

func queryTCP(ctx context.Context, dnsMsg *dns.Msg, upstreamAddr string, isCN bool, socksDialer proxy.Dialer, SocksPort int) ([]byte, string, error) {
	address := upstreamAddr + ":53"
	var conn net.Conn
	var err error

	if isCN {
		conn, err = net.Dial("tcp", address)
	} else {
		conn, err = socksDialer.Dial("tcp", address)
	}

	if err != nil {
		return nil, "", err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, "", err
	}

	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(msg)))
	_, err = conn.Write(append(length, msg...))
	if err != nil {
		return nil, "", err
	}

	response := make([]byte, 4096)
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	default:
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(response)
		if err != nil {
			return nil, "", err
		}
		server := fmt.Sprintf("TCP %s", address)
		if !isCN {
			server = fmt.Sprintf("SOCKS5_%d TCP %s", SocksPort, address)
		}
		return response[2:n], server, nil
	}
}

func queryTLS(ctx context.Context, dnsMsg *dns.Msg, upstreamAddr string, isCN bool, socksDialer proxy.Dialer, SocksPort int) ([]byte, string, error) {
	address := upstreamAddr + ":853"
	var conn net.Conn
	var err error

	if isCN {
		conn, err = tls.Dial("tcp", address, &tls.Config{
			InsecureSkipVerify: true,
		})
	} else {
		conn, err = socksDialer.Dial("tcp", address)
	}

	if err != nil {
		return nil, "", err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, "", err
	}

	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(msg)))
	_, err = conn.Write(append(length, msg...))
	if err != nil {
		return nil, "", err
	}

	response := make([]byte, 4096)
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	default:
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(response)
		if err != nil {
			return nil, "", err
		}
		server := fmt.Sprintf("TLS %s", address)
		if !isCN {
			server = fmt.Sprintf("SOCKS5_%d TLS %s", SocksPort, address)
		}
		return response[2:n], server, nil
	}
}
