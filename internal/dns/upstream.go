package dns

import (
	"log"
	"net"
	"strconv"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/proxy"
)

func (s *DNSServer) queryUpstreamServer(dnsMsg *dns.Msg, isCN bool) ([]byte, error) {
	var conn net.Conn
	var err error

	if isCN {
		conn, err = net.Dial("udp", s.UpstreamCN[0])
		if err != nil {
			return nil, err
		}
	} else {
		dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:"+strconv.Itoa(s.SocksPort), nil, proxy.Direct)
		if err != nil {
			return nil, err
		}
		// 通过 TCP 建立代理连接
		tcpConn, err := dialer.Dial("tcp", "1.1.1.1:53")
		if err != nil {
			return nil, err
		}
		defer tcpConn.Close()

		// 通过 TCP 代理创建 UDP 连接
		udpAddr, err := net.ResolveUDPAddr("udp", "1.1.1.1:53")
		if err != nil {
			return nil, err
		}
		conn, err = net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			return nil, err
		}
	}
	defer conn.Close()

	log.Printf("Querying upstream server for domain: %s, isCN: %v, ID: %d", dnsMsg.Question[0].Name, isCN, dnsMsg.Id)

	// 将 DNS 消息打包并发送
	msg, err := dnsMsg.Pack()
	if err != nil {
		return nil, err
	}
	_, err = conn.Write(msg)
	if err != nil {
		return nil, err
	}

	// 接收响应
	response := make([]byte, 4096)                        // 增大缓冲区以接收较大的响应
	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) // 设置读取超时
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	log.Printf("Received response from upstream server for domain: %s, ID: %d", dnsMsg.Question[0].Name, dnsMsg.Id)

	return response[:n], nil
}
