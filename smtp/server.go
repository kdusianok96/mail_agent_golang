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
	// Using currentConn.Write for responses to simplify; a buffered writer would be reset after STARTTLS.

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
			_, _ = currentConn.Write([]byte(errMsg))
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
		var response string

		switch command {
		case "QUIT":
			response = "221 Bye\r\n"
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
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
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
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
				respBase := fmt.Sprintf("250-%s Hello %s\r\n", state.cfg.ServerHostname, state.clientHostname)
				currentConn.Write([]byte(respBase)) // Send part by part
				log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(respBase))

				if state.cfg.TLSCertPath != "" && state.cfg.TLSKeyPath != "" && !state.isTls {
					starttlsAdvert := "250-STARTTLS\r\n"
					currentConn.Write([]byte(starttlsAdvert))
					log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(starttlsAdvert))
				}
				if state.isTls && len(state.cfg.Users) > 0 {
					authAdvert := "250-AUTH PLAIN LOGIN\r\n"
					currentConn.Write([]byte(authAdvert))
					log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(authAdvert))
				}
				respPipelinig := "250 PIPELINING\r\n"
				currentConn.Write([]byte(respPipelinig))
				log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(respPipelinig))
				continue // Skip default response sending at the end
			}
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response)) // For 501 case
			currentConn.Write([]byte(response))

		case "AUTH":
			// ... (AUTH logic as before)
			if !state.isTls {
				response = "538 Encryption required for requested authentication mechanism\r\n"
				log.Printf("WARN: [%s] AUTH attempt without TLS.", clientIP)
			} else if state.isAuthenticated {
				response = "503 Bad sequence of commands (already authenticated)\r\n"
				log.Printf("WARN: [%s] AUTH attempt when already authenticated as %s.", clientIP, state.authenticatedUser)
			} else if len(state.cfg.Users) == 0 {
				response = "454 4.7.0 Temporary authentication failure (no users configured)\r\n"
				log.Printf("WARN: [%s] AUTH attempt but no users configured.", clientIP)
			} else {
				authParts := parts
				if len(authParts) < 2 {
					response = "501 Syntax error in parameters or arguments (AUTH mechanism)\r\n"
				} else {
					mechanism := strings.ToUpper(authParts[1])
					switch mechanism {
					case "PLAIN":
						var plainAuthData string
						if len(authParts) > 2 {
							plainAuthData = authParts[2]
						} else {
							promptResp := "334 \r\n"
							currentConn.Write([]byte(promptResp)); log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(promptResp))
							state.CommandsProcessed++
							if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
								errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname)
								log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); _, _ = currentConn.Write([]byte(errMsg)); return
							}
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
							usernamePrompt := "334 VXNlcm5hbWU6\r\n"; currentConn.Write([]byte(usernamePrompt)); log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(usernamePrompt))
							state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
								errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); _, _ = currentConn.Write([]byte(errMsg)); return }
							usernameLine, errRead := reader.ReadString('\n'); if errRead != nil { log.Printf("ERROR: [%s] Error reading username for AUTH LOGIN: %v", clientIP, errRead); return }
							usernameLine = strings.TrimSpace(usernameLine); log.Printf("RECV [%s]: %s (AUTH LOGIN username b64)", clientIP, usernameLine)
							if usernameLine == "*" { response = "501 Authentication canceled\r\n"; log.Printf("INFO: [%s] AUTH LOGIN canceled by client at username.", clientIP); break }
							usernameBytes, errDec := base64.StdEncoding.DecodeString(usernameLine)
							if errDec != nil { response = "501 Syntax error (bad base64 username for LOGIN)\r\n"; log.Printf("WARN: [%s] AUTH LOGIN bad base64 username: %s", clientIP, usernameLine); break }
							username = string(usernameBytes)
						}
						passwordPrompt := "334 UGFzc3dvcmQ6\r\n"; currentConn.Write([]byte(passwordPrompt)); log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(passwordPrompt))
						state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
							errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); _, _ = currentConn.Write([]byte(errMsg)); return }
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
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))

		case "MAIL":
			if state.cfg.RequireAuth && len(state.cfg.Users) > 0 && !state.isAuthenticated {
				response = "530 5.7.0 Authentication required\r\n"; log.Printf("WARN: [%s] MAIL FROM rejected. Auth required.", clientIP)
			} else if !state.hasSeenHelo {
				response = "503 Bad sequence (HELO/EHLO first)\r\n"; log.Printf("WARN: [%s] MAIL FROM before HELO/EHLO.", clientIP)
			} else if len(parts) < 2 || !strings.HasPrefix(strings.ToUpper(parts[1]), "FROM:") {
				response = "501 Syntax error (MAIL FROM:<address>)\r\n"
			} else {
				addr := strings.TrimPrefix(strings.Join(parts[1:], " "), "FROM:"); addr = strings.Trim(addr, "<> ")
				if addr == "" { response = "501 Syntax error (empty address in MAIL FROM)\r\n"
				} else { state.mailFromAddress = addr; log.Printf("INFO: [%s] Mail from: %s", clientIP, state.mailFromAddress); response = "250 OK\r\n" }
			}
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "RCPT":
			commandArgs := ""
			if len(parts) > 1 { commandArgs = strings.Join(parts[1:], " ") }

			if cfg.RateLimitEnable && cfg.MaxRecipientsPerMessage > 0 {
				if len(state.rcptToAddresses) >= cfg.MaxRecipientsPerMessage {
					rejectedRecipient := strings.TrimPrefix(strings.ToUpper(commandArgs), "TO:"); rejectedRecipient = strings.Trim(rejectedRecipient, "<> ")
					log.Printf("WARN: [%s] Recipient limit exceeded. Max: %d. Recipient %s rejected.", clientIP, cfg.MaxRecipientsPerMessage, rejectedRecipient)
					response = fmt.Sprintf("452 4.5.3 Too many recipients for this message (limit: %d).\r\n", cfg.MaxRecipientsPerMessage)
					log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
					currentConn.Write([]byte(response))
					continue // Skip further processing for this RCPT TO
				}
			}

			if state.mailFromAddress == "" {
				response = "503 Bad sequence (MAIL FROM first)\r\n"; log.Printf("WARN: [%s] RCPT TO before MAIL FROM.", clientIP)
			} else if !strings.HasPrefix(strings.ToUpper(commandArgs), "TO:") {
				response = "501 Syntax error (RCPT TO:<address>)\r\n"
			} else {
				addr := strings.TrimPrefix(commandArgs, "TO:"); addr = strings.Trim(addr, "<> ") // Case sensitive by default for address part
				if addr == "" { response = "501 Syntax error (empty address in RCPT TO)\r\n"
				} else { state.rcptToAddresses = append(state.rcptToAddresses, addr); log.Printf("INFO: [%s] Recipient to: %s (Total: %d)", clientIP, addr, len(state.rcptToAddresses)); response = "250 OK\r\n" }
			}
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "STARTTLS":
			if state.isTls { response = "503 Bad sequence (TLS already active)\r\n"; log.Printf("WARN: [%s] STARTTLS when TLS active.", clientIP)
			} else if state.cfg.TLSCertPath == "" || state.cfg.TLSKeyPath == "" { response = "454 TLS not available (server not configured)\r\n"; log.Printf("WARN: [%s] STARTTLS when not configured.", clientIP)
			} else {
				cert, errTLS := tls.LoadX509KeyPair(state.cfg.TLSCertPath, state.cfg.TLSKeyPath)
				if errTLS != nil { log.Printf("ERROR: [%s] Error loading TLS cert/key: %v", clientIP, errTLS); response = "454 TLS not available (cert/key error)\r\n"
				} else {
					currentConn.Write([]byte("220 Ready to start TLS\r\n")); log.Printf("SENT [%s]: 220 Ready to start TLS", clientIP)
					tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
					tlsConn := tls.Server(currentConn, tlsConfig)
					log.Printf("INFO: [%s] Starting TLS handshake...", clientIP)
					errTLS = tlsConn.Handshake(); if errTLS != nil { log.Printf("ERROR: [%s] TLS handshake failed: %v", clientIP, errTLS); return }
					currentConn = tlsConn; reader = bufio.NewReader(currentConn)
					state.isTls = true; state.hasSeenHelo = false; state.clientHostname = ""; state.mailFromAddress = ""; state.rcptToAddresses = make([]string, 0)
					state.isInDataMode = false; state.isAuthenticated = false; state.authenticatedUser = ""
					log.Printf("INFO: [%s] Connection upgraded to TLS. Session state reset.", clientIP); continue
				}
			}
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "DATA":
			if len(state.rcptToAddresses) == 0 { response = "503 Bad sequence (RCPT TO first)\r\n"; log.Printf("WARN: [%s] DATA before RCPT TO.", clientIP)
			} else {
				state.isInDataMode = true; response = "354 Start mail input; end with <CRLF>.<CRLF>\r\n"
				log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response)); currentConn.Write([]byte(response))
				var emailBody strings.Builder
				for {
					state.CommandsProcessed++; if cfg.RateLimitEnable && cfg.MaxCommandsPerSession > 0 && state.CommandsProcessed > cfg.MaxCommandsPerSession {
						errMsg := fmt.Sprintf("421 %s Service not available, too many commands. Closing.\r\n", cfg.ServerHostname); log.Printf("WARN: [%s] %s", clientIP, strings.TrimSpace(errMsg)); _, _ = currentConn.Write([]byte(errMsg)); return }
					dataLine, errRead := reader.ReadString('\n'); if errRead != nil { log.Printf("ERROR: [%s] Error reading data: %s", clientIP, errRead.Error()); return }
					// Log first few chars of data line for privacy, or just a generic message
					// log.Printf("RECV_DATA [%s]: %s", clientIP, strings.TrimRight(dataLine, "\r\n"))
					if strings.TrimSpace(dataLine) == "." { break }
					if strings.HasPrefix(dataLine, "..") { emailBody.WriteString(dataLine[1:]) } else { emailBody.WriteString(dataLine) }
				}
				state.isInDataMode = false; mailDir := state.cfg.MailDir
				if _, errStat := os.Stat(mailDir); os.IsNotExist(errStat) {
					log.Printf("INFO: [%s] MailDir %s not found, creating.", clientIP, mailDir)
					if errMkdir := os.MkdirAll(mailDir, 0750); errMkdir != nil {
						log.Printf("ERROR: [%s] Failed to create MailDir %s: %v", clientIP, mailDir, errMkdir); response = "451 Error in processing (cannot create mail dir)\r\n"; break
					}
				}
				timestamp := time.Now().Format("20060102150405"); randomBytes := make([]byte, 4); _, _ = rand.Read(randomBytes)
				messageID := fmt.Sprintf("%s_%s", timestamp, hex.EncodeToString(randomBytes))
				emlFilePath := filepath.Join(mailDir, messageID+".eml"); metaFilePath := filepath.Join(mailDir, messageID+".meta")
				if errWrite := os.WriteFile(emlFilePath, []byte(emailBody.String()), 0640); errWrite != nil {
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
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "RSET":
			log.Printf("INFO: [%s] Session reset by RSET.", clientIP)
			state.resetMailState(); state.isAuthenticated = false; state.authenticatedUser = ""; state.hasSeenHelo = false; state.clientHostname = ""
			log.Printf("INFO: [%s] Auth and HELO state reset by RSET.", clientIP)
			response = "250 OK\r\n"
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		case "NOOP":
			response = "250 OK\r\n"
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		default:
			response = "500 Syntax error, command unrecognized\r\n"
			log.Printf("WARN: [%s] Unknown command: %s", clientIP, commandLine)
			log.Printf("SENT [%s]: %s", clientIP, strings.TrimSpace(response))
			currentConn.Write([]byte(response))
		}
	}
}
