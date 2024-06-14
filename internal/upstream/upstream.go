package upstream

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// QueryUpstreamServer queries the upstream DNS servers
func QueryUpstreamServer(dnsMsg *dns.Msg, isCN bool, upstreamCN, upstreamNonCN []string, socksPort int) ([]byte, string, error) {
	upstreamServers := upstreamNonCN
	if isCN {
		upstreamServers = upstreamCN
	}

	resultChan := make(chan struct {
		response []byte
		server   string
		err      error
	}, len(upstreamServers)*3+len(upstreamServers))
	var wg sync.WaitGroup

	for _, upstreamAddr := range upstreamServers {
		wg.Add(1)
		go queryAllMethods(dnsMsg, upstreamAddr, resultChan, &wg)
	}

	if !isCN {
		for _, upstreamAddr := range upstreamServers {
			wg.Add(1)
			go querySocks5UDP(dnsMsg, socksPort, upstreamAddr, resultChan, &wg)
		}
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

func queryAllMethods(dnsMsg *dns.Msg, upstreamAddr string, resultChan chan struct {
	response []byte
	server   string
	err      error
}, wg *sync.WaitGroup) {
	defer wg.Done()
	if response, err := queryDirectUDP(dnsMsg, upstreamAddr); err == nil {
		resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: "UDP " + upstreamAddr, err: nil}
		return
	}

	if response, err := queryDirectTCP(dnsMsg, upstreamAddr); err == nil {
		resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: "TCP " + upstreamAddr, err: nil}
		return
	}

	if response, err := queryDirectTLS(dnsMsg, upstreamAddr); err == nil {
		resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: "TLS " + upstreamAddr, err: nil}
		return
	}
}

func querySocks5UDP(dnsMsg *dns.Msg, socksPort int, upstreamAddr string, resultChan chan struct {
	response []byte
	server   string
	err      error
}, wg *sync.WaitGroup) {
	defer wg.Done()
	response, err := queryThroughSocks5UDP(dnsMsg, socksPort, upstreamAddr)
	if err == nil {
		resultChan <- struct {
			response []byte
			server   string
			err      error
		}{response: response, server: fmt.Sprintf("SOCKS5 127.0.0.1:%d to %s", socksPort, upstreamAddr), err: nil}
	}
}

func queryDirectUDP(dnsMsg *dns.Msg, upstreamAddr string) ([]byte, error) {
	address := upstreamAddr + ":53"
	conn, err := net.Dial("udp", address)
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

	response := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	return response[:n], nil
}

func queryDirectTCP(dnsMsg *dns.Msg, upstreamAddr string) ([]byte, error) {
	address := upstreamAddr + ":53"
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, err
	}

	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(msg)))
	_, err = conn.Write(append(length, msg...))
	if err != nil {
		return nil, err
	}

	response := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	return response[2:n], nil
}

func queryDirectTLS(dnsMsg *dns.Msg, upstreamAddr string) ([]byte, error) {
	address := upstreamAddr + ":853"
	conn, err := tls.Dial("tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, err
	}

	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(msg)))
	_, err = conn.Write(append(length, msg...))
	if err != nil {
		return nil, err
	}

	response := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	return response[2:n], nil
}

func queryThroughSocks5UDP(dnsMsg *dns.Msg, socksPort int, upstreamAddr string) ([]byte, error) {
	socksAddr := "127.0.0.1:" + strconv.Itoa(socksPort)
	tcpConn, err := net.Dial("tcp", socksAddr)
	if err != nil {
		return nil, err
	}
	defer tcpConn.Close()

	if _, err := tcpConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := tcpConn.Read(resp); err != nil {
		return nil, err
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return nil, fmt.Errorf("SOCKS5 handshake failed for %s", socksAddr)
	}

	udpRequest := []byte{0x05, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := tcpConn.Write(udpRequest); err != nil {
		return nil, err
	}
	resp = make([]byte, 10)
	if _, err := tcpConn.Read(resp); err != nil {
		return nil, err
	}
	if resp[1] != 0x00 {
		return nil, fmt.Errorf("SOCKS5 UDP Associate failed for %s", socksAddr)
	}

	udpAddr := &net.UDPAddr{
		IP:   net.IP(resp[4:8]),
		Port: int(binary.BigEndian.Uint16(resp[8:10])),
	}
	udpConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
	defer udpConn.Close()

	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, err
	}

	socks5UDPRequest := buildSocks5UDPRequest(msg, upstreamAddr, 53)
	_, err = udpConn.Write(socks5UDPRequest)
	if err != nil {
		return nil, err
	}

	response := make([]byte, 4096)
	udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := udpConn.Read(response)
	if err != nil {
		return nil, err
	}

	return removeSocks5UDPHeader(response[:n])
}

func buildSocks5UDPRequest(data []byte, destAddr string, destPort int) []byte {
	destIP := net.ParseIP(destAddr).To4()
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(destPort))

	header := append([]byte{0x00, 0x00, 0x00, 0x01}, destIP...)
	header = append(header, portBytes...)
	return append(header, data...)
}

func removeSocks5UDPHeader(data []byte) ([]byte, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("invalid SOCKS5 UDP response")
	}
	return data[10:], nil
}
