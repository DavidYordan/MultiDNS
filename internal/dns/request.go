package dns

import (
	"log"
	"multidns/internal/utils"
	"net"
	"time"

	"github.com/miekg/dns"
)

func handleDNSRequest(s *DNSServer, conn net.PacketConn, addr net.Addr, msg []byte) {
	start := time.Now()
	var dnsMsg dns.Msg
	err := dnsMsg.Unpack(msg)
	if err != nil {
		log.Printf("Failed to unpack DNS message: %v", err)
		return
	}

	if len(dnsMsg.Question) == 0 {
		log.Printf("Received DNS request with no questions")
		return
	}

	question := dnsMsg.Question[0]
	key := question.Name
	isCN := s.Config.StreamSplit && utils.IsCNDomain(question.Name, s.CNDomains) // 判断一次，存储结果

	log.Printf("Received DNS request for domain: %s, isCN: %v, ID: %d", key, isCN, dnsMsg.Id)

	var response []byte
	cacheHit := false
	cacheSource := ""

	if isCN {
		if cachedResponse, found := s.CacheCN.Get(key); found {
			response = append([]byte{}, cachedResponse...)
			cacheHit = true
			cacheSource = "CacheCN"
			log.Printf("Cache hit for CN domain: %s (CacheCN)", key)
		}
	} else {
		if cachedResponse, found := s.Cache.Get(key); found {
			response = append([]byte{}, cachedResponse...)
			cacheHit = true
			cacheSource = "Cache"
			log.Printf("Cache hit for non-CN domain: %s (Cache)", key)
		}
	}

	if !cacheHit {
		log.Printf("Cache miss for domain: %s, querying upstream", key)
		response, err = s.queryUpstreamServer(&dnsMsg, isCN) // 传递isCN状态
		duration := time.Since(start)
		log.Printf("Upstream query duration for domain %s: %v", key, duration)
		if err != nil {
			log.Printf("Failed to query upstream server for domain %s: %v", key, err)
			return
		}

		ttl := utils.GetTTLFromResponse(response)
		if isCN {
			s.CacheCN.Set(key, response, ttl)
			log.Printf("Setting cache for CN domain: %s, TTL: %v", key, ttl)
		} else {
			s.Cache.Set(key, response, ttl)
			log.Printf("Setting cache for non-CN domain: %s, TTL: %v", key, ttl)
		}
	}

	// 确保响应的 ID 与请求的 ID 匹配
	var responseMsg dns.Msg
	err = responseMsg.Unpack(response)
	if err != nil {
		log.Printf("Failed to unpack response for domain %s: %v", key, err)
		return
	}
	responseMsg.Id = dnsMsg.Id
	response, err = responseMsg.Pack()
	if err != nil {
		log.Printf("Failed to repack response for domain %s: %v", key, err)
		return
	}

	// 创建新的 UDP 连接以发送响应
	localAddr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0}
	remoteAddr := addr.(*net.UDPAddr)
	outConn, err := net.DialUDP("udp", localAddr, remoteAddr)
	if err != nil {
		log.Printf("Failed to create UDP connection for response: %v", err)
		return
	}
	defer outConn.Close()

	log.Printf("Sending DNS response for domain %s to %s from local address %s", key, remoteAddr.String(), outConn.LocalAddr().String())

	_, err = outConn.Write(response)
	if err != nil {
		log.Printf("Failed to send DNS response for domain %s to %s: %v", key, remoteAddr.String(), err)
	} else {
		log.Printf("Sent DNS response for domain %s (Cache: %s, ID: %d) to %s", key, cacheSource, dnsMsg.Id, remoteAddr.String())
	}

	durationTotal := time.Since(start)
	log.Printf("Total processing duration for domain %s: %v", key, durationTotal)

	// 记录返回的 IP 地址
	for _, answer := range responseMsg.Answer {
		if aRecord, ok := answer.(*dns.A); ok {
			log.Printf("DNS response for domain %s contains IP: %s", key, aRecord.A.String())
		}
	}
}
