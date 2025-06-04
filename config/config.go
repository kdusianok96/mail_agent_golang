package config

import (
	"os"
	"github.com/BurntSushi/toml"
)

// Config holds the application configuration values
type Config struct {
	ListenInterface string `toml:"ListenInterface"`
	ListenPort      int    `toml:"ListenPort"`
	ServerHostname  string `toml:"ServerHostname"`
	MailDir         string `toml:"MailDir"`
	TLSCertPath     string `toml:"TLSCertPath"` // Path to TLS certificate file
	TLSKeyPath      string `toml:"TLSKeyPath"`  // Path to TLS private key file
	Users           map[string]string `toml:"Users"`   // Username to bcrypt hashed password
	RequireAuth     bool   `toml:"RequireAuth"` // If true, AUTH is required for MAIL FROM if users are configured and connection is secure

	QueueScanIntervalStr string `toml:"QueueScanInterval"`    // e.g., "30s", "1m"
	DefaultRetryIntervalStr string `toml:"DefaultRetryInterval"` // e.g., "5m", "1h"
	MaxDeliveryAttempts    int    `toml:"MaxDeliveryAttempts"`

	// Parsed values, not directly in TOML
	QueueScanIntervalDuration   time.Duration `toml:"-"`
	DefaultRetryIntervalDuration time.Duration `toml:"-"`
}

// LoadConfig reads a TOML configuration file and unmarshals it into a Config struct.
// It also parses duration strings and sets defaults.
func LoadConfig(filePath string) (*Config, error) {
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if _, err := toml.Decode(string(configFile), &cfg); err != nil {
		return nil, err
	}

	// Parse QueueScanIntervalStr
	if cfg.QueueScanIntervalStr == "" {
		cfg.QueueScanIntervalStr = "30s" // Default value if string is empty
		log.Printf("WARN: QueueScanInterval not set in config, using default: %s", cfg.QueueScanIntervalStr)
	}
	parsedScanInterval, err := time.ParseDuration(cfg.QueueScanIntervalStr)
	if err != nil {
		log.Printf("WARN: Failed to parse QueueScanInterval '%s': %v. Using default 30s.", cfg.QueueScanIntervalStr, err)
		parsedScanInterval = 30 * time.Second
	}
	cfg.QueueScanIntervalDuration = parsedScanInterval

	// Parse DefaultRetryIntervalStr
	if cfg.DefaultRetryIntervalStr == "" {
		cfg.DefaultRetryIntervalStr = "5m" // Default value if string is empty
		log.Printf("WARN: DefaultRetryInterval not set in config, using default: %s", cfg.DefaultRetryIntervalStr)
	}
	parsedRetryInterval, err := time.ParseDuration(cfg.DefaultRetryIntervalStr)
	if err != nil {
		log.Printf("WARN: Failed to parse DefaultRetryInterval '%s': %v. Using default 5m.", cfg.DefaultRetryIntervalStr, err)
		parsedRetryInterval = 5 * time.Minute
	}
	cfg.DefaultRetryIntervalDuration = parsedRetryInterval

	// Set default for MaxDeliveryAttempts
	if cfg.MaxDeliveryAttempts <= 0 {
		log.Printf("WARN: MaxDeliveryAttempts not set or invalid in config, using default: 5")
		cfg.MaxDeliveryAttempts = 5
	}

	return &cfg, nil
}
