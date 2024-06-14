package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	Segment     int  `yaml:"segment"`
	StreamSplit bool `yaml:"stream_split"`
}

type Config struct {
	Servers    []ServerConfig `yaml:"servers"`
	Capacity   int            `yaml:"capacity"`
	UpstreamCN struct {
		Address []string `yaml:"address"`
	} `yaml:"upstream_cn"`
	UpstreamNonCN struct {
		Address []string `yaml:"address"`
	} `yaml:"upstream_non_cn"`
	CnDomainFile string `yaml:"cn_domain_file"`
}

var (
	cfg  *Config
	once sync.Once
)

func GetConfig() (*Config, error) {
	var err error
	once.Do(func() {
		fmt.Println("Loading configuration from /etc/multidns/multidns.yaml")
		data, err := os.ReadFile("/etc/multidns/multidns.yaml")
		if err != nil {
			fmt.Printf("Failed to read configuration file: %v\n", err)
			return
		}
		fmt.Println("Unmarshaling configuration")
		err = yaml.Unmarshal(data, &cfg)
		if err != nil {
			fmt.Printf("Failed to unmarshal configuration: %v\n", err)
			return
		}
		fmt.Println("Configuration loaded successfully")
	})
	return cfg, err
}
