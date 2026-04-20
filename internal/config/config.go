package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port int `yaml:"port"`
}

type Sub2APIConfig struct {
	BaseURL        string `yaml:"base_url"`
	AdminToken     string `yaml:"admin_token"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type LiteLLMConfig struct {
	RemoteURL    string `yaml:"remote_url"`
	FallbackFile string `yaml:"fallback_file"`
}

type SiteConfig struct {
	Name      string `yaml:"name"`
	Domain    string `yaml:"domain"`
	Currency  string `yaml:"currency"`
	PriceUnit string `yaml:"price_unit"`
}

type PricingConfig struct {
	PriceDecimals int `yaml:"price_decimals"`
}

type CacheConfig struct {
	TTLSeconds int `yaml:"ttl_seconds"`
}

type Config struct {
	Server        ServerConfig  `yaml:"server"`
	Sub2API       Sub2APIConfig `yaml:"sub2api"`
	LiteLLM       LiteLLMConfig `yaml:"litellm"`
	Site          SiteConfig    `yaml:"site"`
	Pricing       PricingConfig `yaml:"pricing"`
	Cache         CacheConfig   `yaml:"cache"`
	IncludeGroups []string      `yaml:"include_groups"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	expanded := os.ExpandEnv(string(raw))
	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyEnvOverrides(cfg)
	applyDefaults(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SUB2API_ADMIN_TOKEN"); v != "" {
		cfg.Sub2API.AdminToken = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8085
	}
	if cfg.Sub2API.TimeoutSeconds == 0 {
		cfg.Sub2API.TimeoutSeconds = 10
	}
	if cfg.Pricing.PriceDecimals == 0 {
		cfg.Pricing.PriceDecimals = 4
	}
	if cfg.Cache.TTLSeconds == 0 {
		cfg.Cache.TTLSeconds = 600
	}
	if cfg.Site.Currency == "" {
		cfg.Site.Currency = "CNY"
	}
	if cfg.Site.PriceUnit == "" {
		cfg.Site.PriceUnit = "per_1m_tokens"
	}
}
