package cmd

import (
	// "fmt" // Using log for all output
	"log"
	"context" // For graceful shutdown
	"fmt"     // For quick response
	"log"
	"net"
	"os"      // For os.Signal
	"os/signal" // For signal.Notify
	"sync"    // For sync.WaitGroup
	"syscall" // For syscall.SIGINT, syscall.SIGTERM
	"time"    // For shutdown timeout

	"go-smtp/config"
	"go-smtp/queue"     // Added for queue processor
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
		log.Println("INFO: Attempting to start GoSMTP server...")

		// --- Configuration Loading ---
		log.Printf("INFO: Loading configuration from: %s", cfgFile)
		appConfig, err := config.LoadConfig(cfgFile)
		if err != nil {
			log.Fatalf("FATAL: Failed to load configuration from %s: %v", cfgFile, err)
		}
		setupLogging(appConfig.LogFilePath) // Setup logging ASAP after config is loaded
		log.Println("INFO: Configuration loaded and logging initialized.")
		// Log key config details (already done in validate_config, but good for start log too)
		log.Printf("INFO: ServerHostname: %s, Listen: %s:%d, MailDir: %s",
			appConfig.ServerHostname, appConfig.ListenInterface, appConfig.ListenPort, appConfig.MailDir)


		// --- Graceful Shutdown Setup ---
		shutdownCtx, cancelSignal := context.WithCancel(context.Background())
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

		var wg sync.WaitGroup


		// --- Start Queue Processor ---
		wg.Add(1)
		go queue.StartProcessor(shutdownCtx, appConfig, &wg)


		// --- Start SMTP Listener ---
		listenAddr := fmt.Sprintf("%s:%d", appConfig.ListenInterface, appConfig.ListenPort)
		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			log.Fatalf("FATAL: Error listening on %s: %s", listenAddr, err.Error())
		}
		log.Printf("INFO: SMTP server listener started on %s", listener.Addr().String())

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer listener.Close() // Ensure listener is closed when this goroutine exits

			for {
				conn, err := listener.Accept()
				if err != nil {
					select {
					case <-shutdownCtx.Done(): // Shutdown signal received
						log.Printf("INFO: SMTP listener: Accept loop shutting down due to signal.")
						return // Exit goroutine
					default:
						// Genuine accept error
						log.Printf("ERROR: SMTP listener: Error accepting connection: %s", err.Error())
						if opErr, ok := err.(*net.OpError); ok && !opError.Temporary() {
							log.Printf("ERROR: SMTP listener: Non-temporary error: %v. Shutting down listener accept loop.", err)
							// This will cause the main startCmd.Run to proceed to shutdown if this was the only thing it was waiting for.
							// However, the signal handler is the primary shutdown mechanism.
							// We might want to trigger cancelSignal() here too if listener fails catastrophically.
							// For now, just stopping this goroutine.
							return
						}
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
						quickResponse := fmt.Sprintf("421 %s Service not available, too many connections from your IP. Please try again later.\r\n", appConfig.ServerHostname)
						_, _ = conn.Write([]byte(quickResponse))
						conn.Close()
						continue
					}
				}
				// Note: Active SMTP sessions (HandleConnection) are not currently added to the WaitGroup.
				// For a more complete graceful shutdown, each HandleConnection would Add(1) and Done().
				go smtp.HandleConnection(conn, appConfig, clientIP)
			}
		}()

		// --- Wait for Shutdown Signal ---
		sig := <-signalChan
		log.Printf("INFO: Received signal: %s. Initiating graceful shutdown...", sig)

		cancelSignal() // Signal all dependent goroutines to stop

		// --- Wait for Goroutines to Finish with Timeout ---
		waitTimeout := 30 * time.Second
		waitChan := make(chan struct{})
		go func() {
			log.Println("INFO: Waiting for services to shut down...")
			wg.Wait()
			close(waitChan)
		}()

		select {
		case <-waitChan:
			log.Println("INFO: All services shut down gracefully.")
		case <-time.After(waitTimeout):
			log.Println("WARN: Shutdown timed out waiting for services to stop. Forcing exit.")
		}

		// Global logFileHandle is defined in cmd/root.go
		if logFileHandle != nil {
			log.Println("INFO: Closing log file.")
			if err := logFileHandle.Close(); err != nil {
				// Log to stderr as log file might be problematic
				fmt.Fprintf(os.Stderr, "Error closing log file: %v\n", err)
			}
		}
		log.Println("INFO: Server shutdown complete.")
	},
}

func init() {
	RootCmd.AddCommand(startCmd)

	// Here you can define flags specific to the start command if needed
	// startCmd.Flags().StringP("port", "p", "2525", "Port to listen on")
}
