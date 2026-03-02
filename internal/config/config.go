package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type StorageConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Storage: StorageConfig{
			SQLitePath: "./awayteam.db",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return cfg, err
			}
		} else if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}

	// Environment variables override config file
	if v := os.Getenv("AWAYTEAM_PORT"); v != "" {
		var port int
		if _, err := fmt.Sscanf(v, "%d", &port); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("AWAYTEAM_DB_PATH"); v != "" {
		cfg.Storage.SQLitePath = v
	}

	return cfg, nil
}
