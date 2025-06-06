// internal/config/config.go
package config

import (
	"errors"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	LogLevel             string        `mapstructure:"LOG_LEVEL"`
	DBURL                string        `mapstructure:"DB_URL"`
	GithubToken          string        `mapstructure:"GITHUB_TOKEN"`
	ReposToSync          []string      `mapstructure:"REPOS_TO_SYNC"`
	SyncInterval         time.Duration `mapstructure:"SYNC_INTERVAL"`
	DefaultSyncSinceDate string        `mapstructure:"DEFAULT_SYNC_SINCE_DATE"`
	DefaultSyncSinceTime time.Time     `mapstructure:"-"`
}

// LoadConfig reads configuration from file and/or environment variables.
func LoadConfig() (*Config, error) {
	// Set default values
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("SYNC_INTERVAL", "1h")
	viper.SetDefault("DEFAULT_SYNC_SINCE_DATE", "2023-01-01T00:00:00Z")

	// Load from .env file if it exists
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	_ = viper.ReadInConfig() // Ignore error if file not found

	// Bind environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse DefaultSyncSinceDate
	parsedTime, err := time.Parse(time.RFC3339, cfg.DefaultSyncSinceDate)
	if err != nil {
		return nil, errors.New("DEFAULT_SYNC_SINCE_DATE must be in RFC3339 format (e.g. 2023-01-01T00:00:00Z)")
	}
	cfg.DefaultSyncSinceTime = parsedTime

	// Validate required fields
	if cfg.DBURL == "" {
		return nil, errors.New("DB_URL is a required configuration field")
	}
	if cfg.GithubToken == "" {
		return nil, errors.New("GITHUB_TOKEN is a required configuration field")
	}
	if len(cfg.ReposToSync) == 0 {
		return nil, errors.New("REPOS_TO_SYNC must contain at least one repository")
	}

	return &cfg, nil
}
