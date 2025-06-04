# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files for later outbound delivery.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features like robust TLS certificate validation for client connections (though server cert verification for outbound is configurable), comprehensive spam filtering, and advanced performance optimizations found in production-grade mail servers. The queuing and delivery system is also rudimentary.

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
*   Configuration is managed via a TOML file (`config.toml`).
*   Provides a basic Command Line Interface (CLI).

## Prerequisites

*   Go programming language (version 1.21 or later recommended).
*   OpenSSL (optional, for generating self-signed certificates, DKIM keys, and testing with `s_client`).

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
    *   `QueueScanInterval` (string): How often the queue processor scans for pending emails. Uses Go's `time.ParseDuration` format (e.g., "30s", "2m", "1h"). Defaults to "30s".
    *   `DefaultRetryInterval` (string): Base delay used for the first retry after a temporary delivery failure. This interval is the starting point for exponential backoff. Uses Go's `time.ParseDuration` format. Defaults to "5m".
    *   `MaxRetryInterval` (string): The maximum possible delay for a single retry attempt, acting as a cap for the exponential backoff calculation. Uses Go's `time.ParseDuration` format. Defaults to "12h".
    *   `MaxDeliveryAttempts` (int): Maximum number of delivery attempts for an email before it's considered permanently failed (especially for temporary or network errors). Defaults to 5.
    *   `DKIMEnable`, `DKIMDomain`, `DKIMSelector`, `DKIMPrivateKeyPath`, `DKIMHeaders`: For DKIM signing of outgoing mail.
    *   `OutboundSTARTTLSPolicy` (string): Defines if/how STARTTLS is used for *outgoing* mail. Options: "opportunistic" (default), "mandatory", "disabled".
    *   `OutboundTLSVerifyCert` (boolean): If `true` (default), the client verifies remote server certificates during STARTTLS for *outgoing* mail. Setting to `false` is insecure.
    *   `OutboundRelayHost` (string): Address (e.g., "smtp.example.com:587") of an SMTP relay/smarthost. If set, MX lookups are bypassed, and all mail goes through this relay.
    *   `OutboundRelayUsername` (string): Username for authenticating to the `OutboundRelayHost`.
    *   `OutboundRelayPassword` (string): Password for authenticating to the `OutboundRelayHost`.

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
    ```

## Email Authentication Setup (For Mail Sent *By* GoSMTPServer)
(This section remains largely as is)
...

## Setting up DKIM Signing (for Outgoing Email)
(This section remains largely as is)
...

## SPF (Sender Policy Framework) Considerations
(This section remains largely as is)
...

## Setting up TLS (STARTTLS) for *Incoming* Connections
(This section remains largely as is)
...

## Setting up SMTP Authentication for *Incoming* Connections
(This section remains largely as is)
...

## Mail Queuing and Outbound Delivery

(Overview, Queue Structure sections remain largely as is)
...

### Queue Processor
A background goroutine (the "queue processor") is started when the server launches. It periodically scans the `MailDir` for `.meta` files.
*   **Scanning**: The interval for scanning is defined by `QueueScanInterval` in `config.toml`.
*   **Processing**: For each message that is due for a delivery attempt (based on its `NextAttemptTime` metadata):
    1.  The processor reads the metadata and the corresponding `.eml` file.
    2.  It attempts to deliver the email using the server's built-in outbound SMTP client.
*   **Delivery Outcomes & Error Handling**:
    *   **Success**: If delivery is successful, the `.eml` and `.meta` files are deleted.
    *   **Permanent SMTP Error (5xx codes)**: If the outbound client reports a permanent SMTP error (e.g., a 550 "User unknown" from the remote server), the queue processor recognizes this. The email is immediately moved to the `failed/` directory, and no further retries for this message will occur. The specific SMTP error is logged in the metadata.
    *   **Temporary SMTP Error (4xx codes) or Network/Other Errors**: If delivery fails with a temporary SMTP error (e.g., a 421 "Service not available") or a general network error (e.g., connection timeout), the `AttemptCount` in the metadata is incremented, and the error is recorded. The `NextAttemptTime` is then rescheduled using an **exponential backoff** strategy:
        *   The interval for the first retry (after the initial attempt fails, so `AttemptCount` in metadata becomes 1) is `DefaultRetryInterval`.
        *   For subsequent retries, the interval is calculated as `DefaultRetryInterval * 2^(AttemptCount-1)`. For example, if `DefaultRetryInterval` is 5 minutes:
            *   1st retry: 5m
            *   2nd retry: 10m
            *   3rd retry: 20m
            *   ...and so on.
        *   This calculated retry interval is capped at `MaxRetryInterval` (e.g., if `MaxRetryInterval` is "12h", the delay won't exceed 12 hours for any single retry).
        *   The metadata file is updated with this new `NextAttemptTime`.
    *   **Maximum Attempts Reached**: If an email repeatedly fails with temporary/network errors and its `AttemptCount` reaches `MaxDeliveryAttempts`, it is then considered permanently failed and moved to the `failed/` subdirectory.

(Outbound SMTP Client, Monitoring the Queue sections remain largely as is)
...

### Outbound Connection Security (STARTTLS and Relay Authentication)
(This section remains largely as is)
...

### Limitations of the Current Queuing & Delivery System
*   **Retry Logic**: Uses exponential backoff with configurable base (`DefaultRetryInterval`), cap (`MaxRetryInterval`), and total attempts (`MaxDeliveryAttempts`). It distinguishes between permanent (5xx) and temporary (4xx) SMTP errors from remote servers to guide retry decisions. However, it does not parse specific SMTP *sub-codes* (e.g., 5.1.1 vs 5.7.1) for more nuanced behavior, and all non-SMTP (network) errors are treated as generic temporary failures subject to the same retry logic.
*   **Single Queue**: No prioritization or per-domain separation.
*   **Outbound Recipient Handling**: Processes one recipient per transaction for direct MX. Relays handle all recipients.
*   **Outbound Client Security**:
    *   STARTTLS policies (opportunistic, mandatory, disabled) and certificate verification are implemented for direct MX and relay connections.
    *   SMTP AUTH (PLAIN, LOGIN) to a configured relay is implemented and only attempted over TLS.
    *   **SMTP AUTH for direct MX deliveries (non-relay) is not implemented.**
*   **Not for High Volume/Critical Deliveries**.

## Testing Outbound Security Features
(This section remains largely as is)
...

## Security Notes
(This section remains largely as is)
...

## Limitations
(This section is now primarily covered by "Limitations of the Current Queuing & Delivery System". This top-level section can be kept for very general points or removed if redundant.)
*   **Incoming Mail Security**: Opportunistic STARTTLS; `AUTH PLAIN`/`LOGIN` over TLS. No mTLS.
*   **Queuing & Outbound Delivery System**: See specific limitations under that section.
*   **DKIM**: Uses `github.com/toorop/go-dkim`. Canonicalization is "relaxed/relaxed".
*   **General**: Basic error handling for complex SMTP edge cases. Simple concurrency model for incoming connections. Queue processor is single-threaded (processes one email at a time during each scan).

## Future Enhancements (Potential)
*   **Outbound Delivery**:
    *   Implement SMTP AUTH for direct outbound connections (non-relay).
    *   Support for more outbound AUTH mechanisms (e.g., CRAM-MD5).
    *   Client certificate authentication for outbound TLS.
    *   Parsing specific SMTP error sub-codes (e.g., 5.1.1 vs 5.7.1) for more nuanced retry/failure decisions.
*   **Queuing**:
    *   Separate queues per destination domain or priority.
    *   More sophisticated DSN (Delivery Status Notification) parsing and bounce handling.
*   **Incoming Mail**:
    *   Implement an option for an Implicit TLS listener (SMTPS on a dedicated port).
*   **General**:
    *   Spam/virus filter hooks.
    *   Daemonization/background running.
    *   More detailed and configurable logging levels (e.g., DEBUG, INFO, WARN, ERROR).
    *   Rate limiting and connection controls for incoming mail.
    *   CLI tools for queue management (e.g., view queue, force retry, delete message).

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
