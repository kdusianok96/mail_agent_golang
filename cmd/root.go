package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath" // For ensuring log directory exists

	"github.com/spf13/cobra"
)

var (
	// cfgFile is the path to the configuration file
	cfgFile string
	// logFileHandle is a global handle to the log file, so we can attempt to close it on exit.
	// This is a simple approach; more robust logging might use a dedicated logging package with its own management.
	logFileHandle *os.File
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "gosmtp",
	Short: "A simple SMTP server written in Go.",
	Long: `GoSMTP is a lightweight, configurable SMTP server designed for development,
testing, or small-scale email receiving applications.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// It's generally better to set up logging once the config is loaded,
	// as the config might specify the log path.
	// However, cobra's PersistentPreRunE could be a place if we want logging
	// before any command's Run executes, but after flags are parsed.
	// For now, setupLogging will be called explicitly by commands that need it (e.g. start).

	if err := RootCmd.Execute(); err != nil {
		// At this point, logging might already be redirected.
		// If Execute fails very early, it might go to stderr.
		// If it fails after logging setup, it goes to the configured log output.
		log.Printf("ERROR: Error executing root command: %v", err) // Use log package
		os.Exit(1)
	}

	// Attempt to close the log file if it was opened.
	if logFileHandle != nil {
		log.Println("INFO: Closing log file.")
		logFileHandle.Close()
	}
}

// setupLogging configures the log output based on the provided path.
// If logPath is empty or "-", logs go to os.Stdout. Otherwise, to the specified file.
func setupLogging(logPath string) {
	if logPath != "" && logPath != "-" {
		// Ensure the directory for the log file exists.
		dir := filepath.Dir(logPath)
		if dir != "." && dir != "" { // Check if a directory part exists
			if err := os.MkdirAll(dir, 0750); err != nil {
				log.Fatalf("Failed to create log directory %s: %v. Falling back to stdout.", dir, err)
			}
		}

		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			// Use log.Printf which will go to current log output (stderr if this fails early)
			log.Printf("WARN: Failed to open log file %s: %v. Falling back to stdout.", logPath, err)
			log.SetOutput(os.Stdout) // Explicitly set back to stdout
			log.SetFlags(log.LstdFlags | log.Lmicroseconds)
			log.Println("INFO: Logging continues on standard output.")
		} else {
			log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile) // Add file/line for file logs
			log.SetOutput(file)
			logFileHandle = file // Store for potential closing
			log.Printf("INFO: Logging initialized. Server logs will be written to: %s", logPath)
		}
	} else {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		log.SetOutput(os.Stdout)
		log.Println("INFO: Logging initialized. Server logs will be written to standard output.")
	}
}


func init() {
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "config.toml", "Configuration file path")
	// No cobra.OnInitialize(initConfig) here, as config loading and logging setup
	// will be handled by individual commands (like startCmd) that need the full config.
}
