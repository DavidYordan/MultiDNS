package utils

import (
	"bufio"
	"os"
	"time"

	"github.com/miekg/dns"
)

func GetTTLFromResponse(response []byte) time.Duration {
	var dnsMsg dns.Msg
	if err := dnsMsg.Unpack(response); err != nil {
		return 0
	}

	if len(dnsMsg.Answer) > 0 {
		return time.Duration(dnsMsg.Answer[0].Header().Ttl) * time.Second
	}

	return 0
}

func LoadCNDomains(filePath string) (map[string]struct{}, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cnDomains := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		domain := scanner.Text()
		if domain != "" {
			cnDomains[domain] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return cnDomains, nil
}

func IsCNDomain(domain string, cnDomains map[string]struct{}) bool {
	_, found := cnDomains[domain]
	return found
}
