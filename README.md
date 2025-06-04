# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files for later outbound delivery.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features like robust TLS certificate validation for client connections (though server cert verification for outbound is configurable), comprehensive spam filtering (though it supports integration with external scanners), and advanced performance optimizations found in production-grade mail servers. The queuing and delivery system is also rudimentary.

## Features

*   Supports common SMTP commands: `HELO`/`EHLO`, `MAIL FROM`, `RCPT TO`, `DATA`, `RSET`, `QUIT`, `NOOP`.
*   STARTTLS support for opportunistic TLS encryption for incoming connections.
*   SMTP Authentication (`AUTH PLAIN`, `AUTH LOGIN`) over TLS to control mail sending *through* this server.
*   On-disk mail queuing for asynchronous outbound delivery.
*   Background queue processor with:
    *   Configurable delivery attempts (`MaxDeliveryAttempts`).
    *   Exponential backoff for retries, with configurable base (`DefaultRetryInterval`) and maximum (`MaxRetryInterval`) intervals.
    *   Enhanced SMTP error code analysis (distinguishing 4xx temporary vs. 5xx permanent errors) to guide retry/failure decisions.
    *   Permanent failure handling (moving to "failed" directory).
*   Outbound SMTP client with:
    *   MX lookup for direct delivery.
    *   Configurable STARTTLS policies ("opportunistic", "mandatory", "disabled") for connections to remote servers (both direct MX and relay).
    *   Configurable TLS certificate verification for outbound STARTTLS.
    *   Support for sending through an authenticated SMTP relay (smarthost), using STARTTLS and AUTH PLAIN/LOGIN.
*   DKIM (DomainKeys Identified Mail) signing for outgoing emails.
*   Basic rate limiting for incoming connections:
    *   Max concurrent connections per IP (implemented).
    *   Max commands per session (implemented).
    *   Max recipients per message (implemented).
*   External script integration for incoming email content filtering (e.g., spam/virus scanning) with configurable actions (reject, add_header, quarantine).
*   Configuration is managed via a TOML file (`config.toml`).
*   Provides a basic Command Line Interface (CLI).

## Prerequisites

*   Go programming language (version 1.21 or later recommended).
*   OpenSSL (optional, for generating self-signed certificates, DKIM keys, and testing with `s_client`).
*   An external script for content scanning if `IncomingFilterEnable` is true (e.g., a shell script calling SpamAssassin `spamc` or ClamAV `clamscan`).

## Setup & Configuration

1.  **Get the Code**:
    ```bash
    git clone <repository_url>
    cd gosmtpserver
    ```
    *(Replace `<repository_url>` with the actual URL of the repository)*

2.  **Configuration File (`config.toml`)**:
    Located in the project root. Key options:

    *   `ListenInterface`, `ListenPort`: For the incoming SMTP server.
    *   `ServerHostname`: Used in SMTP greetings and as the `EHLO` name for its own outbound mail. Critical for SPF alignment.
    *   `MailDir`: Directory for the mail queue and its subdirectories (`corrupt`, `failed`).
    *   `TLSCertPath`, `TLSKeyPath`: For enabling STARTTLS on *incoming* connections.
    *   `RequireAuth`: If `true`, enforces SMTP AUTH for *incoming* mail that is to be relayed/sent.
    *   `[Users]`: Defines usernames and bcrypt-hashed passwords for SMTP AUTH (for *incoming* connections).
    *   `QueueScanInterval`, `DefaultRetryInterval`, `MaxRetryInterval`, `MaxDeliveryAttempts`: Control queue processing behavior.
    *   `DKIMEnable`, `DKIMDomain`, `DKIMSelector`, `DKIMPrivateKeyPath`, `DKIMHeaders`: For DKIM signing of outgoing mail.
    *   `OutboundSTARTTLSPolicy`, `OutboundTLSVerifyCert`: Control STARTTLS usage for *outgoing* mail.
    *   `OutboundRelayHost`, `OutboundRelayUsername`, `OutboundRelayPassword`: For using an outbound SMTP relay.
    *   **Rate Limiting (Incoming Connections)**:
        *   `RateLimitEnable` (boolean): Enables basic rate limiting.
        *   `MaxConnectionsPerIP` (int): Max concurrent connections from one IP.
        *   `MaxCommandsPerSession` (int): Max commands per session.
        *   `MaxRecipientsPerMessage` (int): Max recipients per message transaction.
    *   **Incoming Email Filtering**:
        *   `IncomingFilterEnable` (boolean): Enables external script-based content filtering.
        *   `IncomingFilterScriptPath` (string): Full path to the filter script.
        *   `IncomingFilterScriptTimeout` (string): Timeout for script execution (e.g., "30s"). Defaults to "30s".
        *   `IncomingFilterScriptArgs` ([]string, optional): Arguments to pass to the script.
        *   `IncomingFilterActionOnDetection` (string): Action if script detects an issue ("reject", "add_header" (default), "quarantine").
        *   `IncomingFilterRejectMessage` (string): SMTP message for "reject" action. Defaults to "554 5.7.1 Message content rejected...".
        *   `IncomingFilterHeaderName` (string): Header name for "add_header" action if script gives no STDOUT headers. Defaults to "X-GoSMTPServer-Scan-Result".
        *   `IncomingFilterQuarantineDir` (string): Directory for "quarantine" action. Required if this action is chosen.

    **Default `config.toml` example (see comments within for details):**
    ```toml
    ListenInterface = "0.0.0.0"
    ListenPort = 2525
    ServerHostname = "gosmtp.example.com" # Should be an FQDN resolving to your server's IP
    MailDir = "maildata"

    # --- Incoming Connection Security ---
    TLSCertPath = "server.crt"
    TLSKeyPath = "server.key"

    [Users] # For incoming SMTP AUTH
    # testuser1 = "$2a$10$YourBcryptHashForUser1PasswordGoesHere"

    RequireAuth = false # For incoming mail

    # --- Mail Queue & Delivery ---
    QueueScanInterval = "30s"
    DefaultRetryInterval = "5m"  # Base for exponential backoff
    MaxRetryInterval = "12h"     # Cap for exponential backoff interval
    MaxDeliveryAttempts = 5

    # --- DKIM Signing for Outgoing Mail ---
    DKIMEnable = false
    DKIMDomain = "yourdomain.com"
    DKIMSelector = "default"
    DKIMPrivateKeyPath = "/etc/gosmtpd/dkim/private.key"
    # DKIMHeaders = ["From", "To", "Cc", "Subject", "Date", "Message-ID"]

    # --- Outbound Connection Security & Relay ---
    OutboundSTARTTLSPolicy = "opportunistic"
    OutboundTLSVerifyCert = true

    OutboundRelayHost = ""
    OutboundRelayUsername = ""
    OutboundRelayPassword = ""

    # --- Basic Rate Limiting Configuration (Incoming Connections) ---
    RateLimitEnable = false
    MaxConnectionsPerIP = 10
    MaxCommandsPerSession = 200
    MaxRecipientsPerMessage = 100

    # --- Incoming Email Filtering (Content Scanning) ---
    IncomingFilterEnable = false
    IncomingFilterScriptPath = "/usr/local/bin/email_scanner.sh"
    IncomingFilterScriptTimeout = "30s"
    # IncomingFilterScriptArgs = ["--config", "/etc/scanner.conf"]
    IncomingFilterActionOnDetection = "add_header" # "reject", "add_header", or "quarantine"
    IncomingFilterRejectMessage = "554 5.7.1 Message content rejected by content filter."
    IncomingFilterHeaderName = "X-Content-Scan-Result"
    IncomingFilterQuarantineDir = "/var/spool/gosmtpd/quarantine"
    ```

## Rate Limiting (Incoming Connections)
(This section remains as is)
...

## Incoming Email Filtering (Content Scanning)

GoSMTPServer can integrate with external scripts for content filtering (e.g., spam or virus scanning) of incoming emails. This process occurs after the full email content (`DATA` command) is received from the client but before the email is accepted into the delivery queue or handled otherwise.

**Configuration (`config.toml`):** (See Setup & Configuration for option details)
*   `IncomingFilterEnable`
*   `IncomingFilterScriptPath`
*   `IncomingFilterScriptTimeout`
*   `IncomingFilterScriptArgs`
*   `IncomingFilterActionOnDetection`
*   `IncomingFilterRejectMessage`
*   `IncomingFilterHeaderName`
*   `IncomingFilterQuarantineDir`

**Expected Script Behavior:**

1.  **STDIN**: The script receives the **raw email content** (including all original headers and the full body) via its Standard Input.
2.  **Exit Codes**:
    *   **`0`**: Indicates the email is clean and should be processed normally.
    *   **Non-zero (e.g., `1`)**: Indicates the email is detected as problematic (spam, virus, etc.).
3.  **STDOUT (for "add_header" action)**: If the script detects an issue (non-zero exit) or even if clean, it can output one or more valid email header lines (e.g., `X-Spam-Status: Yes`) to its Standard Output. These are prepended to the email if the action is `"add_header"`, or if the scan was clean and headers were provided.

**Server Behavior (when `IncomingFilterEnable = true` and `IncomingFilterScriptPath` is set):**

*   GoSMTPServer executes the configured script for each received email.
*   **Script Execution Error**: If the script itself fails to execute (e.g., timeout, not found, permissions error), GoSMTPServer logs the error and defaults to a "fail-open" behavior: the original email is allowed through to the queue without modifications from the filter.
*   **Script Indicates Detection (Non-Zero Exit Code)**:
    *   The server logs that a detection occurred and which action will be taken.
    *   Based on `IncomingFilterActionOnDetection`:
        *   `"reject"`: The email is rejected. The SMTP session is terminated with the `IncomingFilterRejectMessage`. The email is not queued.
        *   `"add_header"` (Default): If the script provided headers via STDOUT, they are prepended. If not, and `IncomingFilterHeaderName` is set, a default header (e.g., `X-Content-Scan-Result: Detected`) is prepended. The (potentially modified) email is then queued for delivery.
        *   `"quarantine"`: The email is moved to `IncomingFilterQuarantineDir`. If this directory is not configured or writable, the server logs an error and (currently) fails open by queuing the email normally. If successfully quarantined, the client receives a `250 OK` (as the server accepted the message for special handling), and the email is not queued for normal delivery.
*   **Script Indicates Clean (Exit Code 0)**:
    *   The server logs that the email is clean.
    *   If the script provided any headers on STDOUT (e.g., informational like `X-Spam-Score: 1.2`), these are prepended to the email before it's queued.

**Monitoring Filter Activity / Logs**:
Server logs provide insights into the filtering process. Look for messages prefixed with `FILTER:` from the `filter/scanner.go` package, and SMTP session logs for actions taken:
*   Script execution start: `INFO: [{clientIP}] MessageID: {messageID} - Executing incoming mail filter script: {scriptPath}`
*   Script execution details: `INFO: FILTER: Script '{scriptPath}' execution finished in {duration}. Exit code: {exitCode}`
*   Script STDERR: `INFO: FILTER: Script '{scriptPath}' STDERR: {stderr_output}` (if any)
*   Headers from script: `DEBUG: FILTER: Script '{scriptPath}' provided header for addition: '{HeaderName}: {HeaderValue}'`
*   Final scan result from script: `INFO: FILTER: Script '{scriptPath}' final scan result: {clean/detected/error}. Headers to add: {count}.`
*   Action taken by server:
    *   `INFO: [{clientIP}] MessageID: {messageID} - Rejected by filter script. SMTP Response: {reject_message}`
    *   `INFO: [{clientIP}] MessageID: {messageID} - Added {count} headers from filter script.`
    *   `INFO: [{clientIP}] MessageID: {messageID} - Added default detection header: {HeaderName}`
    *   `INFO: [{clientIP}] MessageID: {messageID} - Quarantined to {quarantine_path} due to filter detection.`
*   Script execution error (fail-open): `ERROR: [{clientIP}] MessageID: {messageID} - Filter script execution error: {error_details}. Allowing message through (fail-open).`
*   Quarantine misconfiguration (fail-open): `ERROR: [{clientIP}] MessageID: {messageID} - Action 'quarantine' but IncomingFilterQuarantineDir is not set. Allowing message through...`

**Example Filter Script (`scanner.sh`)**:
This is a very basic example using `spamc` (SpamAssassin client). Ensure `spamd` is running.
```bash
#!/bin/bash
# Example email filter script for GoSMTPServer

# Read email from STDIN into a temporary file
TMP_EMAIL=$(mktemp)
trap 'rm -f "$TMP_EMAIL"' EXIT # Ensure temp file is deleted on exit

cat > "$TMP_EMAIL"

# Example: Using SpamAssassin's spamc client
# spamc exits 0 if not spam, 1 if spam.
# It can also add X-Spam-* headers directly to the output email if desired,
# or just return an exit code and headers via STDOUT.

# This example focuses on getting exit code and simple STDOUT headers.
# For spamc to output headers to STDOUT that we can capture, it's a bit tricky
# as it usually modifies the email directly or just gives a report.
# A more robust script might parse spamc's report output or use its header adding features
# and then reconstruct headers for GoSMTPServer if needed.

# Simple check: just get the exit code.
spamc -c < "$TMP_EMAIL" >/dev/null # -c just checks, doesn't output modified mail
SPAMC_EXIT_CODE=$?

if [ $SPAMC_EXIT_CODE -eq 1 ]; then
    # Spam detected by spamc
    echo "X-Spam-Flag: YES"
    echo "X-Spam-Level: *****" # Example, actual score not captured here
    exit 1 # Signal detection
elif [ $SPAMC_EXIT_CODE -eq 0 ]; then
    # Clean according to spamc
    echo "X-Spam-Flag: NO"
    exit 0 # Signal clean
else
    # Some other error with spamc itself
    echo "X-Filter-Error: spamc exited with $SPAMC_EXIT_CODE"
    exit 0 # Treat spamc error as "clean" for fail-open, but add info header
fi
```
*   Make it executable: `chmod +x scanner.sh`.
*   Configure `IncomingFilterScriptPath` in `config.toml` to point to this script.
*   **Disclaimer**: This is a rudimentary example. Real-world scripts should have more robust error handling, logging, and potentially more sophisticated ways to interact with scanning tools and pass information back.

## Email Authentication Setup (For Mail Sent *By* GoSMTPServer)
...
## Setting up DKIM Signing (for Outgoing Email)
...
## SPF (Sender Policy Framework) Considerations
...
## Setting up TLS (STARTTLS) for *Incoming* Connections
...
## Setting up SMTP Authentication for *Incoming* Connections
...
## Mail Queuing and Outbound Delivery
...
## Testing Outbound Security Features
...
## Security Notes
...
## Limitations
*   ...
*   **Incoming Email Filtering**:
    *   Integrates with external scripts for content scanning via STDIN/STDOUT/exit-codes.
    *   Actions include reject, add_header, and quarantine.
    *   Script execution errors (timeout, script not found, etc.) currently result in a "fail-open" behavior (email is allowed through without filter modification).
    *   Quarantine directory misconfiguration also currently leads to "fail-open".
    *   Basic STDOUT parsing for headers (one header per line `Name: Value`); does not support complex script interactions or body modification by the script.
*   **Rate Limiting**: Basic in-memory limits are implemented...
*   ...
## Future Enhancements (Potential)
*   ...
*   **Incoming Email Filtering**:
    *   Configurable behavior on script execution error (e.g., fail-close, temporary deferral).
    *   Allow script to modify email body.
    *   More detailed parsing of script STDOUT (e.g., for specific instructions beyond headers).
    *   Support for multiple filter scripts in a chain.
*   **Rate Limiting**:
    *   More advanced techniques...
*   ...

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
