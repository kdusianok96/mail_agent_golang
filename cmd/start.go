package cmd

import (
	// "fmt" // Using log for all output
	"log"
	"fmt" // For quick response
	"log"
	"net"

	"go-smtp/config"
	"go-smtp/queue" // Added for queue processor
	"go-smtp/ratelimit" // Added for connection rate limiting
	"go-smtp/smtp"

	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the SMTP server",
	Long:  `Starts the SMTP server, listening for incoming connections and processing emails based on the provided configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("INFO: Starting GoSMTP server...")

		// cfgFile is retrieved from RootCmd's persistent flags
		log.Printf("INFO: Loading configuration from: %s", cfgFile)
		appConfig, err := config.LoadConfig(cfgFile)
		if err != nil {
			log.Fatalf("FATAL: Failed to load configuration from %s: %v", cfgFile, err)
		}

		log.Println("INFO: Configuration loaded successfully.")
		log.Printf("  Listen Interface: %s", appConfig.ListenInterface)
		log.Printf("  Listen Port: %d", appConfig.ListenPort)
		log.Printf("  Server Hostname: %s", appConfig.ServerHostname)
		log.Printf("  Mail Directory: %s", appConfig.MailDir)


		listenAddr := fmt.Sprintf("%s:%d", appConfig.ListenInterface, appConfig.ListenPort)
		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			log.Fatalf("FATAL: Error listening on %s: %s", listenAddr, err.Error())
		}
		defer listener.Close()
		log.Printf("INFO: SMTP server listening on %s", listenAddr)

		// Start the queue processor in a new goroutine
		go queue.StartProcessor(appConfig)

		// Accept connections in the main goroutine (or this one)
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("ERROR: Error accepting connection: %s", err.Error())
				// Depending on the error, we might want to continue or break for certain errors
				// For now, just log and continue accepting
				// If listener.Accept() returns a non-recoverable error, the server might effectively stop.
				// For example, if the listener socket is closed.
				// Consider if this loop should break on certain errors.
				if opError, ok := err.(*net.OpError); ok && !opError.Temporary() {
					log.Fatalf("FATAL: Unrecoverable listener error: %v. Shutting down.", err)
				}
				continue
			}

			clientIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
			if err != nil {
				log.Printf("ERROR: Could not extract IP from RemoteAddr %s: %v", conn.RemoteAddr().String(), err)
				conn.Close()
				continue
			}

			if appConfig.RateLimitEnable && appConfig.MaxConnectionsPerIP > 0 {
				if !ratelimit.AllowConnection(clientIP, appConfig.MaxConnectionsPerIP) {
					log.Printf("REJECT: Connection from %s rejected: too many concurrent connections (limit: %d)", clientIP, appConfig.MaxConnectionsPerIP)
					// Best effort to send 421
					quickResponse := fmt.Sprintf("421 %s Service not available, too many connections from your IP. Please try again later.\r\n", appConfig.ServerHostname)
					_, _ = conn.Write([]byte(quickResponse))
					conn.Close()
					continue
				}
			}

			// Pass clientIP to HandleConnection
			go smtp.HandleConnection(conn, appConfig, clientIP)
		}
	},
}

func init() {
	RootCmd.AddCommand(startCmd)

	// Here you can define flags specific to the start command if needed
	// startCmd.Flags().StringP("port", "p", "2525", "Port to listen on")
}
