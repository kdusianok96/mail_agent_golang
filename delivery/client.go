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
)

const clientLogPrefix = "SMTP_CLIENT" // For distinguishing client logs

// Email holds the data for an email to be sent.
type Email struct {
	From     string
	To       []string // For now, client handles one recipient from this list
	Data     []byte   // Raw email content (headers + body), CRLF line endings expected
	Hostname string   // Hostname of our server, for EHLO
}

// SendEmail attempts to deliver the email to the recipient's mail server.
func SendEmail(email *Email) error {
	if len(email.To) == 0 {
		return fmt.Errorf("no recipients specified")
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
		err = performSMTPTransaction(conn, email, recipient, targetHost) // Pass targetHost for logging
		if err != nil {
			log.Printf("WARN: %s: SMTP transaction failed with %s: %v", clientLogPrefix, targetHost, err)
			// conn.Close() is handled by defer in performSMTPTransaction
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

	if expectedCodePrefix != "" && !strings.HasPrefix(response, expectedCodePrefix) {
		return response, fmt.Errorf("unexpected response for command %s from %s: got '%s', expected prefix '%s'", command, mxHost, response, expectedCodePrefix)
	}
	return response, nil
}

// performSMTPTransaction handles the SMTP conversation after a connection is established.
func performSMTPTransaction(conn net.Conn, email *Email, recipient string, mxHost string) error {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	var lastCommand string // For error reporting

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

	// 2. EHLO
	lastCommand = fmt.Sprintf("EHLO %s", email.Hostname)
	_, err = sendCommandAndReadResponse(writer, reader, mxHost, lastCommand, "250")
	if err != nil {
		return fmt.Errorf("%s: [OUTBOUND %s] EHLO command failed: %w", clientLogPrefix, mxHost, err)
	}

	// 3. MAIL FROM
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
