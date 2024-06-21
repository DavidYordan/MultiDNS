package internal

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
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

	if isCN {
		for _, upstreamAddr := range upstreamServers {
			wg.Add(1)
			go queryDirectUDP(ctx, dnsMsg, upstreamAddr, s.socksDialer, resultChan, &wg)
		}
	} else {
		for _, upstreamAddr := range upstreamServers {
			wg.Add(1)
			go querySocksMethods(ctx, dnsMsg, upstreamAddr, s.socksDialer, resultChan, &wg, s.Config.StreamUot)
		}
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
}, wg *sync.WaitGroup, useTCP bool) {
	defer wg.Done()

	var response []byte
	var err error
	if useTCP {
		response, err = queryThroughSocks5TCP(dnsMsg, dialer, upstreamAddr)
	} else {
		response, err = queryThroughSocks5UDP(dnsMsg, dialer, upstreamAddr)
	}

	if err == nil {
		sendResult(fmt.Sprintf("%s", upstreamAddr), response, nil, resultChan)
	} else {
		sendResult(fmt.Sprintf("%s", upstreamAddr), nil, err, resultChan)
	}
}

func queryThroughSocks5TCP(dnsMsg *dns.Msg, dialer *socks5.Client, upstreamAddr string) ([]byte, error) {
	conn, err := dialer.Dial("tcp", upstreamAddr+":53")
	if err != nil {
		fmt.Printf("Error(Dial): TCP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		fmt.Printf("Error(Pack): TCP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}

	lengthMsg := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(lengthMsg[:2], uint16(len(msg)))
	copy(lengthMsg[2:], msg)

	_, err = conn.Write(lengthMsg)
	if err != nil {
		fmt.Printf("Error(Write): TCP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}

	var lengthBuf [2]byte
	_, err = io.ReadFull(conn, lengthBuf[:])
	if err != nil {
		fmt.Printf("Error(ReadFull): TCP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}
	length := binary.BigEndian.Uint16(lengthBuf[:])

	response := make([]byte, length)
	_, err = io.ReadFull(conn, response)
	if err != nil {
		fmt.Printf("Error(ReadFull): TCP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}

	return response, nil
}

func queryThroughSocks5UDP(dnsMsg *dns.Msg, dialer *socks5.Client, upstreamAddr string) ([]byte, error) {
	conn, err := dialer.Dial("udp", upstreamAddr+":53")
	if err != nil {
		fmt.Printf("Error(Dial): UDP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		fmt.Printf("Error(Pack): UDP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}

	_, err = conn.Write(msg)
	if err != nil {
		fmt.Printf("Error(Write): UDP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}

	initialBufferSize := 512
	buffer := make([]byte, initialBufferSize)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Printf("Error(Read): UDP %s_%s - %v\n", dialer.Server, upstreamAddr, err)
		return nil, err
	}

	if n == initialBufferSize {
		extendedBuffer := make([]byte, 4096)
		extendedN, err := conn.Read(extendedBuffer)
		if err == nil && extendedN > 0 {
			buffer = append(buffer, extendedBuffer[:extendedN]...)
			n += extendedN
		}
	}

	return buffer[:n], nil
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
