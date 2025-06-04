package delivery

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"sort"
	"strings"
	"time"
	"crypto/tls" // For outbound STARTTLS
	"encoding/base64" // For SMTP AUTH
	"bytes" // For AUTH PLAIN payload construction

	"go-smtp/config" // To access outbound TLS policies
	"strconv"        // For parsing SMTP codes
	"errors"         // For errors.As
)

const clientLogPrefix = "SMTP_CLIENT"

// SMTPError is a custom error type for SMTP-specific errors.
type SMTPError struct {
	Code        int    // The integer SMTP status code (e.g., 550, 421)
	Message     string // The full SMTP response message
	Err         error  // Underlying error, if any (e.g., for connection issues)
	IsPermanent bool   // True if it's a 5xx class error
	IsTemporary bool   // True if it's a 4xx class error
}

func (e *SMTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("SMTP error: code=%d, msg=\"%s\", underlying_err=%v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("SMTP error: code=%d, msg=\"%s\"", e.Code, e.Message)
}

// NewSMTPError creates a new SMTPError
func NewSMTPError(code int, message string, underlyingErr error) *SMTPError {
	return &SMTPError{
		Code:        code,
		Message:     message,
		Err:         underlyingErr,
		IsPermanent: code >= 500 && code < 600,
		IsTemporary: code >= 400 && code < 500,
	}
}

// Email holds the data for an email to be sent.
type Email struct {
	From     string
	To       []string // For now, client handles one recipient from this list
	Data     []byte   // Raw email content (headers + body), CRLF line endings expected
	Hostname string   // Hostname of our server, for EHLO
}

// SendEmail attempts to deliver the email, using a configured relay if specified,
// otherwise falling back to MX lookup.
// It now accepts config to guide outbound TLS and relay policies.
func SendEmail(email *Email, cfg *config.Config) error {
	if len(email.To) == 0 {
		return fmt.Errorf("no recipients specified")
	}

	// For simplicity, we still focus on the first recipient for logging/domain extraction,
	// even though the email might be sent to a relay that handles all recipients.
	recipientForLog := email.To[0]

	// --- Relay Logic ---
	if cfg.OutboundRelayHost != "" {
		log.Printf("INFO: %s: OutboundRelayHost (%s) is configured. Bypassing MX lookup and connecting directly.", clientLogPrefix, cfg.OutboundRelayHost)

		targetHost, _, err := net.SplitHostPort(cfg.OutboundRelayHost)
		if err != nil {
			// If SplitHostPort fails, it might be that no port was specified (e.g. "relay.example.com")
			// In this case, assume port 25. net.Dial will need a port.
			// However, OutboundRelayHost is expected to be host:port.
			log.Printf("ERROR: %s: Invalid OutboundRelayHost format (%s): %v. Expected host:port.", clientLogPrefix, cfg.OutboundRelayHost, err)
			return fmt.Errorf("invalid OutboundRelayHost format '%s': %w", cfg.OutboundRelayHost, err)
		}
		// If no port was in OutboundRelayHost, SplitHostPort sets port to "" and targetHost is the full string.
		// The Dial below needs host:port. For relay, port is usually explicit.

		log.Printf("INFO: %s: Attempting to connect to relay host %s", clientLogPrefix, cfg.OutboundRelayHost)
		conn, err := net.DialTimeout("tcp", cfg.OutboundRelayHost, 20*time.Second)
		if err != nil {
			log.Printf("ERROR: %s: Failed to connect to relay host %s: %v", clientLogPrefix, cfg.OutboundRelayHost, err)
			return fmt.Errorf("failed to connect to relay host %s: %w", cfg.OutboundRelayHost, err)
		}

		// Use the extracted hostname (without port) for logging and potentially SNI via performSMTPTransaction's mxHost param.
		// If SplitHostPort failed to find a port, targetHost will be the full OutboundRelayHost.
		// It's better if performSMTPTransaction gets the host part for SNI/logging.
		hostnameForTransaction, _, err := net.SplitHostPort(cfg.OutboundRelayHost)
		if err != nil { // Should not happen if the first SplitHostPort succeeded and format is host:port
			hostnameForTransaction = cfg.OutboundRelayHost // Fallback, though less ideal
		}


		log.Printf("INFO: %s: Connected to relay %s. Performing SMTP transaction for recipient %s (and potentially others).", clientLogPrefix, hostnameForTransaction, recipientForLog)
		// When relaying, all recipients in email.To are handled by the relay server after a single successful transaction.
		// We still pass recipientForLog for consistency in performSMTPTransaction's logging if it singles out one.
		err = performSMTPTransaction(conn, email, recipientForLog, hostnameForTransaction, cfg)
		if err != nil {
			log.Printf("ERROR: %s: SMTP transaction with relay %s failed: %v", clientLogPrefix, hostnameForTransaction, err)
			return err // Return the error from the transaction
		}
		log.Printf("INFO: %s: Email successfully relayed via %s for recipients %v.", clientLogPrefix, hostnameForTransaction, email.To)
		return nil // Success
	}

	// --- MX Lookup Logic (if no relay host is configured) ---
	log.Printf("INFO: %s: No OutboundRelayHost configured. Proceeding with MX lookup for direct delivery.", clientLogPrefix)
	recipient := email.To[0] // For domain extraction for MX lookup
	parts := strings.Split(recipient, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid recipient address for MX lookup: %s", recipient)
	}

	// For simplicity in this initial client, we'll use the first recipient
	// to determine the destination domain.
	recipient := email.To[0]
	parts := strings.Split(recipient, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid recipient address: %s", recipient)
	}
	domain := parts[1]

	log.Printf("INFO: %s: Initiating delivery for email from %s to %s (domain: %s)", clientLogPrefix, email.From, recipient, domain)

	log.Printf("INFO: %s: Attempting MX lookup for domain %s", clientLogPrefix, domain)
	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		log.Printf("ERROR: %s: MX lookup failed for %s: %v", clientLogPrefix, domain, err)
		return fmt.Errorf("MX lookup failed for %s: %w", domain, err)
	}
	if len(mxRecords) == 0 {
		log.Printf("WARN: %s: No MX records found for domain %s", clientLogPrefix, domain)
		return fmt.Errorf("no MX records found for %s", domain)
	}
    // Log found MX records
    mxHosts := make([]string, len(mxRecords))
    for i, r := range mxRecords {
        mxHosts[i] = fmt.Sprintf("%s (Pref: %d)", r.Host, r.Pref)
    }
    log.Printf("INFO: %s: Found MX records for %s: %v", clientLogPrefix, domain, mxHosts)


	// Sort MX records by preference
	sort.Slice(mxRecords, func(i, j int) bool {
		return mxRecords[i].Pref < mxRecords[j].Pref
	})

	var lastErr error
	for _, mx := range mxRecords {
		targetHost := strings.TrimRight(mx.Host, ".") // Ensure host doesn't end with a dot for Dial
		log.Printf("INFO: %s: Attempting to connect to mail server %s (Pref: %d) for domain %s on port 25", clientLogPrefix, targetHost, mx.Pref, domain)

		conn, err := net.DialTimeout("tcp", targetHost+":25", 20*time.Second)
		if err != nil {
			log.Printf("WARN: %s: Failed to connect to %s: %v", clientLogPrefix, targetHost, err)
			lastErr = err
			continue
		}

		log.Printf("INFO: %s: Connected to %s. Performing SMTP transaction.", clientLogPrefix, targetHost)
		err = performSMTPTransaction(conn, email, recipient, targetHost, cfg) // Pass cfg
		if err != nil {
			log.Printf("WARN: %s: SMTP transaction failed with %s: %v", clientLogPrefix, targetHost, err)
			lastErr = err
			continue
		}

		log.Printf("INFO: %s: Email successfully delivered via MX host %s for recipient %s", clientLogPrefix, targetHost, recipient)
		return nil
	}

	log.Printf("ERROR: %s: Failed to deliver email to %s after trying all MX records.", clientLogPrefix, recipient)
	if lastErr != nil {
		return fmt.Errorf("failed to connect to any mail server for %s: %w", domain, lastErr)
	}
	return fmt.Errorf("failed to deliver email to %s (no specific connection error, check logs)", recipient)
}

// sendCommandAndReadResponse sends a command and expects a response starting with expectedCodePrefix.
func sendCommandAndReadResponse(writer *bufio.Writer, reader *bufio.Reader, mxHost, command string, expectedCodePrefix string) (string, error) {
	log.Printf("DEBUG: %s: [OUTBOUND %s] > %s", clientLogPrefix, mxHost, command)
	_, err := writer.WriteString(command + "\r\n")
	if err != nil {
		return "", fmt.Errorf("error writing command %s to %s: %w", command, mxHost, err)
	}
	err = writer.Flush()
	if err != nil {
		return "", fmt.Errorf("error flushing command %s to %s: %w", command, mxHost, err)
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading response for command %s from %s: %w", command, mxHost, err)
	}
	response = strings.TrimSpace(response)
	log.Printf("DEBUG: %s: [OUTBOUND %s] < %s", clientLogPrefix, mxHost, response)

	if len(response) < 3 {
		return response, NewSMTPError(0, response, fmt.Errorf("response too short"))
	}
	code, convErr := strconv.Atoi(response[0:3])
	if convErr != nil {
		return response, NewSMTPError(0, response, fmt.Errorf("failed to parse SMTP code: %w", convErr))
	}

	if expectedCodePrefix != "" && !strings.HasPrefix(response, expectedCodePrefix) {
		// Not the expected success code, return it as an SMTPError
		return response, NewSMTPError(code, response, fmt.Errorf("unexpected SMTP response"))
	}
	// If expectedCodePrefix is empty, any valid code is fine (e.g. for initial EHLO read)
	// but if we are here, it means no error was found by prefix check if one was provided.
	// If a command expects success (2xx, 3xx) and gets 4xx/5xx, it's an error.
	// This check is slightly simplified; specific commands expect specific success classes.
	if (strings.HasPrefix(expectedCodePrefix, "2") || strings.HasPrefix(expectedCodePrefix, "3")) && (code >= 400) {
		return response, NewSMTPError(code, response, fmt.Errorf("command failed with SMTP error"))
	}


	return response, nil
}

// performSMTPTransaction handles the SMTP conversation after a connection is established.
// It now accepts config to guide outbound TLS policies.
func performSMTPTransaction(conn net.Conn, email *Email, recipient string, mxHost string, cfg *config.Config) error {
	defer conn.Close()

	currentConn := conn // Use currentConn, which might be upgraded to TLS
	reader := bufio.NewReader(currentConn)
	writer := bufio.NewWriter(currentConn)
	var lastCommand string
	isTlsActive := false

	// 1. Read greeting
	greeting, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] Failed to read server greeting: %w", clientLogPrefix, mxHost, err)
	}
	greeting = strings.TrimSpace(greeting)
	log.Printf("DEBUG: %s: [OUTBOUND %s] < %s", clientLogPrefix, mxHost, greeting)
	if !strings.HasPrefix(greeting, "220") {
		return fmt.Errorf("%s: [OUTBOUND %s] Unexpected greeting: %s", clientLogPrefix, mxHost, greeting)
	}

	// 2. Initial EHLO and STARTTLS Logic
	lastCommand = fmt.Sprintf("EHLO %s", email.Hostname)
	ehloResponse, err := sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "") // Don't fail on non-250 yet, need to parse caps
	if err != nil {
		// If EHLO itself fails badly (e.g., I/O error), then we can't proceed.
		return fmt.Errorf("%s: [OUTBOUND %s] Initial EHLO command failed badly: %w", clientLogPrefix, mxHost, err)
	}
	if !strings.HasPrefix(ehloResponse, "250") {
		// Server might not support EHLO, or gave an error. Could try HELO. For now, strict.
		return fmt.Errorf("%s: [OUTBOUND %s] Initial EHLO response not 250: %s", clientLogPrefix, mxHost, ehloResponse)
	}

	serverSupportsSTARTTLS := strings.Contains(strings.ToUpper(ehloResponse), "STARTTLS") // Simplified check

	if cfg.OutboundSTARTTLSPolicy == "disabled" {
		log.Printf("INFO: %s: [OUTBOUND %s] Outbound STARTTLS is disabled by policy. Proceeding in plaintext.", clientLogPrefix, mxHost)
	} else { // opportunistic or mandatory
		if serverSupportsSTARTTLS {
			log.Printf("INFO: %s: [OUTBOUND %s] Server supports STARTTLS. Policy: '%s'. Attempting to upgrade.", clientLogPrefix, mxHost, cfg.OutboundSTARTTLSPolicy)
			lastCommand = "STARTTLS"
			starttlsResp, starttlsErr := sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "220")
			if starttlsErr != nil {
				errMessage := fmt.Sprintf("STARTTLS command failed or server responded incorrectly: %v (response: %s)", starttlsErr, starttlsResp)
				if cfg.OutboundSTARTTLSPolicy == "mandatory" {
					log.Printf("ERROR: %s: [OUTBOUND %s] %s Policy is mandatory. Aborting connection.", clientLogPrefix, mxHost, errMessage)
					return fmt.Errorf(errMessage)
				}
				log.Printf("WARN: %s: [OUTBOUND %s] %s Policy is opportunistic. Proceeding in plaintext.", clientLogPrefix, mxHost, errMessage)
			} else { // STARTTLS command successful (220 response)
				log.Printf("INFO: %s: [OUTBOUND %s] STARTTLS command accepted. Initiating TLS handshake.", clientLogPrefix, mxHost)
				tlsCfg := &tls.Config{
					ServerName:         mxHost, // For SNI and certificate validation
					InsecureSkipVerify: !cfg.OutboundTLSVerifyCert,
				}
				tlsClient := tls.Client(currentConn, tlsCfg)
				handshakeErr := tlsClient.Handshake()
				if handshakeErr != nil {
					errMessage := fmt.Sprintf("TLS handshake failed: %v", handshakeErr)
					if cfg.OutboundSTARTTLSPolicy == "mandatory" {
						log.Printf("ERROR: %s: [OUTBOUND %s] %s Policy is mandatory. Aborting connection.", clientLogPrefix, mxHost, errMessage)
						return fmt.Errorf(errMessage)
					}
					log.Printf("WARN: %s: [OUTBOUND %s] %s Policy is opportunistic. Proceeding in plaintext.", clientLogPrefix, mxHost, errMessage)
				} else {
					log.Printf("INFO: %s: [OUTBOUND %s] TLS handshake successful. Connection upgraded.", clientLogPrefix, mxHost)
					currentConn = tlsClient // Upgrade to TLS connection
					isTlsActive = true
					reader = bufio.NewReader(currentConn) // Re-init reader and writer
					writer = bufio.NewWriter(currentConn)

					// Re-issue EHLO over the secure channel
					log.Printf("INFO: %s: [OUTBOUND %s] Re-issuing EHLO over secure channel.", clientLogPrefix, mxHost)
					lastCommand = fmt.Sprintf("EHLO %s", email.Hostname) // Use original configured Hostname
					_, err = sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "250")
					if err != nil {
						// If re-EHLO fails, it's a significant issue for the session.
						return fmt.Errorf("%s: [OUTBOUND %s] EHLO command failed after STARTTLS: %w", clientLogPrefix, mxHost, err)
					}
					// Update ehloResponse with the new capabilities after STARTTLS
					ehloResponse, err = sendCommandAndReadResponse(writer, reader, mxHost, fmt.Sprintf("EHLO %s", email.Hostname), "250")
					if err != nil {
						return fmt.Errorf("%s: [OUTBOUND %s] Second EHLO command failed after STARTTLS: %w", clientLogPrefix, mxHost, err)
					}
				}
			}
		} else { // Server does not support STARTTLS
			if cfg.OutboundSTARTTLSPolicy == "mandatory" {
				errMsg := fmt.Sprintf("Server does not support STARTTLS and policy is mandatory. Aborting connection.")
				log.Printf("ERROR: %s: [OUTBOUND %s] %s", clientLogPrefix, mxHost, errMsg)
				return fmt.Errorf(errMsg)
			}
			log.Printf("INFO: %s: [OUTBOUND %s] Server does not support STARTTLS. Proceeding in plaintext (policy: %s).", clientLogPrefix, mxHost, cfg.OutboundSTARTTLSPolicy)
		}
	}
	// If TLS is active, all subsequent commands use the upgraded tlsConn via currentConn

	// 3. SMTP AUTH (if connected to relay, TLS is active, and credentials are provided)
	// Note: mxHost might be a general MX server or the specific relay host.
	// We should only attempt AUTH if mxHost matches cfg.OutboundRelayHost (or part of it if port is included).
	isRelayConnection := false
	if cfg.OutboundRelayHost != "" {
		relayHostOnly := strings.Split(cfg.OutboundRelayHost, ":")[0]
		if strings.EqualFold(mxHost, relayHostOnly) { // Case-insensitive compare for hostnames
			isRelayConnection = true
		}
	}

	if isRelayConnection && cfg.OutboundRelayUsername != "" {
		if !isTlsActive {
			log.Printf("WARN: %s: [OUTBOUND %s] Relay username configured, but connection is not secure via STARTTLS. Skipping AUTH.", clientLogPrefix, mxHost)
		} else {
			log.Printf("INFO: %s: [OUTBOUND %s] Attempting SMTP AUTH as relay user %s.", clientLogPrefix, mxHost, cfg.OutboundRelayUsername)
			serverAuthCaps := strings.ToUpper(ehloResponse) // Use capabilities from the latest EHLO
			authSuccessful := false

			if strings.Contains(serverAuthCaps, "AUTH PLAIN") {
				log.Printf("INFO: %s: [OUTBOUND %s] Relay supports AUTH PLAIN. Attempting.", clientLogPrefix, mxHost)
				// Format: \0username\0password
				authData := []byte(fmt.Sprintf("\x00%s\x00%s", cfg.OutboundRelayUsername, cfg.OutboundRelayPassword))
				authPayload := base64.StdEncoding.EncodeToString(authData)
				authCmd := fmt.Sprintf("AUTH PLAIN %s", authPayload)

				resp, authErr := sendCommandAndReadResponse(writer, reader, mxHost, authCmd, "235")
				if authErr == nil {
					log.Printf("INFO: %s: [OUTBOUND %s] AUTH PLAIN successful for user %s.", clientLogPrefix, mxHost, cfg.OutboundRelayUsername)
					authSuccessful = true
				} else {
					log.Printf("WARN: %s: [OUTBOUND %s] AUTH PLAIN failed for user %s: %v (Response: %s)", clientLogPrefix, mxHost, cfg.OutboundRelayUsername, authErr, resp)
					// Do not return yet, might try LOGIN if PLAIN fails and LOGIN is available
				}
			}

			if !authSuccessful && strings.Contains(serverAuthCaps, "AUTH LOGIN") {
				log.Printf("INFO: %s: [OUTBOUND %s] Relay supports AUTH LOGIN. Attempting.", clientLogPrefix, mxHost)
				authCmd := "AUTH LOGIN"
				resp, loginErr := sendCommandAndReadResponse(writer, reader, mxHost, authCmd, "334") // Expect "334 VXNlcm5hbWU6"
				if loginErr != nil {
					log.Printf("WARN: %s: [OUTBOUND %s] AUTH LOGIN initial command failed: %v (Response: %s)", clientLogPrefix, mxHost, loginErr, resp)
				} else {
					// Send username
					userB64 := base64.StdEncoding.EncodeToString([]byte(cfg.OutboundRelayUsername))
					resp, loginErr = sendCommandAndReadResponse(writer, reader, mxHost, userB64, "334") // Expect "334 UGFzc3dvcmQ6"
					if loginErr != nil {
						log.Printf("WARN: %s: [OUTBOUND %s] AUTH LOGIN username submission failed: %v (Response: %s)", clientLogPrefix, mxHost, loginErr, resp)
					} else {
						// Send password
						passB64 := base64.StdEncoding.EncodeToString([]byte(cfg.OutboundRelayPassword))
						resp, loginErr = sendCommandAndReadResponse(writer, reader, mxHost, passB64, "235")
						if loginErr == nil {
							log.Printf("INFO: %s: [OUTBOUND %s] AUTH LOGIN successful for user %s.", clientLogPrefix, mxHost, cfg.OutboundRelayUsername)
							authSuccessful = true
						} else {
							log.Printf("WARN: %s: [OUTBOUND %s] AUTH LOGIN password submission failed: %v (Response: %s)", clientLogPrefix, mxHost, loginErr, resp)
						}
					}
				}
			}

			if !authSuccessful {
				// If neither PLAIN nor LOGIN succeeded (or were available) and username was set
				errMsg := fmt.Sprintf("SMTP AUTH failed with relay %s. No supported/successful AUTH mechanism (PLAIN/LOGIN).", mxHost)
				log.Printf("ERROR: %s: [OUTBOUND %s] %s", clientLogPrefix, mxHost, errMsg)
				return fmt.Errorf(errMsg)
			}
		}
	} else if isRelayConnection && cfg.OutboundRelayUsername == "" {
		log.Printf("INFO: %s: [OUTBOUND %s] Connected to relay host, but no relay username configured. Proceeding without AUTH.", clientLogPrefix, mxHost)
	}


	// 4. MAIL FROM
	lastCommand = fmt.Sprintf("MAIL FROM:<%s>", email.From)
	_, err = sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "250")
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] MAIL FROM command failed: %w", clientLogPrefix, mxHost, err)
	}

	// 4. RCPT TO
	lastCommand = fmt.Sprintf("RCPT TO:<%s>", recipient)
	_, err = sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "250")
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] RCPT TO command for %s failed: %w", clientLogPrefix, mxHost, recipient, err)
	}

	// 5. DATA
	lastCommand = "DATA"
	_, err = sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "354")
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] DATA command failed: %w", clientLogPrefix, mxHost, err)
	}

	log.Printf("DEBUG: %s: [OUTBOUND %s] Sending email data via DotWriter.", clientLogPrefix, mxHost)
	dw := textproto.NewDotWriter(writer)
	_, err = dw.Write(email.Data)
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] Failed to write email data via DotWriter: %w", clientLogPrefix, mxHost, err)
	}
	err = dw.Close()
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] Failed to close DotWriter: %w", clientLogPrefix, mxHost, err)
	}
	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] Failed to flush after email data: %w", clientLogPrefix, mxHost, err)
	}
	log.Printf("DEBUG: %s: [OUTBOUND %s] > . (end of data)", clientLogPrefix, mxHost)

	dataResponse, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] Error reading response after DATA: %w", clientLogPrefix, mxHost, err)
	}
	dataResponse = strings.TrimSpace(dataResponse)
	log.Printf("DEBUG: %s: [OUTBOUND %s] < %s", clientLogPrefix, mxHost, dataResponse)
	if !strings.HasPrefix(dataResponse, "250") {
		return fmt.Errorf("%s: [OUTBOUND %s] Unexpected response after DATA: %s", clientLogPrefix, mxHost, dataResponse)
	}

	// 6. QUIT
	lastCommand = "QUIT"
	_, err = sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "221")
	if err != nil {
		log.Printf("WARN: %s: [OUTBOUND %s] QUIT command response was not 221 as expected: %v", clientLogPrefix, mxHost, err)
	}

	log.Printf("INFO: %s: [OUTBOUND %s] Email transaction successful for recipient %s.", clientLogPrefix, mxHost, recipient)
	return nil
	}

	return nil
}
