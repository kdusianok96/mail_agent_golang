package smtp

import (
	"bufio"
	"bytes"        // For splitting AUTH PLAIN payload
	"crypto/rand"
	"crypto/tls"   // Added for STARTTLS
	"encoding/base64" // Added for AUTH
	"encoding/hex"
	"fmt"
	"log" // Added for enhanced logging
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-smtp/config" // Added import for config
	"go-smtp/queue"  // Added for mail queue metadata
	"golang.org/x/crypto/bcrypt" // Added for password hashing
)

type sessionState struct {
	clientHostname    string
	hasSeenHelo       bool
	mailFromAddress   string
	rcptToAddresses   []string
	isInDataMode      bool           // Technically managed by the DATA command flow itself
	isTls             bool           // True if STARTTLS has been successfully negotiated
	isAuthenticated   bool           // True if SMTP AUTH was successful
	authenticatedUser string         // Username if authenticated
	cfg               *config.Config
}

func newSessionState(cfg *config.Config) *sessionState {
	return &sessionState{
		rcptToAddresses:   make([]string, 0),
		isTls:             false,
		isAuthenticated:   false,
		authenticatedUser: "",
		cfg:               cfg,
	}
}

func (s *sessionState) resetMailState() {
	s.mailFromAddress = ""
	s.rcptToAddresses = make([]string, 0)
	// Note: Authentication state is NOT reset here, only mail transaction state.
	// RSET command and successful STARTTLS will handle auth state reset.
}

func verifyCredentials(username, password string, cfgUsers map[string]string) bool {
	storedHash, userExists := cfgUsers[username]
	if !userExists {
		log.Printf("Auth attempt for non-existent user: %s", username)
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err == nil {
		return true // Password matches
	}
	log.Printf("Password mismatch for user: %s (err: %v)", username, err)
	return false
}

func HandleConnection(conn net.Conn, cfg *config.Config) {
	clientAddr := conn.RemoteAddr().String()
	log.Printf("INFO: New connection from %s", clientAddr)
	defer func() {
		conn.Close()
		log.Printf("INFO: Client disconnected: %s", clientAddr)
	}()

	state := newSessionState(cfg)

	welcomeMsg := fmt.Sprintf("220 %s Welcome to GoSMTP\r\n", state.cfg.ServerHostname)
	log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(welcomeMsg))
	conn.Write([]byte(welcomeMsg))

	currentConn := conn
	reader := bufio.NewReader(currentConn)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "use of closed network connection") {
			} else {
				log.Printf("ERROR: Error reading from client %s: %s", clientAddr, err.Error())
			}
			return
		}

		commandLine := strings.TrimSpace(line)
		if commandLine == "" {
			continue
		}
		log.Printf("RECV [%s]: %s", clientAddr, commandLine)

		parts := strings.Fields(commandLine)
		if len(parts) == 0 {
			continue
		}
		command := strings.ToUpper(parts[0])
		var response string

		switch command {
		case "QUIT":
			response = "221 Bye\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
			log.Printf("INFO: Client %s issued QUIT command.", clientAddr)
			return
		case "HELO":
			if state.isAuthenticated { // Require re-auth after HELO if already authed, or reset auth. Simpler to reset.
				log.Printf("INFO [%s]: Resetting auth state due to HELO after authentication.", clientAddr)
				state.isAuthenticated = false
				state.authenticatedUser = ""
			}
			if len(parts) < 2 {
				response = "501 Syntax error in parameters or arguments\r\n"
			} else {
				state.clientHostname = parts[1]
				state.hasSeenHelo = true
				response = fmt.Sprintf("250 %s Hello %s\r\n", state.cfg.ServerHostname, state.clientHostname)
			}
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "EHLO":
			if state.isAuthenticated { // Require re-auth after EHLO if already authed, or reset auth. Simpler to reset.
				log.Printf("INFO [%s]: Resetting auth state due to EHLO after authentication.", clientAddr)
				state.isAuthenticated = false
				state.authenticatedUser = ""
			}
			if len(parts) < 2 {
				response = "501 Syntax error in parameters or arguments\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}
			state.clientHostname = parts[1]
			state.hasSeenHelo = true
			respBase := fmt.Sprintf("250-%s Hello %s\r\n", state.cfg.ServerHostname, state.clientHostname)
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(respBase))
			currentConn.Write([]byte(respBase))

			if state.cfg.TLSCertPath != "" && state.cfg.TLSKeyPath != "" && !state.isTls {
				starttlsAdvert := "250-STARTTLS\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(starttlsAdvert))
				currentConn.Write([]byte(starttlsAdvert))
			}
			if state.isTls && len(state.cfg.Users) > 0 {
				authAdvert := "250-AUTH PLAIN LOGIN\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(authAdvert))
				currentConn.Write([]byte(authAdvert))
			}
			respPipelinig := "250 PIPELINING\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(respPipelinig))
			currentConn.Write([]byte(respPipelinig))

		case "AUTH":
			if !state.isTls {
				response = "538 Encryption required for requested authentication mechanism\r\n"
				log.Printf("WARN [%s]: AUTH attempt without TLS.", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}
			if state.isAuthenticated {
				response = "503 Bad sequence of commands (already authenticated)\r\n"
				log.Printf("WARN [%s]: AUTH attempt when already authenticated as %s.", clientAddr, state.authenticatedUser)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}
			if len(state.cfg.Users) == 0 {
				response = "454 4.7.0 Temporary authentication failure (no users configured)\r\n" // 504 might also be used
				log.Printf("WARN [%s]: AUTH attempt but no users configured.", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}

			authParts := parts // commandLine was already split into parts
			if len(authParts) < 2 {
				response = "501 Syntax error in parameters or arguments (AUTH mechanism)\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}
			mechanism := strings.ToUpper(authParts[1])

			switch mechanism {
			case "PLAIN":
				var plainAuthData string
				if len(authParts) > 2 { // AUTH PLAIN <initial-response>
					plainAuthData = authParts[2]
				} else { // AUTH PLAIN, expect client to send data next
					promptResp := "334 \r\n" // Empty prompt
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(promptResp))
					currentConn.Write([]byte(promptResp))
					payloadLine, err := reader.ReadString('\n')
					if err != nil {
						log.Printf("ERROR [%s]: Error reading AUTH PLAIN payload: %v", clientAddr, err)
						return // Connection likely dropped
					}
					plainAuthData = strings.TrimSpace(payloadLine)
					log.Printf("RECV [%s]: %s (AUTH PLAIN payload)", clientAddr, plainAuthData)
				}

				decodedBytes, err := base64.StdEncoding.DecodeString(plainAuthData)
				if err != nil {
					response = "501 Syntax error in parameters or arguments (bad base64 for PLAIN)\r\n"
					log.Printf("WARN [%s]: AUTH PLAIN with bad base64: %s", clientAddr, plainAuthData)
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
					currentConn.Write([]byte(response))
					continue
				}

				authFields := bytes.Split(decodedBytes, []byte{0})
				if len(authFields) != 3 {
					response = "501 Syntax error in parameters or arguments (malformed PLAIN payload)\r\n"
					log.Printf("WARN [%s]: AUTH PLAIN malformed payload, expected 3 parts, got %d", clientAddr, len(authFields))
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
					currentConn.Write([]byte(response))
					continue
				}
				// authorizationID := string(authFields[0]) // We don't use authorization_id
				username := string(authFields[1])
				password := string(authFields[2])

				if verifyCredentials(username, password, state.cfg.Users) {
					state.isAuthenticated = true
					state.authenticatedUser = username
					response = "235 2.7.0 Authentication Succeeded\r\n"
					log.Printf("INFO [%s]: User '%s' authenticated successfully via AUTH PLAIN.", clientAddr, username)
				} else {
					response = "535 5.7.8 Authentication credentials invalid\r\n"
					log.Printf("WARN [%s]: User '%s' failed AUTH PLAIN.", clientAddr, username)
				}
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))

			case "LOGIN":
				var username string
				if len(authParts) > 2 { // AUTH LOGIN <initial-username-base64>
					usernameB64 := authParts[2]
					log.Printf("RECV [%s]: %s (AUTH LOGIN initial username b64)", clientAddr, usernameB64)
					usernameBytes, err := base64.StdEncoding.DecodeString(usernameB64)
					if err != nil {
						response = "501 Syntax error (bad base64 initial username for LOGIN)\r\n"
						log.Printf("WARN [%s]: AUTH LOGIN with bad base64 initial username: %s", clientAddr, usernameB64)
						log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
						currentConn.Write([]byte(response))
						continue
					}
					username = string(usernameBytes)
				} else { // AUTH LOGIN, prompt for username
					usernamePrompt := "334 VXNlcm5hbWU6\r\n" // "Username:"
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(usernamePrompt))
					currentConn.Write([]byte(usernamePrompt))

					usernameLine, err := reader.ReadString('\n')
					if err != nil {
						log.Printf("ERROR [%s]: Error reading username for AUTH LOGIN: %v", clientAddr, err)
						return
					}
					usernameLine = strings.TrimSpace(usernameLine)
					log.Printf("RECV [%s]: %s (AUTH LOGIN username b64)", clientAddr, usernameLine)
					if usernameLine == "*" {
						response = "501 Authentication canceled by client\r\n"
						log.Printf("INFO [%s]: AUTH LOGIN canceled by client at username prompt.", clientAddr)
						log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
						currentConn.Write([]byte(response))
						continue
					}
					usernameBytes, err := base64.StdEncoding.DecodeString(usernameLine)
					if err != nil {
						response = "501 Syntax error (bad base64 username for LOGIN)\r\n"
						log.Printf("WARN [%s]: AUTH LOGIN with bad base64 username: %s", clientAddr, usernameLine)
						log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
						currentConn.Write([]byte(response))
						continue
					}
					username = string(usernameBytes)
				}

				passwordPrompt := "334 UGFzc3dvcmQ6\r\n" // "Password:"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(passwordPrompt))
				currentConn.Write([]byte(passwordPrompt))

				passwordLine, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("ERROR [%s]: Error reading password for AUTH LOGIN: %v", clientAddr, err)
					return
				}
				passwordLine = strings.TrimSpace(passwordLine)
				log.Printf("RECV [%s]: **** (AUTH LOGIN password b64 - not logged for security)", clientAddr)
				if passwordLine == "*" {
					response = "501 Authentication canceled by client\r\n"
					log.Printf("INFO [%s]: AUTH LOGIN canceled by client at password prompt.", clientAddr)
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
					currentConn.Write([]byte(response))
					continue
				}
				passwordBytes, err := base64.StdEncoding.DecodeString(passwordLine)
				if err != nil {
					response = "501 Syntax error (bad base64 password for LOGIN)\r\n"
					log.Printf("WARN [%s]: AUTH LOGIN with bad base64 password.", clientAddr)
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
					currentConn.Write([]byte(response))
					continue
				}
				password := string(passwordBytes)

				if verifyCredentials(username, password, state.cfg.Users) {
					state.isAuthenticated = true
					state.authenticatedUser = username
					response = "235 2.7.0 Authentication Succeeded\r\n"
					log.Printf("INFO [%s]: User '%s' authenticated successfully via AUTH LOGIN.", clientAddr, username)
				} else {
					response = "535 5.7.8 Authentication credentials invalid\r\n"
					log.Printf("WARN [%s]: User '%s' failed AUTH LOGIN.", clientAddr, username)
				}
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))

			default:
				response = "504 5.5.4 Unrecognized authentication type\r\n"
				log.Printf("WARN [%s]: Unsupported AUTH mechanism: %s", clientAddr, mechanism)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
			}
		case "MAIL":
			// RFC 4954 Section 5: MAIL command is not permitted during an authentication exchange.
			// (Our AUTH implementation is blocking, so this is implicitly handled)
			// RFC 4954 Section 6: MAIL command requires authentication if server policy dictates.
			// For now, we don't enforce auth for MAIL FROM if users are configured, but this is where it would go.
			// Example: if len(state.cfg.Users) > 0 && !state.isAuthenticated { respond 530 Authentication required; continue }

			// Authentication Check
			if state.cfg.RequireAuth && len(state.cfg.Users) > 0 && !state.isAuthenticated {
				response = "530 5.7.0 Authentication required\r\n"
				log.Printf("WARN [%s]: MAIL FROM rejected. Authentication required but not performed.", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}

			if !state.hasSeenHelo {
				response = "503 Bad sequence of commands (HELO/EHLO first)\r\n"
				log.Printf("WARN [%s]: MAIL before HELO/EHLO", clientAddr)
			} else if len(parts) < 2 || !strings.HasPrefix(strings.ToUpper(parts[1]), "FROM:") {
				response = "501 Syntax error in parameters or arguments (MAIL FROM:<address>)\r\n"
			} else {
				addr := strings.TrimPrefix(parts[1], "FROM:")
				addr = strings.Trim(addr, "<>")
				if addr == "" {
					response = "501 Syntax error in parameters or arguments (empty address in MAIL FROM)\r\n"
				} else {
					state.mailFromAddress = addr
					log.Printf("INFO [%s]: Mail from: %s", clientAddr, state.mailFromAddress)
					response = "250 OK\r\n"
				}
			}
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "RCPT":
			if state.mailFromAddress == "" {
				response = "503 Bad sequence of commands (MAIL FROM first)\r\n"
				log.Printf("WARN [%s]: RCPT TO before MAIL FROM", clientAddr)
			} else if len(parts) < 2 || !strings.HasPrefix(strings.ToUpper(parts[1]), "TO:") {
				response = "501 Syntax error in parameters or arguments (RCPT TO:<address>)\r\n"
			} else {
				addr := strings.TrimPrefix(parts[1], "TO:")
				addr = strings.Trim(addr, "<>")
				if addr == "" {
					response = "501 Syntax error in parameters or arguments (empty address in RCPT TO)\r\n"
				} else {
					state.rcptToAddresses = append(state.rcptToAddresses, addr)
					log.Printf("INFO [%s]: Recipient to: %s", clientAddr, addr)
					response = "250 OK\r\n"
				}
			}
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "STARTTLS":
			if state.isTls {
				response = "503 Bad sequence of commands (TLS already active)\r\n"
				log.Printf("WARN [%s]: STARTTLS attempted when TLS is already active.", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}
			if state.cfg.TLSCertPath == "" || state.cfg.TLSKeyPath == "" {
				response = "454 TLS not available (server not configured for TLS)\r\n"
				log.Printf("WARN [%s]: STARTTLS attempted but server not configured for TLS (cert or key path empty).", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}

			cert, err := tls.LoadX509KeyPair(state.cfg.TLSCertPath, state.cfg.TLSKeyPath)
			if err != nil {
				log.Printf("ERROR [%s]: Error loading TLS certificate/key (%s, %s): %v", clientAddr, state.cfg.TLSCertPath, state.cfg.TLSKeyPath, err)
				response = "454 TLS not available (certificate/key error)\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}

			response = "220 Ready to start TLS\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))

			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
			tlsConn := tls.Server(currentConn, tlsConfig)
			log.Printf("INFO [%s]: Starting TLS handshake...", clientAddr)
			err = tlsConn.Handshake()
			if err != nil {
				log.Printf("ERROR [%s]: TLS handshake failed: %v", clientAddr, err)
				currentConn.Close()
				return
			}

			currentConn = tlsConn
			reader = bufio.NewReader(currentConn)

			state.isTls = true
			state.hasSeenHelo = false
			state.clientHostname = ""
			state.mailFromAddress = ""
			state.rcptToAddresses = make([]string, 0)
			state.isInDataMode = false
			state.isAuthenticated = false // Reset auth state after STARTTLS
			state.authenticatedUser = ""  // Reset auth user after STARTTLS

			log.Printf("INFO [%s]: Connection successfully upgraded to TLS and session state reset. Client should re-EHLO.", clientAddr)
			continue

		case "DATA":
			if len(state.rcptToAddresses) == 0 {
				response = "503 Bad sequence of commands (RCPT TO first)\r\n"
				log.Printf("WARN [%s]: DATA before RCPT TO", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				continue
			}
			state.isInDataMode = true
			response = "354 Start mail input; end with <CRLF>.<CRLF>\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))

			var emailBody strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("ERROR [%s]: Error reading data: %s", clientAddr, err.Error())
					state.isInDataMode = false
					state.resetMailState()
					return
				}

				trimmedLine := strings.TrimRight(dataLine, "\r\n")
				log.Printf("RECV_DATA [%s]: %s", clientAddr, trimmedLine)


				if trimmedLine == "." {
					break
				}

				if strings.HasPrefix(trimmedLine, "..") {
					emailBody.WriteString(trimmedLine[1:] + "\r\n")
				} else {
					emailBody.WriteString(trimmedLine + "\r\n")
				}
			}
			state.isInDataMode = false

			mailDir := state.cfg.MailDir
			if _, err := os.Stat(mailDir); os.IsNotExist(err) {
				log.Printf("INFO [%s]: Mail directory %s does not exist, attempting to create it.", clientAddr, mailDir)
				if err := os.MkdirAll(mailDir, 0750); err != nil {
					log.Printf("ERROR [%s]: Error creating mail directory %s: %v", clientAddr, mailDir, err)
					response = "451 Requested action aborted: local error in processing (cannot create mail directory)\r\n"
					log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
					currentConn.Write([]byte(response))
					state.resetMailState()
					continue
				}
				log.Printf("INFO [%s]: Mail directory %s created successfully.", clientAddr, mailDir)
			}

			timestamp := time.Now().Format("20060102150405")
			randomBytes := make([]byte, 4)
			if _, err := rand.Read(randomBytes); err != nil {
				log.Printf("ERROR [%s]: Error generating random string for filename: %v", clientAddr, err)
				response = "451 Requested action aborted: local error in processing (cannot generate filename)\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				state.resetMailState()
				continue
			}
			randomString := hex.EncodeToString(randomBytes)
			messageID := fmt.Sprintf("%s_%s", timestamp, randomString) // Filename base is now MessageID
			emlFilePath := filepath.Join(mailDir, messageID+".eml")
			metaFilePath := filepath.Join(mailDir, messageID+".meta")

			fullEmailContent := emailBody.String()
			err = os.WriteFile(emlFilePath, []byte(fullEmailContent), 0640)
			if err != nil {
				log.Printf("ERROR [%s]: Error writing email data to %s: %v", clientAddr, emlFilePath, err)
				response = "451 Requested action aborted: local error in processing (cannot save email data)\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				state.resetMailState()
				continue
			}
			log.Printf("INFO [%s]: Email data from %s to %v saved to %s", clientAddr, state.mailFromAddress, state.rcptToAddresses, emlFilePath)

			// Create and save metadata
			now := time.Now()
			metadata := &queue.MailMetadata{
				MessageID:       messageID,
				Sender:          state.mailFromAddress,
				Recipients:      state.rcptToAddresses, // Storing all recipients from the transaction
				ReceivedTime:    now,
				NextAttemptTime: now, // Ready for immediate processing
				AttemptCount:    0,
				// LastAttemptTime is zero initially
				// LastError is empty initially
			}

			if err := queue.SaveMetadata(metaFilePath, metadata); err != nil {
				log.Printf("CRITICAL [%s]: Email data saved to %s but FAILED to save metadata to %s: %v. This message may not be processed.", clientAddr, emlFilePath, metaFilePath, err)
				// This is an inconsistent state. The email is saved but won't be processed without metadata.
				// Options: attempt to delete emlFilePath, or leave it for manual recovery.
				// For now, log critical and send a generic error, as client already got 354.
				// A more specific error might be 451 or 554.
				response = "451 Requested action aborted: local error in processing (internal server error)\r\n" // Client doesn't need to know it's metadata
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				currentConn.Write([]byte(response))
				state.resetMailState()
				continue
			}
			// Consolidated log message for queuing
			log.Printf("INFO [%s]: Email (MessageID: %s) from %s to %v queued successfully. Data: %s, Meta: %s",
				clientAddr, messageID, state.mailFromAddress, state.rcptToAddresses, emlFilePath, metaFilePath)

			response = fmt.Sprintf("250 OK: message accepted for delivery (queued as %s)\r\n", messageID)
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
			state.resetMailState()
		case "RSET":
			log.Printf("INFO [%s]: Session reset initiated by RSET command.", clientAddr)
			state.resetMailState()
			state.isAuthenticated = false // Reset authentication state
			state.authenticatedUser = ""
			state.hasSeenHelo = false // RSET should also clear HELO state
			state.clientHostname = ""
			log.Printf("INFO [%s]: Authentication and HELO state reset due to RSET.", clientAddr)
			response = "250 OK\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "NOOP":
			response = "250 OK\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		default:
			response = "500 Syntax error, command unrecognized\r\n"
			log.Printf("WARN [%s]: Unknown command: %s", clientAddr, commandLine)
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		}
	}
}
