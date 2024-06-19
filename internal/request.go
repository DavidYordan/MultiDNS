package internal

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
)

func handleDNSRequest(s *DNSServer, conn net.PacketConn, addr net.Addr, msg []byte) {
	start := time.Now()
	var dnsMsg dns.Msg
	err := dnsMsg.Unpack(msg)
	if err != nil {
		fmt.Printf("Failed to unpack DNS message: %v\n", err)
		return
	}

	if len(dnsMsg.Question) == 0 {
		fmt.Printf("No questions in a DNS message\n")
		return
	}

	clientAddr := addr.String()

	if len(dnsMsg.Question) > 1 {
		response, source, err := handleMultipleQuestions(s, conn, &dnsMsg)
		if err != nil {
			fmt.Printf("Failed to handle multiple questions: %v\n", err)
		}
		sendResponse(conn, addr, response)
		durationTotal := time.Since(start).Seconds() * 1000
		fmt.Printf("%.3fms - %s_%s - multipleQuestions\n", durationTotal, clientAddr, source)
		return
	} else {
		question := dnsMsg.Question[0]
		domain := question.Name
		qtype := dns.TypeToString[question.Qtype]
		key := generateKey(s.CacheName, qtype, domain)

		var responseMsg dns.Msg
		responseMsg.SetReply(&dnsMsg)

		isCN := s.Config.StreamSplit && s.CNDomains.Search(question.Name)
		response, remainingTTL, source, err := s.Cache.GetOrUpdate(s, &dnsMsg, isCN, key)
		if err != nil {
			fmt.Printf("Failed to query upstream server for domain %s: %v\n", question.Name, err)
			return
		}

		packedResponse, err := unpackSetTTLAndID(response, dnsMsg.Id, remainingTTL)
		if err != nil {
			fmt.Printf("Failed to process response for domain %s: %v\n", question.Name, err)
			return
		}

		sendResponse(conn, addr, packedResponse)
		durationTotal := time.Since(start).Seconds() * 1000
		fmt.Printf("%.3fms - %s_%s - %s\n", durationTotal, clientAddr, source, question.Name)
	}
}

func generateKey(cacheName, qtype, domain string) string {
	return cacheName + ":" + qtype + ":" + normalizeDomain(domain)
}

func handleMultipleQuestions(s *DNSServer, conn net.PacketConn, dnsMsg *dns.Msg) ([]byte, string, error) {
	isCN := s.Config.StreamSplit && s.CNDomains.Search(dnsMsg.Question[0].Name)
	response, source, err := QueryUpstreamServer(s, dnsMsg, isCN)
	if err != nil {
		fmt.Printf("Failed to query upstream server: %v\n", err)
		return nil, source, err
	}

	packedResponse, err := unpackSetID(response, dnsMsg.Id)
	if err != nil {
		fmt.Printf("Failed to process response: %v\n", err)
		return nil, source, err
	}
	return packedResponse, source, nil
}

func sendResponse(conn net.PacketConn, addr net.Addr, response []byte) error {
	if conn != nil {
		err := sendRawUDPResponse(addr, response)
		if err != nil {
			fmt.Printf("Failed to send raw UDP response: %v\n", err)
		}
		return err
	}
	err := sendRawTCPResponse(addr, response)
	if err != nil {
		fmt.Printf("Failed to send raw TCP response: %v\n", err)
	}
	return err
}

func sendRawUDPResponse(addr net.Addr, response []byte) error {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("address is not UDPAddr")
	}

	conn, err := net.ListenPacket("ip4:udp", "0.0.0.0")
	if err != nil {
		return err
	}
	defer conn.Close()

	rawConn, err := ipv4.NewRawConn(conn)
	if err != nil {
		return err
	}

	header := &ipv4.Header{
		Version:  4,
		Len:      ipv4.HeaderLen,
		TotalLen: ipv4.HeaderLen + 8 + len(response),
		TTL:      64,
		Protocol: 17,
		Src:      net.ParseIP("8.8.8.8").To4(),
		Dst:      udpAddr.IP.To4(),
	}

	udpHeader := &UDPHeader{
		SrcPort: 53,
		DstPort: uint16(udpAddr.Port),
		Length:  uint16(8 + len(response)),
	}

	udpPayload := append(udpHeader.Marshal(), response...)

	checksum := udpChecksum(header, udpPayload)
	udpHeader.Checksum = checksum

	payload := append(udpHeader.Marshal(), response...)

	return rawConn.WriteTo(header, payload, nil)
}

func sendRawTCPResponse(addr net.Addr, response []byte) error {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("address is not TCPAddr")
	}

	conn, err := net.Dial("tcp", tcpAddr.String())
	if err != nil {
		return fmt.Errorf("Failed to create TCP connection: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write(response)
	if err != nil {
		return fmt.Errorf("Failed to send TCP response: %v", err)
	}

	return nil
}

func udpChecksum(header *ipv4.Header, payload []byte) uint16 {
	pseudoHeader := pseudoHeader(header.Src, header.Dst, 17, len(payload))
	checksum := calculateChecksum(append(pseudoHeader, payload...))
	return checksum
}

func pseudoHeader(src, dst net.IP, protocol, length int) []byte {
	return []byte{
		src[0], src[1], src[2], src[3],
		dst[0], dst[1], dst[2], dst[3],
		0,
		byte(protocol),
		byte(length >> 8), byte(length),
	}
}

func calculateChecksum(data []byte) uint16 {
	sum := 0
	for i := 0; i < len(data)-1; i += 2 {
		sum += int(data[i])<<8 | int(data[i+1])
	}
	if len(data)%2 == 1 {
		sum += int(data[len(data)-1]) << 8
	}
	sum = (sum >> 16) + (sum & 0xffff)
	sum += sum >> 16
	return uint16(^sum)
}

type UDPHeader struct {
	SrcPort  uint16
	DstPort  uint16
	Length   uint16
	Checksum uint16
}

func (h *UDPHeader) Marshal() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint16(b[0:2], h.SrcPort)
	binary.BigEndian.PutUint16(b[2:4], h.DstPort)
	binary.BigEndian.PutUint16(b[4:6], h.Length)
	binary.BigEndian.PutUint16(b[6:8], h.Checksum)
	return b
}

func unpackSetID(response []byte, originalID uint16) ([]byte, error) {
	var responseMsg dns.Msg
	err := responseMsg.Unpack(response)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack response: %v", err)
	}
	responseMsg.Id = originalID

	packedResponse, err := responseMsg.Pack()
	if err != nil {
		return nil, fmt.Errorf("failed to repack response: %v", err)
	}
	return packedResponse, nil
}

func unpackSetTTLAndID(response []byte, originalID uint16, remainingTTL int64) ([]byte, error) {
	var responseMsg dns.Msg
	err := responseMsg.Unpack(response)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack response: %v", err)
	}
	responseMsg.Id = originalID

	if remainingTTL > 0 {
		for i := range responseMsg.Answer {
			responseMsg.Answer[i].Header().Ttl = uint32(remainingTTL)
		}
		for i := range responseMsg.Ns {
			responseMsg.Ns[i].Header().Ttl = uint32(remainingTTL)
		}
		for i := range responseMsg.Extra {
			responseMsg.Extra[i].Header().Ttl = uint32(remainingTTL)
		}
	}

	packedResponse, err := responseMsg.Pack()
	if err != nil {
		return nil, fmt.Errorf("failed to repack response: %v", err)
	}
	return packedResponse, nil
}
