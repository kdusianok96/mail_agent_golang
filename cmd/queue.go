package cmd

import (
	"github.com/spf13/cobra"
)

// queueCmd represents the base command for queue management.
var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the mail queue",
	Long: `Provides subcommands to inspect and manage the GoSMTPServer mail queue.
Use this to view summaries, list messages, or potentially retry/delete messages in the future.`,
	// Run: func(cmd *cobra.Command, args []string) {
	//  // If called without a subcommand, you could print help or a default summary.
	//  // For now, let it require a subcommand.
	// },
}

func init() {
	RootCmd.AddCommand(queueCmd)
	// Subcommands like 'summary', 'list', 'retry', 'delete' will be added to queueCmd.
}
