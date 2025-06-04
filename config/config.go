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
	MaxRetryIntervalDuration    time.Duration `toml:"-"` // Added for exponential backoff cap

	// DKIM Configuration
	DKIMEnable         bool     `toml:"DKIMEnable"`
	DKIMDomain         string   `toml:"DKIMDomain"`
	DKIMSelector       string   `toml:"DKIMSelector"`
	DKIMPrivateKeyPath string   `toml:"DKIMPrivateKeyPath"`
	DKIMHeaders        []string `toml:"DKIMHeaders,omitempty"`

	// Outbound Email Security
	OutboundSTARTTLSPolicy string `toml:"OutboundSTARTTLSPolicy"` // "opportunistic", "mandatory", "disabled"
	OutboundTLSVerifyCert  bool   `toml:"OutboundTLSVerifyCert"`  // Default to true

	// Outbound Relay (Smarthost) Configuration
	OutboundRelayHost         string `toml:"OutboundRelayHost"`     // e.g., "smtp.example.com:587"
	OutboundRelayUsername     string `toml:"OutboundRelayUsername"`
	OutboundRelayPassword     string `toml:"OutboundRelayPassword"`
	MaxRetryIntervalStr       string `toml:"MaxRetryInterval"`      // e.g., "12h", "24h"
	// OutboundRelayAuthMechanism string `toml:"OutboundRelayAuthMechanism"` // Future: "PLAIN", "LOGIN", "CRAM-MD5"
}

// newConfigWithDefaults creates a Config with default values set before TOML unmarshalling.
func newConfigWithDefaults() Config {
	return Config{
		OutboundTLSVerifyCert: true, // Default to true
		// Initialize other fields that need non-zero defaults if TOML lib doesn't set them when key is missing
	}
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
		log.Printf("WARN: MaxDeliveryAttempts not set or invalid in config, using default: 5.")
		cfg.MaxDeliveryAttempts = 5
	}

	// Set defaults for DKIM
	if cfg.DKIMEnable {
		if cfg.DKIMDomain == "" {
			log.Printf("WARN: DKIMEnable is true, but DKIMDomain is not set. DKIM signing will likely fail.")
			// DKIMDomain is critical, cannot really default.
		}
		if cfg.DKIMSelector == "" {
			cfg.DKIMSelector = "default"
			log.Printf("WARN: DKIMSelector not set in config while DKIMEnable is true, using default: '%s'", cfg.DKIMSelector)
		}
		if cfg.DKIMPrivateKeyPath == "" {
			log.Printf("WARN: DKIMEnable is true, but DKIMPrivateKeyPath is not set. DKIM signing will fail.")
		}
		// DKIMHeaders can often be left to a library's default.
		// If cfg.DKIMHeaders is nil (not just empty), it means it wasn't in the TOML.
		// If it's an empty list `[]`, it means the user explicitly set it to empty.
		// For now, we don't set default headers here; the signing library might have its own.
		// If specific default headers were required by our chosen library, this would be the place.
		// Example:
		// if cfg.DKIMHeaders == nil && cfg.DKIMEnable { // only default if not specified at all
		//    cfg.DKIMHeaders = []string{"From", "To", "Cc", "Subject", "Date", "Message-ID"}
		//    log.Printf("INFO: DKIMHeaders not set, using library defaults (or internal defaults if any).")
		// }
	}

	// Validate and set default for OutboundSTARTTLSPolicy
	policy := strings.ToLower(cfg.OutboundSTARTTLSPolicy)
	switch policy {
	case "opportunistic", "mandatory", "disabled":
		cfg.OutboundSTARTTLSPolicy = policy // Ensure it's stored in lowercase
	case "":
		log.Printf("WARN: OutboundSTARTTLSPolicy not set in config, using default: 'opportunistic'.")
		cfg.OutboundSTARTTLSPolicy = "opportunistic"
	default:
		log.Printf("WARN: Invalid OutboundSTARTTLSPolicy '%s' in config, using default: 'opportunistic'.", cfg.OutboundSTARTTLSPolicy)
		cfg.OutboundSTARTTLSPolicy = "opportunistic"
	}
	// Note: OutboundTLSVerifyCert default is handled by newConfigWithDefaults if key is missing.
	// If key `OutboundTLSVerifyCert = false` is present, it will be false.

	if cfg.OutboundRelayHost != "" {
		log.Printf("INFO: OutboundRelayHost is configured (%s). All outgoing mail will be sent via this relay.", cfg.OutboundRelayHost)
		if cfg.OutboundRelayUsername != "" && cfg.OutboundRelayPassword == "" {
			log.Printf("WARN: OutboundRelayUsername (%s) is set, but OutboundRelayPassword is empty. Relay authentication might fail.", cfg.OutboundRelayUsername)
		}
	}

	// Parse MaxRetryIntervalStr
	if cfg.MaxRetryIntervalStr == "" {
		cfg.MaxRetryIntervalStr = "12h" // Default value
		log.Printf("WARN: MaxRetryInterval not set in config, using default: %s", cfg.MaxRetryIntervalStr)
	}
	parsedMaxRetryInterval, err := time.ParseDuration(cfg.MaxRetryIntervalStr)
	if err != nil {
		log.Printf("WARN: Failed to parse MaxRetryInterval '%s': %v. Using default 12h.", cfg.MaxRetryIntervalStr, err)
		parsedMaxRetryInterval = 12 * time.Hour
	}
	cfg.MaxRetryIntervalDuration = parsedMaxRetryInterval

	// Ensure DefaultRetryInterval is not greater than MaxRetryInterval
	if cfg.DefaultRetryIntervalDuration > cfg.MaxRetryIntervalDuration {
		log.Printf("WARN: DefaultRetryInterval (%s) is greater than MaxRetryInterval (%s). Setting DefaultRetryInterval to MaxRetryInterval.",
			cfg.DefaultRetryIntervalDuration, cfg.MaxRetryIntervalDuration)
		cfg.DefaultRetryIntervalDuration = cfg.MaxRetryIntervalDuration
	}


	return &cfg, nil
}
