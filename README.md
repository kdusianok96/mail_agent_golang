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
*   Basic rate limiting for incoming connections:
    *   Max concurrent connections per IP (implemented).
    *   Max commands per session (implemented).
    *   Max recipients per message (implemented).
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
    *   `QueueScanInterval`, `DefaultRetryInterval`, `MaxRetryInterval`, `MaxDeliveryAttempts`: Control queue processing behavior.
    *   `DKIMEnable`, `DKIMDomain`, `DKIMSelector`, `DKIMPrivateKeyPath`, `DKIMHeaders`: For DKIM signing of outgoing mail.
    *   `OutboundSTARTTLSPolicy`, `OutboundTLSVerifyCert`: Control STARTTLS usage for *outgoing* mail.
    *   `OutboundRelayHost`, `OutboundRelayUsername`, `OutboundRelayPassword`: For using an outbound SMTP relay.
    *   `RateLimitEnable` (boolean): Enables or disables basic rate limiting for incoming connections. Default is `false`.
    *   `MaxConnectionsPerIP` (int): Maximum concurrent connections from a single IP. `0` means unlimited. Implemented.
    *   `MaxCommandsPerSession` (int): Maximum commands (including command data lines like in AUTH or DATA phases) per SMTP session. `0` means unlimited. Implemented.
    *   `MaxRecipientsPerMessage` (int): Maximum `RCPT TO` commands per message transaction. `0` means unlimited. Implemented.


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
    RateLimitEnable = false # Enable basic rate limiting features

    # Maximum concurrent connections from a single IP address. 0 means unlimited.
    MaxConnectionsPerIP = 10

    # Maximum commands allowed in a single SMTP session (includes command data lines). 0 means unlimited.
    MaxCommandsPerSession = 200

    # Maximum recipients allowed for a single email message (via RCPT TO commands). 0 means unlimited.
    MaxRecipientsPerMessage = 100
    ```

## Rate Limiting (Incoming Connections)

GoSMTPServer provides basic rate limiting capabilities to help mitigate abuse and manage server load from incoming SMTP connections. These features are controlled by settings in `config.toml`.

*   **`RateLimitEnable = true`**: This master switch must be set to `true` to enable any of the rate limiting features described below. If `false` (the default), all other rate limit settings are ignored.

*   **`MaxConnectionsPerIP = 10`**:
    *   Defines the maximum number of concurrent connections allowed from a single IP address.
    *   If set to `0`, there is no limit on concurrent connections from a single IP.
    *   This helps prevent a single client from overwhelming the server by opening too many connections simultaneously.
    *   This limit is enforced when a new connection is accepted.

*   **`MaxCommandsPerSession = 200`**:
    *   Defines the maximum total number of command lines (including lines received during AUTH challenge-response or the DATA phase) that a client can issue within a single SMTP session.
    *   If set to `0`, there is no limit on the number of commands per session.
    *   This can help prevent resource exhaustion. If exceeded, the server sends a `421` error and closes the connection.
    *   This limit is enforced for each command line received from the client.

*   **`MaxRecipientsPerMessage = 100`**:
    *   Defines the maximum number of recipients that can be specified (via multiple `RCPT TO` commands) for a single email message transaction (between one `MAIL FROM` and the corresponding `DATA` command).
    *   If set to `0`, there is no limit on the number of recipients per message.
    *   If this limit is exceeded during a session, the server will respond with a `452 4.5.3 Too many recipients` error for each `RCPT TO` command beyond the limit for that message.
    *   This limit is enforced for each `RCPT TO` command.

**Important Considerations for Rate Limiting:**
*   The current implementation of these limits is basic (e.g., connection counts are managed in-memory).
*   These settings provide a first line of defense. For more advanced abuse prevention, consider integrating with external tools or services.

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
*   **Rate Limiting**: Basic in-memory limits for incoming connections are implemented:
    *   Max concurrent connections per IP.
    *   Max commands per session (counts each line from client, including AUTH/DATA lines).
    *   Max recipients per message (per transaction).
    *   Does not include more advanced features like time-window based rate limiting or distributed tracking.
*   ...
## Future Enhancements (Potential)
*   ...
*   **Rate Limiting**:
    *   More advanced techniques (e.g., time-window based limits, leaky bucket algorithms, persistent storage for limits, integration with fail2ban).
*   ...

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
