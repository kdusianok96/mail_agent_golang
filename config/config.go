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
}

// LoadConfig reads a TOML configuration file and unmarshals it into a Config struct.
func LoadConfig(filePath string) (*Config, error) {
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config Config
	if _, err := toml.Decode(string(configFile), &config); err != nil {
		return nil, err
	}

	// Set defaults if values are not in the config file (optional, but good practice)
	// For this exercise, we assume the config file is complete as per requirements.
	// Example:
	// if config.ListenInterface == "" {
	// 	config.ListenInterface = "0.0.0.0"
	// }
	// if config.ListenPort == 0 {
	// 	config.ListenPort = 2525
	// }

	return &config, nil
}
