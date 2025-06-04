package cmd

import (
	// "fmt" // Using log for all output
	"log"

	"go-smtp/config"
	"github.com/spf13/cobra"
)

// validateConfigCmd represents the validate-config command
var validateConfigCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validates the configuration file",
	Long:  `Parses the specified configuration file and checks for basic validity. Prints the loaded configuration if successful.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("INFO: Validating configuration file: %s", cfgFile)

		appConfig, err := config.LoadConfig(cfgFile)
		if err != nil {
			log.Fatalf("FATAL: Configuration file %s is invalid: %v", cfgFile, err)
		}

		log.Printf("INFO: Configuration file %s is valid.", cfgFile)
		log.Println("INFO: Loaded configuration values:")
		log.Printf("  Listen Interface: %s", appConfig.ListenInterface)
		log.Printf("  Listen Port: %d", appConfig.ListenPort)
		log.Printf("  Server Hostname: %s", appConfig.ServerHostname)
		log.Printf("  Mail Directory: %s", appConfig.MailDir)
		log.Printf("  TLS Cert Path: %s", appConfig.TLSCertPath)
		log.Printf("  TLS Key Path: %s", appConfig.TLSKeyPath)
		log.Printf("  Require Auth: %t", appConfig.RequireAuth)
		log.Printf("  Queue Scan Interval: %s (parsed: %s)", appConfig.QueueScanIntervalStr, appConfig.QueueScanIntervalDuration.String())
		log.Printf("  Default Retry Interval: %s (parsed: %s)", appConfig.DefaultRetryIntervalStr, appConfig.DefaultRetryIntervalDuration.String())
		log.Printf("  Max Retry Interval: %s (parsed: %s)", appConfig.MaxRetryIntervalStr, appConfig.MaxRetryIntervalDuration.String())
		log.Printf("  Max Delivery Attempts: %d", appConfig.MaxDeliveryAttempts)
		log.Println("  --- DKIM Configuration ---")
		log.Printf("  DKIM Enable: %t", appConfig.DKIMEnable)
		log.Printf("  DKIM Domain: %s", appConfig.DKIMDomain)
		log.Printf("  DKIM Selector: %s", appConfig.DKIMSelector)
		log.Printf("  DKIM Private Key Path: %s", appConfig.DKIMPrivateKeyPath)
		log.Printf("  DKIM Headers: %v", appConfig.DKIMHeaders)
		log.Println("  --- Outbound Email Security ---")
		log.Printf("  Outbound STARTTLS Policy: %s", appConfig.OutboundSTARTTLSPolicy)
		log.Printf("  Outbound TLS Verify Cert: %t", appConfig.OutboundTLSVerifyCert)
		log.Println("  --- Outbound Relay (Smarthost) ---")
		log.Printf("  Outbound Relay Host: %s", appConfig.OutboundRelayHost)
		log.Printf("  Outbound Relay Username: %s", appConfig.OutboundRelayUsername)
		if appConfig.OutboundRelayPassword != "" {
			log.Println("  Outbound Relay Password: [set]")
		} else {
			log.Println("  Outbound Relay Password: [not set]")
		}
		log.Println("  --- Basic Rate Limiting ---")
		log.Printf("  Rate Limiting Enabled: %t", appConfig.RateLimitEnable)
		log.Printf("  Max Connections Per IP: %d", appConfig.MaxConnectionsPerIP)
		log.Printf("  Max Commands Per Session: %d", appConfig.MaxCommandsPerSession)
		log.Printf("  Max Recipients Per Message: %d", appConfig.MaxRecipientsPerMessage)


		if len(appConfig.Users) > 0 {
			userKeys := make([]string, 0, len(appConfig.Users))
			for k := range appConfig.Users {
				userKeys = append(userKeys, k)
			}
			log.Printf("  Loaded SMTP AUTH users: %v (Passwords are hashed and not displayed)", userKeys)
		} else {
			log.Println("  No SMTP AUTH users configured.")
		}
	},
}

func init() {
	RootCmd.AddCommand(validateConfigCmd)

	// You could add flags specific to validate-config if needed
	// For example, a flag to only validate syntax without printing values:
	// validateConfigCmd.Flags().Bool("syntax-only", false, "Only validate syntax, do not print values")
}
