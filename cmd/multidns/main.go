package main

import (
	"fmt"
	"sync"

	"multidns/internal/config"
	"multidns/internal/dns"
	"multidns/internal/utils"
	"multidns/pkg/cache"
)

func main() {
	fmt.Println("Starting multidns service...")

	// 加载配置
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}

	fmt.Printf("Configuration loaded: %+v\n", cfg)

	// 加载 CN 域名列表
	cnDomains, err := utils.LoadCNDomains(cfg.CnDomainFile)
	if err != nil {
		fmt.Printf("Failed to load CN domains: %v\n", err)
		return
	}
	fmt.Println("CN domains loaded successfully")

	// 创建共享缓存
	sharedCache, err := cache.NewDNSCache(int64(cfg.Capacity)<<20, "shared_cache") // Convert MB to bytes
	if err != nil {
		fmt.Printf("Failed to create shared cache: %v\n", err)
		return
	}

	var wg sync.WaitGroup

	// 启动各个 DNS 服务器实例
	for _, serverCfg := range cfg.Servers {
		listenPort := 32000 + serverCfg.Segment
		cacheName := fmt.Sprintf("cache_%d", listenPort)
		socksPort := 31000 + serverCfg.Segment

		dnsServer := dns.NewDNSServer(serverCfg, sharedCache, cfg.UpstreamCN.Address, cfg.UpstreamNonCN.Address, cnDomains, cacheName, socksPort)
		wg.Add(1)
		go func() {
			defer wg.Done()
			dnsServer.StartTransparentUDP(listenPort)
		}()
	}

	wg.Wait()
}
