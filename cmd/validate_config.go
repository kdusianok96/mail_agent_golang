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
	},
}

func init() {
	RootCmd.AddCommand(validateConfigCmd)

	// You could add flags specific to validate-config if needed
	// For example, a flag to only validate syntax without printing values:
	// validateConfigCmd.Flags().Bool("syntax-only", false, "Only validate syntax, do not print values")
}
