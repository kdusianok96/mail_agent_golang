package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"go-smtp/config" // Assuming your config package path
	"github.com/spf13/cobra"
)

// queueSummaryCmd represents the queue summary command
var queueSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Display a summary of the mail queue",
	Long:  `Scans the mail queue directory (and its 'failed' and 'corrupt' subdirectories) to count messages in each state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// cfgFile is the global variable from cmd/root.go for the config file path
		if cfgFile == "" {
			// This case should ideally be handled by cobra if the flag is marked as required,
			// or if a default is always valid. For safety, check if it's empty.
			return fmt.Errorf("configuration file path not provided or found")
		}

		appConfig, err := config.LoadConfig(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load configuration from %s: %w", cfgFile, err)
		}

		// Setup logging to respect LogFilePath from config for this command's output
		// If not, logs from countMetaFiles might go to stdout even if file is configured.
		// However, typical CLI commands output to stdout/stderr directly via fmt or log to stderr.
		// For simplicity, this command's primary output is fmt.Printf, and errors go to stderr via return.
		// Logs from countMetaFiles (if any) will use the default logger (stderr).

		fmt.Println("Mail Queue Summary:")

		activeCount, errActive := countMetaFiles(appConfig.MailDir, "*.meta")
		if errActive != nil {
			// Log to stderr for command output consistency
			log.Printf("Error counting active queue in %s: %v", appConfig.MailDir, errActive)
		}
		fmt.Printf("  Active Queue:    %d messages\n", activeCount)

		failedDirPath := filepath.Join(appConfig.MailDir, "failed")
		failedCount, errFailed := countMetaFiles(failedDirPath, "*.meta")
		if errFailed != nil && !os.IsNotExist(errFailed) { // Don't log error if dir just doesn't exist
			log.Printf("Error counting failed queue in %s: %v", failedDirPath, errFailed)
		}
		fmt.Printf("  Failed Directory:  %d messages\n", failedCount)

		corruptDirPath := filepath.Join(appConfig.MailDir, "corrupt")
		corruptCount, errCorrupt := countMetaFiles(corruptDirPath, "*.meta")
		if errCorrupt != nil && !os.IsNotExist(errCorrupt) { // Don't log error if dir just doesn't exist
			log.Printf("Error counting corrupt queue in %s: %v", corruptDirPath, errCorrupt)
		}
		fmt.Printf("  Corrupt Directory: %d messages\n", corruptCount)

		if errActive != nil || (errFailed != nil && !os.IsNotExist(errFailed)) || (errCorrupt != nil && !os.IsNotExist(errCorrupt)) {
			return fmt.Errorf("one or more errors occurred while summarizing the queue")
		}
		return nil
	},
}

// countMetaFiles counts files matching a pattern (e.g., "*.meta") in a given directory.
func countMetaFiles(directoryPath string, pattern string) (int, error) {
	// Check if directory exists first. Glob doesn't error on non-existent dir, returns empty list.
	if _, err := os.Stat(directoryPath); os.IsNotExist(err) {
		log.Printf("DEBUG: Directory %s does not exist, count is 0.", directoryPath)
		return 0, nil // Not an error for counting purposes if dir doesn't exist
	} else if err != nil {
		return 0, fmt.Errorf("error stating directory %s: %w", directoryPath, err)
	}

	globPattern := filepath.Join(directoryPath, pattern)
	files, err := filepath.Glob(globPattern)
	if err != nil {
		return 0, fmt.Errorf("error globbing %s: %w", globPattern, err)
	}
	return len(files), nil
}

func init() {
	queueCmd.AddCommand(queueSummaryCmd)
	// No specific flags for summary command itself yet. It uses the global --config.
}
