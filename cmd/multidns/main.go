package main

import (
	"log"
	"multidns/internal/config"
	"multidns/internal/dns"
	"multidns/internal/utils"
	"multidns/pkg/cache"
	"os"
	"strconv"
	"sync"
)

func main() {
	logDir := "/var/log/multidns"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
			log.Fatalf("Failed to create logs directory: %v", err)
		}
	}

	logFile, err := os.OpenFile(logDir+"/multidns.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Println("Starting multidns service...")

	cfg, err := config.LoadConfig("/etc/multidns/multidns.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: %+v", cfg)

	// 加载 cn_site.list 文件
	cnDomains, err := utils.LoadCNDomains("/etc/multidns/cn_site.list")
	if err != nil {
		log.Fatalf("Failed to load CN domains: %v", err)
	}
	log.Println("CN domains loaded successfully")

	cacheCN := cache.NewDNSCache(cfg.CacheCN.Capacity)

	var wg sync.WaitGroup
	for _, serverCfg := range cfg.Servers {
		port, err := strconv.Atoi(serverCfg.ID)
		if err != nil {
			log.Fatalf("Invalid server ID: %v", err)
		}
		socksPort := port + 1000
		listenPort := port + 2000

		dnsServer := dns.NewDNSServer(serverCfg, cacheCN, cfg.UpstreamCN.Address, socksPort, cnDomains)
		wg.Add(1)
		go func() {
			defer wg.Done()
			dnsServer.StartTransparentUDP(listenPort)
		}()
	}

	wg.Wait()

	log.Println("Service started successfully")
}
