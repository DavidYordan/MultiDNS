package config

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	ID            string `yaml:"id"`
	StreamSplit   bool   `yaml:"stream_split"`
	CacheCapacity int    `yaml:"cache_capacity"`
}

type Config struct {
	Servers []ServerConfig `yaml:"servers"`
	CacheCN struct {
		Capacity int `yaml:"capacity"`
	} `yaml:"cache_cn"`
	UpstreamCN struct {
		Address []string `yaml:"address"`
	} `yaml:"upstream_cn"`
}

func LoadConfig(configPath string) (*Config, error) {
	log.Printf("Loading configuration from %s", configPath)
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read configuration file: %v", err)
		return nil, err
	}
	var config Config
	log.Println("Unmarshaling configuration")
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Failed to unmarshal configuration: %v", err)
		return nil, err
	}
	log.Println("Configuration loaded successfully")
	return &config, nil
}
