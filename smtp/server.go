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
	"go-smtp/filter" // Added for incoming mail filtering
	"go-smtp/queue"  // Added for mail queue metadata
	"go-smtp/ratelimit" // Added for connection rate limiting
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
	clientIP          string         // Store clientIP for consistent logging within session
	CommandsProcessed int            // Counter for commands this session
}

func newSessionState(cfg *config.Config, clientIP string) *sessionState {
	return &sessionState{
		rcptToAddresses:   make([]string, 0),
		isTls:             false,
		isAuthenticated:   false,
		authenticatedUser: "",
		cfg:               cfg,
		clientIP:          clientIP,
		CommandsProcessed: 0,
	}
}

func (s *sessionState) resetMailState() {
	s.mailFromAddress = ""
	s.rcptToAddresses = make([]string, 0)
}

func verifyCredentials(username, password string, cfgUsers map[string]string) bool {
	storedHash, userExists := cfgUsers[username]
	if !userExists {
		log.Printf("Auth attempt for non-existent user: %s", username)
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err == nil {
		return true
	}
	log.Printf("Password mismatch for user: %s (err: %v)", username, err)
	return false
}

func HandleConnection(conn net.Conn, cfg *config.Config, clientIP string) {
	log.Printf("INFO: [%s] New connection received.", clientIP)

	defer func() {
		if cfg.RateLimitEnable && cfg.MaxConnectionsPerIP > 0 {
			ratelimit.DecrementConnectionCount(clientIP)
		}
		conn.Close()
		log.Printf("INFO: [%s] Connection closed.", clientIP)
	}()

	state := newSessionState(cfg, clientIP)

	welcomeMsg := fmt.Sprintf("220 %s Welcome to GoSMTP\r\n", state.cfg.ServerHostname)
	log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(welcomeMsg))
	conn.Write([]byte(welcomeMsg))

	currentConn := conn
	reader := bufio.NewReader(currentConn)
	writer := bufio.NewWriter(currentConn) // Use a buffered writer for responses

	// Helper to send response
	sendResponse := func(msg string) {
		log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(msg))
		_, errWrite := writer.WriteString(msg)
		if errWrite != nil {
			log.Printf("ERROR: [%s] Failed to write response: %v", clientIP, errWrite)
			// Consider this a connection error and return, which will trigger defer
			// For now, we'll let the loop try to continue or exit on next read error.
		}
		errFlush := writer.Flush()
		if errFlush != nil {
			log.Printf("ERROR: [%s] Failed to flush response: %v", clientIP, errFlush)
		}
	}


	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("DEBUG: [%s] Client closed connection (EOF or closed network).", clientIP)
			} else {
				log.Printf("ERROR: [%s] Error reading from client: %s", clientIP, err.Error())
			}
			return
		}

		state.CommandsProcessed++
		if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
			errMsg := fmt.Sprintf("421 %s Service not available, too many commands in this session (%d > %d). Closing connection.\r\n", cfg.ServerHostname, state.CommandsProcessed, cfg.MaxCommandsPerSession)
			log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg))
			sendResponse(errMsg)
			return
		}

		commandLine := strings.TrimSpace(line)
		if commandLine == "" {
			continue
		}
		log.Printf("RECV [%s]: %s", clientIP, commandLine)

		parts := strings.Fields(commandLine)
		if len(parts) == 0 {
			continue
		}
		command := strings.ToUpper(parts[0])
		response = "" // Default to no response / handled internally

		switch command {
		case "QUIT":
			response = "221 Bye\r\n"
			sendResponse(response)
			log.Printf("INFO: [%s] Client issued QUIT command.", clientIP)
			return
		case "HELO":
			if state.isAuthenticated {
				log.Printf("INFO: [%s] Resetting auth state due to HELO after authentication.", clientIP)
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
		case "EHLO":
			if state.isAuthenticated {
				log.Printf("INFO: [%s] Resetting auth state due to EHLO after authentication.", clientIP)
				state.isAuthenticated = false
				state.authenticatedUser = ""
			}
			if len(parts) < 2 {
				response = "501 Syntax error in parameters or arguments\r\n"
			} else {
				state.clientHostname = parts[1]
				state.hasSeenHelo = true
				// Send EHLO response line by line
				sendResponse(fmt.Sprintf("250-%s Hello %s\r\n", state.cfg.ServerHostname, state.clientHostname))
				if state.cfg.TLSCertPath != "" && state.cfg.TLSKeyPath != "" && !state.isTls {
					sendResponse("250-STARTTLS\r\n")
				}
				if state.isTls && len(state.cfg.Users) > 0 {
					sendResponse("250-AUTH PLAIN LOGIN\r\n")
				}
				sendResponse("250 PIPELINING\r\n")
				continue // Skip default response sending at the end of the loop
			}

		case "AUTH":
			if !state.isTls {
				response = "538 Encryption required for requested authentication mechanism\r\n"; log.Printf("WARN: [%s] AUTH attempt without TLS.", clientIP)
			} else if state.isAuthenticated {
				response = "503 Bad sequence of commands (already authenticated)\r\n"; log.Printf("WARN: [%s] AUTH attempt when already authenticated as %s.", clientIP, state.authenticatedUser)
			} else if len(state.cfg.Users) == 0 {
				response = "454 4.7.0 Temporary authentication failure (no users configured)\r\n"; log.Printf("WARN: [%s] AUTH attempt but no users configured.", clientIP)
			} else {
				authParts := parts
				if len(authParts) < 2 {
					response = "501 Syntax error in parameters or arguments (AUTH mechanism)\r\n"
				} else {
					mechanism := strings.ToUpper(authParts[1])
					switch mechanism {
					case "PLAIN":
						var plainAuthData string
						if len(authParts) > 2 { plainAuthData = authParts[2]
						} else {
							sendResponse("334 \r\n")
							state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
								errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); sendResponse(errMsg); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); return }
							payloadLine, errRead := reader.ReadString('\n'); if errRead != nil { log.Printf("ERROR: [%s] Error reading AUTH PLAIN payload: %v", clientIP, errRead); return }
							plainAuthData = strings.TrimSpace(payloadLine); log.Printf("RECV [%s]: %s (AUTH PLAIN payload)", clientIP, plainAuthData)
						}
						decodedBytes, errDec := base64.StdEncoding.DecodeString(plainAuthData)
						if errDec != nil { response = "501 Syntax error (bad base64 for PLAIN)\r\n"; log.Printf("WARN: [%s] AUTH PLAIN bad base64: %s", clientIP, plainAuthData)
						} else {
							authFields := bytes.Split(decodedBytes, []byte{0})
							if len(authFields) != 3 { response = "501 Syntax error (malformed PLAIN payload)\r\n"; log.Printf("WARN: [%s] AUTH PLAIN malformed payload parts: %d", clientIP, len(authFields))
							} else {
								username := string(authFields[1]); password := string(authFields[2])
								if verifyCredentials(username, password, state.cfg.Users) {
									state.isAuthenticated = true; state.authenticatedUser = username
									response = "235 2.7.0 Authentication Succeeded\r\n"; log.Printf("INFO: [%s] User '%s' AUTH PLAIN success.", clientIP, username)
								} else { response = "535 5.7.8 Authentication credentials invalid\r\n"; log.Printf("WARN: [%s] User '%s' AUTH PLAIN failed.", clientIP, username) }
							}
						}
					case "LOGIN":
						var username string
						if len(authParts) > 2 {
							usernameB64 := authParts[2]; log.Printf("RECV [%s]: %s (AUTH LOGIN initial username b64)", clientIP, usernameB64)
							usernameBytes, errDec := base64.StdEncoding.DecodeString(usernameB64)
							if errDec != nil { response = "501 Syntax error (bad base64 initial username for LOGIN)\r\n"; log.Printf("WARN: [%s] AUTH LOGIN bad initial base64 username: %s", clientIP, usernameB64); break }
							username = string(usernameBytes)
						} else {
							sendResponse("334 VXNlcm5hbWU6\r\n"); // "Username:"
							state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
								errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); sendResponse(errMsg); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); return }
							usernameLine, errRead := reader.ReadString('\n'); if errRead != nil { log.Printf("ERROR: [%s] Error reading username for AUTH LOGIN: %v", clientIP, errRead); return }
							usernameLine = strings.TrimSpace(usernameLine); log.Printf("RECV [%s]: %s (AUTH LOGIN username b64)", clientIP, usernameLine)
							if usernameLine == "*" { response = "501 Authentication canceled\r\n"; log.Printf("INFO: [%s] AUTH LOGIN canceled by client at username.", clientIP); break }
							usernameBytes, errDec := base64.StdEncoding.DecodeString(usernameLine)
							if errDec != nil { response = "501 Syntax error (bad base64 username for LOGIN)\r\n"; log.Printf("WARN: [%s] AUTH LOGIN bad base64 username: %s", clientIP, usernameLine); break }
							username = string(usernameBytes)
						}
						sendResponse("334 UGFzc3dvcmQ6\r\n"); // "Password:"
						state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
							errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); sendResponse(errMsg); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); return }
						passwordLine, errRead := reader.ReadString('\n'); if errRead != nil { log.Printf("ERROR: [%s] Error reading password for AUTH LOGIN: %v", clientIP, errRead); return }
						passwordLine = strings.TrimSpace(passwordLine); log.Printf("RECV [%s]: **** (AUTH LOGIN password b64 - not logged)", clientIP)
						if passwordLine == "*" { response = "501 Authentication canceled\r\n"; log.Printf("INFO: [%s] AUTH LOGIN canceled at password.", clientIP); break }
						passwordBytes, errDec := base64.StdEncoding.DecodeString(passwordLine)
						if errDec != nil { response = "501 Syntax error (bad base64 password for LOGIN)\r\n"; log.Printf("WARN: [%s] AUTH LOGIN bad base64 password.", clientIP); break }
						password := string(passwordBytes)
						if verifyCredentials(username, password, state.cfg.Users) {
							state.isAuthenticated = true; state.authenticatedUser = username
							response = "235 2.7.0 Authentication Succeeded\r\n"; log.Printf("INFO: [%s] User '%s' AUTH LOGIN success.", clientIP, username)
						} else { response = "535 5.7.8 Authentication credentials invalid\r\n"; log.Printf("WARN: [%s] User '%s' AUTH LOGIN failed.", clientIP, username) }
					default: response = "504 5.5.4 Unrecognized authentication type\r\n"; log.Printf("WARN: [%s] Unsupported AUTH mechanism: %s", clientIP, mechanism)
					}
				}
			}
			if response == "" { response = "500 Command processing error for AUTH\r\n" }

		case "MAIL":
			if state.cfg.RequireAuth && len(state.cfg.Users) > 0 && !state.isAuthenticated {
				response = "530 5.7.0 Authentication required\r\n"; log.Printf("WARN: [%s] MAIL FROM rejected. Auth required.", clientIP)
			} else if !state.hasSeenHelo {
				response = "503 Bad sequence (HELO/EHLO first)\r\n"; log.Printf("WARN: [%s] MAIL FROM before HELO/EHLO.", clientIP)
			} else if len(parts) < 2 || !strings.HasPrefix(strings.ToUpper(parts[1]), "FROM:") { // Simplified parsing
				response = "501 Syntax error (MAIL FROM:<address>)\r\n"
			} else {
				addr := strings.TrimPrefix(strings.Join(parts[1:], " "), "FROM:"); addr = strings.Trim(addr, "<> ")
				if addr == "" { response = "501 Syntax error (empty address in MAIL FROM)\r\n"
				} else { state.mailFromAddress = addr; log.Printf("INFO: [%s] Mail from: %s", clientIP, state.mailFromAddress); response = "250 OK\r\n" }
			}
		case "RCPT":
			commandArgs := ""; if len(parts) > 1 { commandArgs = strings.Join(parts[1:], " ") }
			rawRecipientArg := strings.TrimPrefix(strings.ToUpper(commandArgs), "TO:"); rawRecipientArg = strings.Trim(rawRecipientArg, "<> ")


			if cfg.RateLimitEnable && cfg.MaxRecipientsPerMessage > 0 {
				if len(state.rcptToAddresses) >= cfg.MaxRecipientsPerMessage {
					log.Printf("WARN: [%s] Recipient limit exceeded. Max: %d. Recipient %s rejected.", clientIP, cfg.MaxRecipientsPerMessage, rawRecipientArg)
					response = fmt.Sprintf("452 4.5.3 Too many recipients for this message (limit: %d).\r\n", cfg.MaxRecipientsPerMessage)
					sendResponse(response)
					continue
				}
			}

			if state.mailFromAddress == "" {
				response = "503 Bad sequence (MAIL FROM first)\r\n"; log.Printf("WARN: [%s] RCPT TO before MAIL FROM.", clientIP)
			} else if !strings.HasPrefix(strings.ToUpper(commandArgs), "TO:") { // Check for "TO:"
				response = "501 Syntax error (RCPT TO:<address>)\r\n"
			} else {
				addr := strings.TrimPrefix(commandArgs, "TO:"); addr = strings.Trim(addr, "<> ") // Use original case for addr
				if addr == "" { response = "501 Syntax error (empty address in RCPT TO)\r\n"
				} else { state.rcptToAddresses = append(state.rcptToAddresses, addr); log.Printf("INFO: [%s] Recipient to: %s (Total: %d)", clientIP, addr, len(state.rcptToAddresses)); response = "250 OK\r\n" }
			}
		case "STARTTLS":
			if state.isTls { response = "503 Bad sequence (TLS already active)\r\n"; log.Printf("WARN: [%s] STARTTLS when TLS active.", clientIP)
			} else if state.cfg.TLSCertPath == "" || state.cfg.TLSKeyPath == "" { response = "454 TLS not available (server not configured)\r\n"; log.Printf("WARN: [%s] STARTTLS when not configured.", clientIP)
			} else {
				cert, errTLS := tls.LoadX509KeyPair(state.cfg.TLSCertPath, state.cfg.TLSKeyPath)
				if errTLS != nil { log.Printf("ERROR: [%s] Error loading TLS cert/key: %v", clientIP, errTLS); response = "454 TLS not available (cert/key error)\r\n"
				} else {
					sendResponse("220 Ready to start TLS\r\n")
					tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
					tlsConn := tls.Server(currentConn, tlsConfig)
					log.Printf("INFO: [%s] Starting TLS handshake...", clientIP)
					errTLS = tlsConn.Handshake(); if errTLS != nil { log.Printf("ERROR: [%s] TLS handshake failed: %v", clientIP, errTLS); return }
					currentConn = tlsConn; reader = bufio.NewReader(currentConn); writer = bufio.NewWriter(currentConn) // Reset writer too!
					state.isTls = true; state.hasSeenHelo = false; state.clientHostname = ""; state.mailFromAddress = ""; state.rcptToAddresses = make([]string, 0)
					state.isInDataMode = false; state.isAuthenticated = false; state.authenticatedUser = ""
					log.Printf("INFO: [%s] Connection upgraded to TLS. Session state reset.", clientIP); continue
				}
			}
		case "DATA":
			if len(state.rcptToAddresses) == 0 { response = "503 Bad sequence (RCPT TO first)\r\n"; log.Printf("WARN: [%s] DATA before RCPT TO.", clientIP)
			} else {
				state.isInDataMode = true; response = "354 Start mail input; end with <CRLF>.<CRLF>\r\n"
				sendResponse(response)
				var emailDataBytes bytes.Buffer // Use bytes.Buffer for raw data
				for {
					state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
						errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); sendResponse(errMsg); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); return }

					dataLine, errRead := reader.ReadString('\n'); if errRead != nil { log.Printf("ERROR: [%s] Error reading data: %s", clientIP, errRead.Error()); return }

					// No logging of raw dataLine here for privacy by default
					// log.Printf("RECV_DATA_LINE [%s]: %s", clientIP, strings.TrimRight(dataLine, "\r\n"))

					if strings.TrimRight(dataLine, "\r\n") == "." { break } // Trim CRLF before checking for "."

					// Handle dot-stuffing: if a line starts with "..", only write one "."
					if strings.HasPrefix(dataLine, "..") {
						emailDataBytes.WriteString(dataLine[1:])
					} else {
						emailDataBytes.WriteString(dataLine)
					}
				}
				state.isInDataMode = false

				// --- Incoming Filter Logic ---
				finalEmailData := emailDataBytes.Bytes() // Data to be potentially queued or quarantined
				messageID := "" // Will be generated once before any file operation

				timestamp := time.Now().Format("20060102150405"); randomBytes := make([]byte, 4); _, _ = rand.Read(randomBytes)
				messageID = fmt.Sprintf("%s_%s", timestamp, hex.EncodeToString(randomBytes))

				if cfg.IncomingFilterEnable && cfg.IncomingFilterScriptPath != "" {
					log.Printf("INFO: [%s] MessageID: %s - Executing incoming mail filter script: %s", clientIP, messageID, cfg.IncomingFilterScriptPath)
					scanOutput := filter.ScanEmailWithScript(
						finalEmailData, // Pass the current email data
						cfg.IncomingFilterScriptPath,
						cfg.IncomingFilterScriptArgs,
						cfg.IncomingFilterScriptTimeoutDuration,
					)

					if scanOutput.Result == filter.ScanResultError {
						log.Printf("ERROR: [%s] MessageID: %s - Filter script execution error: %v. Allowing message through (fail-open).", clientIP, messageID, scanOutput.Error)
					} else if scanOutput.Result == filter.ScanResultDetected {
						log.Printf("INFO: [%s] MessageID: %s - Filter script detected content. Action: %s", clientIP, messageID, cfg.IncomingFilterActionOnDetection)
						switch cfg.IncomingFilterActionOnDetection {
						case "reject":
							response = strings.TrimSpace(cfg.IncomingFilterRejectMessage) + "\r\n"
							sendResponse(response)
							log.Printf("INFO: [%s] MessageID: %s - Rejected due to filter detection.", clientIP, messageID)
							return // Do not queue
						case "add_header":
							if len(scanOutput.HeadersToAdd) > 0 {
								var newHeaderBlock bytes.Buffer
								for name, value := range scanOutput.HeadersToAdd { newHeaderBlock.WriteString(fmt.Sprintf("%s: %s\r\n", name, value)) }
								finalEmailData = append(newHeaderBlock.Bytes(), finalEmailData...)
								log.Printf("INFO: [%s] MessageID: %s - Added %d headers from filter script.", clientIP, messageID, len(scanOutput.HeadersToAdd))
							} else if cfg.IncomingFilterHeaderName != "" {
								 var defaultHeader bytes.Buffer
								 defaultHeader.WriteString(fmt.Sprintf("%s: Detected\r\n", cfg.IncomingFilterHeaderName))
								 finalEmailData = append(defaultHeader.Bytes(), finalEmailData...)
								 log.Printf("INFO: [%s] MessageID: %s - Added default detection header: %s", clientIP, messageID, cfg.IncomingFilterHeaderName)
							}
						case "quarantine":
							if cfg.IncomingFilterQuarantineDir == "" {
								log.Printf("ERROR: [%s] MessageID: %s - Action 'quarantine' but IncomingFilterQuarantineDir not set. Allowing message through (fail-open).", clientIP, messageID)
							} else {
								if errMk := os.MkdirAll(cfg.IncomingFilterQuarantineDir, 0750); errMk != nil {
									log.Printf("ERROR: [%s] MessageID: %s - Cannot create quarantine directory %s: %v. Allowing message through (fail-open).", clientIP, messageID, cfg.IncomingFilterQuarantineDir, errMk)
								} else {
									qFilePath := filepath.Join(cfg.IncomingFilterQuarantineDir, messageID+".eml")
									if errWrite := os.WriteFile(qFilePath, finalEmailData, 0640); errWrite != nil {
										log.Printf("ERROR: [%s] MessageID: %s - Failed to write to quarantine %s: %v. Allowing (fail-open).", clientIP, messageID, qFilePath, errWrite)
									} else {
										log.Printf("INFO: [%s] MessageID: %s - Quarantined to %s due to filter detection.", clientIP, messageID, qFilePath)
										response = fmt.Sprintf("250 2.0.0 OK: message accepted (queued as %s) - internal handling applied\r\n", messageID)
										sendResponse(response)
										state.resetMailState() // Reset for next potential message in session
										continue // Do not queue for normal delivery
									}
								}
							}
						default: log.Printf("WARN: [%s] MessageID: %s - Unknown filter action '%s'. Allowing through.", clientIP, messageID, cfg.IncomingFilterActionOnDetection)
						}
					} else { // ScanResultClean
						log.Printf("INFO: [%s] MessageID: %s - Filter script found content to be clean.", clientIP, messageID)
						if len(scanOutput.HeadersToAdd) > 0 { // Add headers even if clean (e.g. X-Spam-Score)
							var newHeaderBlock bytes.Buffer
							for name, value := range scanOutput.HeadersToAdd { newHeaderBlock.WriteString(fmt.Sprintf("%s: %s\r\n", name, value)) }
							finalEmailData = append(newHeaderBlock.Bytes(), finalEmailData...)
							log.Printf("INFO: [%s] MessageID: %s - Added %d headers from (clean) filter script.", clientIP, messageID, len(scanOutput.HeadersToAdd))
						}
					}
				}
				// Proceed to queue with finalEmailData

				mailDir := state.cfg.MailDir
				if _, errStat := os.Stat(mailDir); os.IsNotExist(errStat) {
					log.Printf("INFO: [%s] MailDir %s not found, creating.", clientIP, mailDir)
					if errMkdir := os.MkdirAll(mailDir, 0750); errMkdir != nil {
						log.Printf("ERROR: [%s] Failed to create MailDir %s: %v", clientIP, mailDir, errMkdir); response = "451 Error in processing (cannot create mail dir)\r\n"; break
					}
				}
				emlFilePath := filepath.Join(mailDir, messageID+".eml"); metaFilePath := filepath.Join(mailDir, messageID+".meta")
				if errWrite := os.WriteFile(emlFilePath, finalEmailData, 0640); errWrite != nil {
					log.Printf("ERROR: [%s] Failed to write .eml %s: %v", clientIP, emlFilePath, errWrite); response = "451 Error in processing (cannot save mail)\r\n"; break
				}
				now := time.Now(); metadata := &queue.MailMetadata{ MessageID: messageID, Sender: state.mailFromAddress, Recipients: state.rcptToAddresses, ReceivedTime: now, NextAttemptTime: now, AttemptCount: 0 }
				if errMeta := queue.SaveMetadata(metaFilePath, metadata); errMeta != nil {
					log.Printf("CRITICAL: [%s] Saved .eml %s but FAILED to save .meta %s: %v", clientIP, emlFilePath, metaFilePath, errMeta); response = "451 Error in processing (metadata failure)\r\n"
				} else {
					log.Printf("INFO: [%s] Email (ID: %s) from %s queued for %v. Files: %s, %s", clientIP, messageID, state.mailFromAddress, state.rcptToAddresses, emlFilePath, metaFilePath)
					response = fmt.Sprintf("250 OK: message accepted for delivery (queued as %s)\r\n", messageID)
				}
				state.resetMailState()
			}
		case "RSET":
			log.Printf("INFO: [%s] Session reset by RSET.", clientIP)
			state.resetMailState(); state.isAuthenticated = false; state.authenticatedUser = ""; state.hasSeenHelo = false; state.clientHostname = ""
			log.Printf("INFO: [%s] Auth and HELO state reset by RSET.", clientIP)
			response = "250 OK\r\n"
		case "NOOP":
			response = "250 OK\r\n"
		default:
			response = "500 Syntax error, command unrecognized\r\n"
			log.Printf("WARN: [%s] Unknown command: %s", clientIP, commandLine)
		}

		if response != "" {
			sendResponse(response)
		}
	}
}
