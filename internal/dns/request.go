package dns

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
		fmt.Printf("Received DNS request with no questions\n")
		return
	}

	var responseMsg dns.Msg
	responseMsg.SetReply(&dnsMsg)
	responseMsg.Compress = true

	var requestInfo string

	for _, question := range dnsMsg.Question {
		key := question.Name
		qtype := question.Qtype

		isCN := s.Config.StreamSplit && s.CNDomains.Search(key)
		var response []byte
		var remainingTTL int64
		var source string
		cacheKey := fmt.Sprintf("%s_%d", key, qtype)

		response, remainingTTL, source, err = s.Cache.GetOrUpdate(&dnsMsg, s.CacheName, isCN, s.UpstreamCN, s.UpstreamNonCN, s.socksDialer, s.SocksPort)

		if err != nil {
			fmt.Printf("Failed to query upstream server for domain %s: %v\n", cacheKey, err)
			return
		}

		requestInfo = fmt.Sprintf("Domain: %s, Type: %d, From: %s, TTL: %d", key, qtype, source, remainingTTL)

		var partialResponse dns.Msg
		err = partialResponse.Unpack(response)
		if err != nil {
			fmt.Printf("Failed to unpack response for domain %s: %v\n", cacheKey, err)
			return
		}

		for i := range partialResponse.Answer {
			partialResponse.Answer[i].Header().Ttl = uint32(remainingTTL)
		}
		for i := range partialResponse.Ns {
			partialResponse.Ns[i].Header().Ttl = uint32(remainingTTL)
		}
		for i := range partialResponse.Extra {
			partialResponse.Extra[i].Header().Ttl = uint32(remainingTTL)
		}

		responseMsg.Answer = append(responseMsg.Answer, partialResponse.Answer...)
		responseMsg.Ns = append(responseMsg.Ns, partialResponse.Ns...)
		responseMsg.Extra = append(responseMsg.Extra, partialResponse.Extra...)
	}

	responseMsg.Id = dnsMsg.Id
	response, err := responseMsg.Pack()
	if err != nil {
		fmt.Printf("Failed to repack response: %v\n", err)
		return
	}

	err = sendRawUDPResponse(addr, response)
	if err != nil {
		fmt.Printf("Failed to send raw UDP response: %v\n", err)
	}

	durationTotal := time.Since(start)
	fmt.Printf("%s - %v\n", requestInfo, durationTotal)
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
