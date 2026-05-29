package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Port      int    `mapstructure:"port"`
	LogLevel  string `mapstructure:"log_level"`
	DBPath    string `mapstructure:"db_path"`
	DataDir   string `mapstructure:"data_dir"`
	JwtSecret string `mapstructure:"jwt_secret"`
}

func generateSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	viper.SetDefault("port", 8080)
	viper.SetDefault("log_level", "info")
	viper.SetDefault("db_path", "./minipanel.db")
	viper.SetDefault("data_dir", "./data")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.JwtSecret == "" {
		if v := os.Getenv("MINIPANEL_JWT_SECRET"); v != "" {
			cfg.JwtSecret = v
		} else {
			cfg.JwtSecret = generateSecret()
		}
	}

	return &cfg, nil
}
