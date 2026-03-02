package config

import (
	"errors"
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
			SQLitePath: "./aid.db",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := defaults()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
