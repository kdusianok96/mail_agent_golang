package smtp

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log" // Added for enhanced logging
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-smtp/config" // Added import for config
)

// const serverHostname = "gosmtp.example.com" // Will be replaced by config

type sessionState struct {
	clientHostname  string
	hasSeenHelo     bool
	mailFromAddress string
	rcptToAddresses []string
	isInDataMode    bool // Technically managed by the DATA command flow itself
	cfg             *config.Config // Added to store config
}

func newSessionState(cfg *config.Config) *sessionState {
	return &sessionState{
		rcptToAddresses: make([]string, 0),
		cfg:             cfg,
	}
}

func (s *sessionState) resetMailState() {
	s.mailFromAddress = ""
	s.rcptToAddresses = make([]string, 0)
}

// HandleConnection now accepts a config object
func HandleConnection(conn net.Conn, cfg *config.Config) {
	clientAddr := conn.RemoteAddr().String()
	log.Printf("INFO: New connection from %s", clientAddr)
	defer func() {
		conn.Close()
		log.Printf("INFO: Client disconnected: %s", clientAddr)
	}()

	state := newSessionState(cfg) // Pass config to session state

	welcomeMsg := fmt.Sprintf("220 %s Welcome to GoSMTP\r\n", state.cfg.ServerHostname)
	log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(welcomeMsg))
	conn.Write([]byte(welcomeMsg))

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "use of closed network connection") {
				// log.Printf("INFO: Client %s disconnected (EOF or closed connection)", clientAddr)
				// Defer will handle logging disconnect
			} else {
				log.Printf("ERROR: Error reading from client %s: %s", clientAddr, err.Error())
			}
			return
		}

		commandLine := strings.TrimSpace(line)
		if commandLine == "" {
			continue // Ignore empty lines
		}
		log.Printf("RECV [%s]: %s", clientAddr, commandLine)

		parts := strings.Fields(commandLine)
		if len(parts) == 0 {
			// Should not happen if commandLine is not empty, but good practice
			continue
		}
		command := strings.ToUpper(parts[0])
		var response string // To store and log the response

		switch command {
		case "QUIT":
			response = "221 Bye\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))
			log.Printf("INFO: Client %s issued QUIT command.", clientAddr)
			return
		case "HELO":
			if len(parts) < 2 {
				response = "501 Syntax error in parameters or arguments\r\n"
			} else {
				state.clientHostname = parts[1]
				state.hasSeenHelo = true
				response = fmt.Sprintf("250 %s Hello %s\r\n", state.cfg.ServerHostname, state.clientHostname)
			}
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))
		case "EHLO":
			if len(parts) < 2 {
				response = "501 Syntax error in parameters or arguments\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				conn.Write([]byte(response))
				continue
			}
			state.clientHostname = parts[1]
			state.hasSeenHelo = true
			// For now, EHLO is the same as HELO. Can add capabilities later.
			respBase := fmt.Sprintf("250-%s Hello %s\r\n", state.cfg.ServerHostname, state.clientHostname)
			respPipelinig := "250 PIPELINING\r\n"
			// respSize := "250 SIZE 10240000\r\n"
			// respHelp := "250 HELP\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(respBase))
			conn.Write([]byte(respBase))
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(respPipelinig))
			conn.Write([]byte(respPipelinig))
			// log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(respSize))
			// conn.Write([]byte(respSize))
			// log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(respHelp))
			// conn.Write([]byte(respHelp))
		case "MAIL":
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
			conn.Write([]byte(response))
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
			conn.Write([]byte(response))
		case "DATA":
			if len(state.rcptToAddresses) == 0 {
				response = "503 Bad sequence of commands (RCPT TO first)\r\n"
				log.Printf("WARN [%s]: DATA before RCPT TO", clientAddr)
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				conn.Write([]byte(response))
				continue
			}
			state.isInDataMode = true
			response = "354 Start mail input; end with <CRLF>.<CRLF>\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))

			var emailBody strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("ERROR [%s]: Error reading data: %s", clientAddr, err.Error())
					state.isInDataMode = false
					state.resetMailState()
					return // Abort connection on error during DATA
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
					conn.Write([]byte(response))
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
				conn.Write([]byte(response))
				state.resetMailState()
				continue
			}
			randomString := hex.EncodeToString(randomBytes)
			filename := fmt.Sprintf("%s_%s.eml", timestamp, randomString)
			filePath := filepath.Join(mailDir, filename)

			fullEmailContent := emailBody.String()
			err = os.WriteFile(filePath, []byte(fullEmailContent), 0640)
			if err != nil {
				log.Printf("ERROR [%s]: Error writing email to file %s: %v", clientAddr, filePath, err)
				response = "451 Requested action aborted: local error in processing (cannot write email file)\r\n"
				log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
				conn.Write([]byte(response))
				state.resetMailState()
				continue
			}

			log.Printf("INFO [%s]: Email from %s to %v saved to %s", clientAddr, state.mailFromAddress, state.rcptToAddresses, filePath)
			response = fmt.Sprintf("250 OK: message accepted for delivery (queued as %s)\r\n", filename)
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))
			state.resetMailState()
		case "RSET":
			log.Printf("INFO [%s]: Session reset initiated by RSET command.", clientAddr)
			state.resetMailState()
			state.hasSeenHelo = false
			state.clientHostname = ""
			response = "250 OK\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))
		case "NOOP":
			response = "250 OK\r\n"
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))
		default:
			response = "500 Syntax error, command unrecognized\r\n"
			log.Printf("WARN [%s]: Unknown command: %s", clientAddr, commandLine)
			log.Printf("SENT [%s]: %s", clientAddr, strings.TrimSpace(response))
			conn.Write([]byte(response))
		}
	}
}
