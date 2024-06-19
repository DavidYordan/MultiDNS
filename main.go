package main

import (
	"fmt"
	"multidns/internal"
	"sync"
)

func main() {
	fmt.Println("Starting multidns service...")

	cfg, err := internal.GetConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}

	fmt.Printf("Configuration loaded: %+v\n", cfg)

	cnDomains, err := internal.LoadCNDomains(cfg.CnDomainFile)
	if err != nil {
		fmt.Printf("Failed to load CN domains: %v\n", err)
		return
	}
	fmt.Println("CN domains loaded successfully")

	sharedCache, err := internal.NewDNSCache(int64(cfg.Capacity) << 20)
	if err != nil {
		fmt.Printf("Failed to create shared cache: %v\n", err)
		return
	}

	var wg sync.WaitGroup

	for _, serverCfg := range cfg.Servers {
		listenPort := 32000 + serverCfg.Segment
		cacheName := fmt.Sprintf("cache_%d", listenPort)
		socksPort := 31000 + serverCfg.Segment

		dnsServer := internal.NewDNSServer(serverCfg, sharedCache, cfg.UpstreamCN.Address, cfg.UpstreamNonCN.Address, cnDomains, cacheName, socksPort)
		wg.Add(1)
		go func() {
			defer wg.Done()
			dnsServer.StartTransparentUDP(listenPort)
		}()
	}

	wg.Wait()
}
