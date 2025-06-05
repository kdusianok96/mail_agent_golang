package config

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds the application configuration values
type Config struct {
	ListenInterface string `toml:"ListenInterface"`
	ListenPort      int    `toml:"ListenPort"`
	ServerHostname  string `toml:"ServerHostname"`
	MailDir         string `toml:"MailDir"`

	// Incoming TLS
	TLSCertPath string `toml:"TLSCertPath"`
	TLSKeyPath  string `toml:"TLSKeyPath"`

	// Incoming SMTP AUTH
	Users       map[string]string `toml:"Users"`
	RequireAuth bool              `toml:"RequireAuth"`

	// Queue & Delivery
	QueueScanIntervalStr         string `toml:"QueueScanInterval"`
	DefaultRetryIntervalStr      string `toml:"DefaultRetryInterval"`
	MaxRetryIntervalStr          string `toml:"MaxRetryInterval"`
	MaxDeliveryAttempts          int    `toml:"MaxDeliveryAttempts"`
	QueueScanIntervalDuration    time.Duration `toml:"-"`
	DefaultRetryIntervalDuration time.Duration `toml:"-"`
	MaxRetryIntervalDuration     time.Duration `toml:"-"`

	// DKIM Signing
	DKIMEnable         bool     `toml:"DKIMEnable"`
	DKIMDomain         string   `toml:"DKIMDomain"`
	DKIMSelector       string   `toml:"DKIMSelector"`
	DKIMPrivateKeyPath string   `toml:"DKIMPrivateKeyPath"`
	DKIMHeaders        []string `toml:"DKIMHeaders,omitempty"`

	// Rate Limiting (Incoming)
	RateLimitEnable         bool `toml:"RateLimitEnable"`
	MaxConnectionsPerIP     int  `toml:"MaxConnectionsPerIP"`
	MaxCommandsPerSession   int  `toml:"MaxCommandsPerSession"`
	MaxRecipientsPerMessage int  `toml:"MaxRecipientsPerMessage"`

	// Outbound Email Security (Direct MX and Relay)
	OutboundSTARTTLSPolicy string `toml:"OutboundSTARTTLSPolicy"` // "opportunistic", "mandatory", "disabled"
	OutboundTLSVerifyCert  bool   `toml:"OutboundTLSVerifyCert"`  // Default to true via newConfigWithDefaults

	// Outbound Relay (Smarthost) Configuration
	OutboundRelayHost     string `toml:"OutboundRelayHost"`
	OutboundRelayUsername string `toml:"OutboundRelayUsername"`
	OutboundRelayPassword string `toml:"OutboundRelayPassword"`

	// Incoming Email Filtering
	IncomingFilterEnable            bool     `toml:"IncomingFilterEnable"`
	IncomingFilterScriptPath        string   `toml:"IncomingFilterScriptPath"`
	IncomingFilterScriptTimeoutStr  string   `toml:"IncomingFilterScriptTimeout"`
	IncomingFilterScriptArgs        []string `toml:"IncomingFilterScriptArgs,omitempty"`
	IncomingFilterActionOnDetection string   `toml:"IncomingFilterActionOnDetection"` // "reject", "add_header", "quarantine"
	IncomingFilterRejectMessage     string   `toml:"IncomingFilterRejectMessage"`
	IncomingFilterHeaderName        string   `toml:"IncomingFilterHeaderName"`
	IncomingFilterQuarantineDir     string   `toml:"IncomingFilterQuarantineDir"`
	IncomingFilterScriptTimeoutDuration time.Duration `toml:"-"`

	// Logging
	LogFilePath string `toml:"LogFilePath"` // Empty or "-" means stdout, otherwise path to log file.
}

// newConfigWithDefaults creates a Config with default values set before TOML unmarshalling.
// This is useful for booleans that should default to true if missing from TOML.
func newConfigWithDefaults() Config {
	return Config{
		OutboundTLSVerifyCert: true, // Default to true
		// LogFilePath defaults to "" (empty string), which setupLogging will interpret as stdout.
	}
}

// LoadConfig reads a TOML configuration file and unmarshals it into a Config struct.
// It also parses duration strings and sets defaults for various options.
func LoadConfig(filePath string) (*Config, error) {
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	cfg := newConfigWithDefaults() // Initialize with defaults
	if _, err := toml.Decode(string(configFile), &cfg); err != nil {
		return nil, err
	}

	// --- Queue & Delivery Durations ---
	if cfg.QueueScanIntervalStr == "" {
		cfg.QueueScanIntervalStr = "30s"; log.Printf("WARN: QueueScanInterval not set, using default: %s", cfg.QueueScanIntervalStr)
	}
	cfg.QueueScanIntervalDuration, err = time.ParseDuration(cfg.QueueScanIntervalStr)
	if err != nil {
		log.Printf("WARN: Failed to parse QueueScanInterval '%s': %v. Using default 30s.", cfg.QueueScanIntervalStr, err)
		cfg.QueueScanIntervalDuration = 30 * time.Second
	}

	if cfg.DefaultRetryIntervalStr == "" {
		cfg.DefaultRetryIntervalStr = "5m"; log.Printf("WARN: DefaultRetryInterval not set, using default: %s", cfg.DefaultRetryIntervalStr)
	}
	cfg.DefaultRetryIntervalDuration, err = time.ParseDuration(cfg.DefaultRetryIntervalStr)
	if err != nil {
		log.Printf("WARN: Failed to parse DefaultRetryInterval '%s': %v. Using default 5m.", cfg.DefaultRetryIntervalStr, err)
		cfg.DefaultRetryIntervalDuration = 5 * time.Minute
	}

	if cfg.MaxRetryIntervalStr == "" {
		cfg.MaxRetryIntervalStr = "12h"; log.Printf("WARN: MaxRetryInterval not set, using default: %s", cfg.MaxRetryIntervalStr)
	}
	cfg.MaxRetryIntervalDuration, err = time.ParseDuration(cfg.MaxRetryIntervalStr)
	if err != nil {
		log.Printf("WARN: Failed to parse MaxRetryInterval '%s': %v. Using default 12h.", cfg.MaxRetryIntervalStr, err)
		cfg.MaxRetryIntervalDuration = 12 * time.Hour
	}
	if cfg.DefaultRetryIntervalDuration > cfg.MaxRetryIntervalDuration {
		log.Printf("WARN: DefaultRetryInterval (%s) > MaxRetryInterval (%s). Setting Default to Max.", cfg.DefaultRetryIntervalDuration, cfg.MaxRetryIntervalDuration)
		cfg.DefaultRetryIntervalDuration = cfg.MaxRetryIntervalDuration
	}
	if cfg.MaxDeliveryAttempts <= 0 {
		log.Printf("WARN: MaxDeliveryAttempts not set or invalid, using default: 5.")
		cfg.MaxDeliveryAttempts = 5
	}

	// --- DKIM ---
	if cfg.DKIMEnable {
		if cfg.DKIMDomain == "" { log.Printf("WARN: DKIMEnable is true, but DKIMDomain is not set.") }
		if cfg.DKIMSelector == "" { cfg.DKIMSelector = "default"; log.Printf("WARN: DKIMSelector not set with DKIMEnable, using default: '%s'", cfg.DKIMSelector) }
		if cfg.DKIMPrivateKeyPath == "" { log.Printf("WARN: DKIMEnable is true, but DKIMPrivateKeyPath is not set.") }
	}

	// --- Rate Limiting ---
	if cfg.RateLimitEnable {
		if cfg.MaxConnectionsPerIP < 0 { cfg.MaxConnectionsPerIP = 0; log.Printf("WARN: MaxConnectionsPerIP negative, defaulting to 0 (unlimited).") }
		if cfg.MaxCommandsPerSession < 0 { cfg.MaxCommandsPerSession = 0; log.Printf("WARN: MaxCommandsPerSession negative, defaulting to 0 (unlimited).") }
		if cfg.MaxRecipientsPerMessage < 0 { cfg.MaxRecipientsPerMessage = 0; log.Printf("WARN: MaxRecipientsPerMessage negative, defaulting to 0 (unlimited).") }
	} else {
		if cfg.MaxConnectionsPerIP != 0 || cfg.MaxCommandsPerSession != 0 || cfg.MaxRecipientsPerMessage != 0 {
			log.Printf("INFO: RateLimitEnable is false. Settings for MaxConnectionsPerIP, MaxCommandsPerSession, MaxRecipientsPerMessage will be ignored.")
		}
	}

	// --- Outbound Security ---
	policy := strings.ToLower(cfg.OutboundSTARTTLSPolicy)
	switch policy {
	case "opportunistic", "mandatory", "disabled": cfg.OutboundSTARTTLSPolicy = policy
	case "": cfg.OutboundSTARTTLSPolicy = "opportunistic"; log.Printf("WARN: OutboundSTARTTLSPolicy not set, using default: 'opportunistic'.")
	default: cfg.OutboundSTARTTLSPolicy = "opportunistic"; log.Printf("WARN: Invalid OutboundSTARTTLSPolicy '%s', using default: 'opportunistic'.", policy)
	}
	// OutboundTLSVerifyCert default is true via newConfigWithDefaults()

	// --- Outbound Relay ---
	if cfg.OutboundRelayHost != "" {
		log.Printf("INFO: OutboundRelayHost configured (%s). Outgoing mail will use this relay.", cfg.OutboundRelayHost)
		if cfg.OutboundRelayUsername != "" && cfg.OutboundRelayPassword == "" {
			log.Printf("WARN: OutboundRelayUsername (%s) set, but OutboundRelayPassword is empty.", cfg.OutboundRelayUsername)
		}
	}

	// --- Incoming Filter Configuration ---
	if cfg.IncomingFilterEnable {
		if cfg.IncomingFilterScriptPath == "" {
			log.Printf("WARN: IncomingFilterEnable is true, but IncomingFilterScriptPath is empty. Filtering will effectively be disabled.")
		}

		if cfg.IncomingFilterScriptTimeoutStr == "" {
			cfg.IncomingFilterScriptTimeoutStr = "30s" // Default value
			log.Printf("WARN: IncomingFilterScriptTimeout not set, using default: %s", cfg.IncomingFilterScriptTimeoutStr)
		}
		parsedFilterTimeout, err := time.ParseDuration(cfg.IncomingFilterScriptTimeoutStr)
		if err != nil {
			log.Printf("WARN: Failed to parse IncomingFilterScriptTimeout '%s': %v. Using default 30s.", cfg.IncomingFilterScriptTimeoutStr, err)
			parsedFilterTimeout = 30 * time.Second
		}
		cfg.IncomingFilterScriptTimeoutDuration = parsedFilterTimeout

		action := strings.ToLower(cfg.IncomingFilterActionOnDetection)
		switch action {
		case "reject", "add_header", "quarantine":
			cfg.IncomingFilterActionOnDetection = action
		case "":
			cfg.IncomingFilterActionOnDetection = "add_header"; log.Printf("WARN: IncomingFilterActionOnDetection not set, using default: 'add_header'.")
		default:
			cfg.IncomingFilterActionOnDetection = "add_header"; log.Printf("WARN: Invalid IncomingFilterActionOnDetection '%s', using default: 'add_header'.", action)
		}

		if cfg.IncomingFilterActionOnDetection == "reject" && cfg.IncomingFilterRejectMessage == "" {
			cfg.IncomingFilterRejectMessage = "554 5.7.1 Message content rejected due to policy."
			log.Printf("WARN: IncomingFilterRejectMessage not set for 'reject' action, using default: '%s'", cfg.IncomingFilterRejectMessage)
		}
		if cfg.IncomingFilterActionOnDetection == "add_header" && cfg.IncomingFilterHeaderName == "" {
			cfg.IncomingFilterHeaderName = "X-GoSMTPServer-Scan-Result"
			log.Printf("WARN: IncomingFilterHeaderName not set for 'add_header' action, using default: '%s'", cfg.IncomingFilterHeaderName)
		}
		if cfg.IncomingFilterActionOnDetection == "quarantine" && cfg.IncomingFilterQuarantineDir == "" {
			log.Printf("ERROR: IncomingFilterActionOnDetection is 'quarantine', but IncomingFilterQuarantineDir is not set. Quarantine action will likely fail or be disabled.")
		}
	}
	// LogFilePath is loaded as a string. Its default interpretation (empty string for stdout)
	// is handled by the setupLogging function.

	return &cfg, nil
}
