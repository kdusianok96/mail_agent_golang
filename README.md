# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files for later outbound delivery.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features like robust TLS certificate validation, comprehensive spam filtering, and advanced performance optimizations found in production-grade mail servers. The queuing and delivery system is also rudimentary.

## Features

*   Supports common SMTP commands:
    *   `HELO` / `EHLO`
    *   `MAIL FROM`
    *   `RCPT TO`
    *   `DATA` (including dot-stuffing removal)
    *   `RSET`
    *   `QUIT`
    *   `NOOP`
*   STARTTLS support for opportunistic TLS encryption.
*   SMTP Authentication (`AUTH PLAIN`, `AUTH LOGIN`) over TLS to control mail sending.
*   On-disk mail queuing: Received emails are stored locally for asynchronous outbound delivery.
*   Background queue processor: Periodically scans the queue, attempts delivery, handles retries for temporary failures, and moves emails to a "failed" directory after repeated failures.
*   Basic outbound SMTP client: Performs MX lookups and attempts to deliver emails to recipient mail servers.
*   Configuration is managed via a TOML file (`config.toml`).
*   Provides a basic Command Line Interface (CLI) for starting the server and validating the configuration.

## Prerequisites

*   Go programming language (version 1.21 or later recommended). You can download it from [golang.org](https://golang.org/dl/).
*   OpenSSL (optional, for generating self-signed certificates and testing with `s_client`).

## Setup & Configuration

1.  **Get the Code**:
    Clone the repository to your local machine:
    ```bash
    git clone <repository_url>
    cd gosmtpserver
    ```
    *(Replace `<repository_url>` with the actual URL of the repository)*

2.  **Configuration File (`config.toml`)**:
    The server is configured using a TOML file named `config.toml` located in the root of the project.

    Available options:

    *   `ListenInterface` (string): IP address to listen on (e.g., `"0.0.0.0"` for all, `"127.0.0.1"` for local).
    *   `ListenPort` (int): Port to listen on (e.g., `2525`).
    *   `ServerHostname` (string): Hostname used in SMTP greetings (e.g., `"gosmtp.example.com"`).
    *   `MailDir` (string): Directory to store received emails for queuing and delivery (e.g., `"maildata"`). This directory and its subdirectories (`corrupt`, `failed`) will be created if they don't exist.
    *   `TLSCertPath` (string): Path to TLS certificate file. Enables STARTTLS if both this and `TLSKeyPath` are set.
    *   `TLSKeyPath` (string): Path to TLS private key file. Enables STARTTLS if both this and `TLSCertPath` are set.
    *   `RequireAuth` (boolean): If `true`, requires clients to authenticate via SMTP AUTH (after STARTTLS) before `MAIL FROM` is accepted. This applies only if users are defined in the `[Users]` section. Default is `false`.
    *   `[Users]` (table): Defines usernames and their bcrypt hashed passwords for SMTP AUTH.
    *   `QueueScanInterval` (string): How often the queue processor scans for pending emails. Uses Go's `time.ParseDuration` format (e.g., "30s", "2m", "1h"). Defaults to "30s".
    *   `DefaultRetryInterval` (string): Default delay before retrying an email after a temporary delivery failure. Uses Go's `time.ParseDuration` format. Defaults to "5m".
    *   `MaxDeliveryAttempts` (int): Maximum number of delivery attempts for an email before it's considered permanently failed and moved to the "failed" directory. Defaults to 5.

    **Default `config.toml` example:**
    ```toml
    ListenInterface = "0.0.0.0"
    ListenPort = 2525
    ServerHostname = "gosmtp.example.com"
    MailDir = "maildata"

    # TLS Configuration
    # If TLSCertPath or TLSKeyPath is empty, STARTTLS will be disabled.
    TLSCertPath = "server.crt"
    TLSKeyPath = "server.key"

    # SMTP User Authentication
    # If [Users] is empty or commented out, AUTH capabilities won't be advertised.
    [Users]
    # testuser1 = "$2a$10$YourBcryptHashForUser1PasswordGoesHere"
    # anotheruser = "$2a$10$AnotherBcryptHashForPasswordHere"

    # Require Authentication for Sending
    # If true, and users are defined, clients must AUTH before MAIL FROM.
    RequireAuth = false

    # Outbound Mail Delivery Queue Configuration
    QueueScanInterval = "30s"    # How often the queue processor scans for pending emails.
                                 # Valid time units: "ns", "us" (or "µs"), "ms", "s", "m", "h".
    DefaultRetryInterval = "5m"  # Default delay before retrying a temporarily failed email.
    MaxDeliveryAttempts = 5      # Maximum number of delivery attempts before an email is moved
                                 # to the 'failed' directory. Must be > 0.
    ```

## Mail Queuing and Outbound Delivery

### Overview
When GoSMTPServer receives an email (after the `DATA` command and successful transaction), it doesn't deliver it directly during the SMTP session. Instead, the email is placed into an on-disk queue for asynchronous outbound delivery.

**Queue Structure:**
*   Emails are stored in the directory specified by `MailDir` in `config.toml`. This directory functions as the mail spool or queue.
*   Each queued email consists of two files:
    *   `<MessageID>.eml`: The raw email content, including all headers and the body.
    *   `<MessageID>.meta`: A JSON file containing metadata about the email, such as sender, recipients, received time, delivery attempts, next scheduled attempt, and any last error. The `MessageID` is typically a timestamp combined with a random string.
*   **Subdirectories within `MailDir`**:
    *   `corrupt/`: If a `.meta` file is unparseable, or if its corresponding `.eml` file is missing or unreadable, the problematic file(s) are moved here by the queue processor.
    *   `failed/`: If an email reaches the `MaxDeliveryAttempts` limit, both its `.eml` and `.meta` files are moved here, and no further delivery attempts will be made by the processor.

### Queue Processor
A background goroutine (the "queue processor") is started when the server launches. It periodically scans the `MailDir` for `.meta` files.
*   **Scanning**: The interval for scanning is defined by `QueueScanInterval` in `config.toml`.
*   **Processing**: For each message that is due for a delivery attempt (based on its `NextAttemptTime` metadata):
    1.  The processor reads the metadata and the corresponding `.eml` file.
    2.  It attempts to deliver the email using the server's built-in outbound SMTP client.
*   **Delivery Outcomes**:
    *   **Success**: If delivery to a recipient's mail server is successful, the `.eml` and `.meta` files for that email are deleted from the queue.
    *   **Temporary Failure**: If delivery fails with what is considered a temporary error (e.g., connection issue, 4xx SMTP code from remote server), the `AttemptCount` in the metadata is incremented, the error is recorded, and `NextAttemptTime` is rescheduled based on `DefaultRetryInterval`. The metadata file is updated with this new information.
    *   **Permanent Failure**: If `AttemptCount` reaches `MaxDeliveryAttempts`, the email is considered permanently failed. The `.eml` and `.meta` files are moved to the `failed/` subdirectory for manual inspection.

### Outbound SMTP Client
GoSMTPServer includes a basic outbound SMTP client responsible for the actual delivery of queued emails.
*   It performs MX lookups for the recipient domains to find the appropriate mail servers.
*   It connects to these servers on port 25 and attempts to deliver the email using standard SMTP commands (EHLO, MAIL FROM, RCPT TO, DATA, QUIT).

### Monitoring the Queue / Checking Logs
The primary way to monitor the queue and delivery status is by observing the server's log output (standard output). Key log messages include:

*   **Queuing**: `INFO [...]: Email (MessageID: ...) ... queued successfully. Data: ..., Meta: ...`
*   **Queue Scan**: `INFO: Queue processor tick: scanning for emails...`
*   **Processing File**: `INFO: Processing queue file: <MailDir>/<MessageID>.meta`
*   **Skipping (Retry Delay)**: `INFO: Skipping MessageID: ..., next attempt not due until ...`
*   **Delivery Attempt**: `INFO: Attempting delivery for MessageID: ... (Attempt X/Y) to recipients: ...`
*   **Outbound Client Activity**: Logs prefixed with `SMTP_CLIENT` show details of MX lookups, connection attempts, and SMTP command interactions with remote servers.
*   **Delivery Success**: `INFO: Email MessageID: ... delivered successfully ... Removing from queue.`
*   **Temporary Failure**: `WARN: Temporary delivery failure for MessageID: ... Next attempt at: ...`
*   **Permanent Failure**: `ERROR: Email MessageID: ... failed permanently ... Moving to 'failed' directory.`
*   **Corrupt/Missing Files**: `ERROR: Failed to load metadata... Moving to 'corrupt' directory.` or `ERROR: Missing .eml file for ... Moving metadata to 'corrupt' directory.`

Users should also check the `MailDir/failed/` and `MailDir/corrupt/` directories for emails that require manual inspection or intervention.

### Limitations of the Current Queuing & Delivery System
This is a **basic queuing and delivery system** and has several important limitations:
*   **Simple Retry Logic**: Uses a fixed `DefaultRetryInterval`. No exponential backoff, per-domain retry policies, or parsing of specific SMTP error codes for smarter retries.
*   **Single Queue**: All outgoing mail resides in a single queue directory. There's no prioritization or separate handling for different destination domains.
*   **Outbound Recipient Handling**: The outbound `delivery.SendEmail` function currently processes only the first recipient listed in an email's metadata if multiple recipients were part of the original transaction. Each recipient for multi-recipient emails will effectively be handled as if it were a separate email to that one recipient during delivery processing by the queue. (The metadata stores all original recipients).
*   **No Outbound STARTTLS/AUTH**: The built-in outbound SMTP client does **not** currently use STARTTLS or SMTP AUTH when connecting to remote mail servers. It sends mail in plaintext. This is a significant limitation for sending to many modern mail services.
*   **Basic Error Handling**: While it attempts to classify errors for retries, it's not exhaustive and doesn't deeply parse DSNs (Delivery Status Notifications).
*   **Not for High Volume or Critical Deliveries**: The system is not designed for high email volume, complex routing scenarios, or situations requiring guaranteed delivery with advanced monitoring.
*   **Concurrency**: The queue processor is single-threaded (processes one email at a time from the queue scan).

## Setting up TLS (STARTTLS)

The server supports STARTTLS for upgrading a plain text connection to encrypted TLS for *incoming* connections.
1.  Enable by setting `TLSCertPath` and `TLSKeyPath` in `config.toml`.
2.  If paths are not set, STARTTLS is disabled for incoming connections.
3.  The server advertises `STARTTLS` in `EHLO` if configured.

### Generating Self-Signed Certificates (for testing only)

**For testing only.** Browsers/clients will warn about self-signed certs. Use a CA like Let's Encrypt for production.
```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"
```
*   `server.key` -> `TLSKeyPath`
*   `server.crt` -> `TLSCertPath`

## Setting up SMTP Authentication

Enable SMTP AUTH for *incoming* connections by defining users in the `[Users]` section of `config.toml`.
*   Authentication is only advertised and processed over a TLS-secured connection (after STARTTLS).
*   If `RequireAuth = true` in `config.toml`, clients must authenticate before they can send mail *through* this server.

1.  **Configure Users and Password Hashes**:
    ```toml
    [Users]
    # testuser1 = "$2a$10$YourBcryptHashForUser1PasswordGoesHere"
    ```
    Passwords **must** be stored as bcrypt hashes.

2.  **Generating bcrypt Hashes**:
    Use the provided `genhash.go` utility (or any bcrypt tool):
    ```go
    package main
    import ("fmt"; "log"; "os"; "golang.org/x/crypto/bcrypt")
    func main() {
	if len(os.Args) < 2 { fmt.Println("Usage: go run genhash.go <password>"); os.Exit(1) }
	hashedP, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), bcrypt.DefaultCost)
	if err != nil { log.Fatal(err) }
	fmt.Println(string(hashedP))
    }
    ```
    Compile and run:
    ```bash
    # cd to directory with genhash.go
    # go mod init genhash && go mod tidy
    # go run genhash.go "yourSecretPassword"
    # Output: $2a$10$... (copy this hash to config.toml)
    ```

## Building the Server

```bash
go build -o gosmtpd .
```

## Running the Server

```bash
./gosmtpd start
# With custom config:
./gosmtpd start --config /path/to/your/custom_config.toml
```
Logs are printed to standard output.

## CLI Commands

*   `./gosmtpd start [-c <config_path>]`: Starts the server and the queue processor.
*   `./gosmtpd validate-config [-c <config_path>]`: Validates configuration.
*   `./gosmtpd help`: Shows help.

## Testing Manually

### Telnet (Basic Checks & STARTTLS Initiation)

1.  Connect: `telnet 127.0.0.1 2525` (use your server's IP/port).
2.  Server: `220 gosmtp.example.com Welcome...`
3.  You: `EHLO testclient.com`
    *   Server (if TLS configured): `250-gosmtp.example.com...`, `250-STARTTLS`, ...
4.  You: `STARTTLS`
    *   Server: `220 Ready to start TLS`
    *   Telnet cannot proceed with TLS. Use `openssl s_client` for further testing.

### OpenSSL s_client (Full STARTTLS and AUTH Testing for Incoming Mail)

1.  Connect and initiate STARTTLS:
    ```bash
    # If ListenInterface is 0.0.0.0, use localhost or a specific IP.
    openssl s_client -connect localhost:2525 -starttls smtp -crlf
    ```
    *(For self-signed certs, you might need `-ign_eof` for interactive sessions).*

2.  After TLS handshake details, re-issue `EHLO`:
    ```
    EHLO testclient.tls.com
    ```
    *   Server (if TLS active & users configured): `250-AUTH PLAIN LOGIN`, ...

3.  **Testing `AUTH PLAIN`**:
    *   Credentials format: `authorization_id\0authentication_id\0password` (authzid often empty).
    *   Generate base64: `echo -ne '\0yourusername\0yourpassword' | base64`
        (e.g., output: `AHlvdXJ1c2VybmFtZQB5b3VycGFzc3dvcmQ=`)
    *   Send command:
        ```
        AUTH PLAIN AHlvdXJ1c2VybmFtZQB5b3VycGFzc3dvcmQ=
        ```
    *   Server: `235 2.7.0 Authentication Succeeded` or `535 ... Invalid`.

4.  **Testing `AUTH LOGIN`**:
    *   Generate base64 for username (e.g., `echo -n 'yourusername' | base64` -> `eW91cnVzZXJuYW1l`)
    *   Generate base64 for password (e.g., `echo -n 'yourpassword' | base64` -> `eW91cnBhc3N3b3Jk`)
    *   Interaction:
        ```
        AUTH LOGIN
        ```
        Server: `334 VXNlcm5hbWU6` (Username:)
        ```
        eW91cnVzZXJuYW1l
        ```
        Server: `334 UGFzc3dvcmQ6` (Password:)
        ```
        eW91cnBhc3N3b3Jk
        ```
    *   Server: `235 2.7.0 Authentication Succeeded` or `535 ... Invalid`.

5.  **Sending Email After Successful AUTH**:
    (Required if `RequireAuth = true`)
    ```
    MAIL FROM:<yourusername@example.com>
    RCPT TO:<recipient@somewhere.com>
    DATA
    Subject: Test Email After AUTH

    This email was sent over STARTTLS after successful SMTP AUTH.
    .
    QUIT
    ```
    Check `MailDir` for the saved email (it will now be a `.eml` and `.meta` file pair).

## Security Notes
*   **TLS is Essential for Incoming AUTH**: SMTP AUTH (PLAIN/LOGIN) for incoming connections is only advertised and processed by this server after a successful STARTTLS handshake.
*   **Bcrypt Hashes**: Passwords in `config.toml` must be bcrypt hashes.
*   **Self-Signed Certificates**: Suitable for testing only.
*   **User Storage**: Storing user credentials in the configuration file is basic.
*   **Open Relay**: If `RequireAuth = false` or no users are configured, this server can act as an open relay for *incoming* mail. Configure carefully.
*   **Outbound Security**: The current outbound SMTP client does **not** use STARTTLS or SMTP AUTH. This is a major security limitation for sending mail to external servers.

## Limitations

*   **Incoming Mail Security**:
    *   Opportunistic TLS (STARTTLS) for incoming connections relies on client initiation.
    *   SMTP Authentication Scope for incoming connections: `AUTH PLAIN` and `AUTH LOGIN` are supported over TLS.
    *   Client Certificate Authentication (mTLS) for incoming connections: Not supported.
*   **Queuing and Outbound Delivery System (see "Mail Queuing and Outbound Delivery" section for more details)**:
    *   Basic retry logic, single queue, basic remote response parsing.
    *   Outbound client sends to one recipient per transaction.
    *   **No outbound STARTTLS or SMTP AUTH for connections *to* other servers.**
*   **General**:
    *   Basic Error Handling: For complex SMTP scenarios.
    *   Single Goroutine Per Connection (Incoming SMTP): Simple concurrency model.
    *   Queue Processor Concurrency: Processes one email from the queue at a time.

## Future Enhancements (Potential)

*   **Outbound Delivery**:
    *   Implement STARTTLS and SMTP AUTH for the outbound delivery client.
    *   More sophisticated retry mechanisms (e.g., exponential backoff, per-domain retry times).
    *   Enhanced DSN (Delivery Status Notification) parsing and bounce handling.
    *   Separate queues per destination domain or priority.
*   **Incoming Mail**:
    *   Implement an option for an Implicit TLS listener (SMTPS on a dedicated port).
*   **General**:
    *   Add hooks or integration points for spam/virus filtering tools.
    *   Options for daemonization/background running.
    *   More detailed and configurable logging levels.
    *   Rate limiting and connection controls.

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
