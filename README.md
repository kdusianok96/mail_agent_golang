# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files for later outbound delivery.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features like robust TLS certificate validation for client connections (though server cert verification for outbound is configurable), comprehensive spam filtering, and advanced performance optimizations found in production-grade mail servers. The queuing and delivery system is also rudimentary.

## Features

*   Supports common SMTP commands: `HELO`/`EHLO`, `MAIL FROM`, `RCPT TO`, `DATA`, `RSET`, `QUIT`, `NOOP`.
*   STARTTLS support for opportunistic TLS encryption for incoming connections.
*   SMTP Authentication (`AUTH PLAIN`, `AUTH LOGIN`) over TLS to control mail sending *through* this server.
*   On-disk mail queuing for asynchronous outbound delivery.
*   Background queue processor with configurable retries and permanent failure handling.
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
    *   `QueueScanInterval`, `DefaultRetryInterval`, `MaxDeliveryAttempts`: Control queue processing behavior.
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
    DefaultRetryInterval = "5m"
    MaxDeliveryAttempts = 5

    # --- DKIM Signing for Outgoing Mail ---
    DKIMEnable = false
    DKIMDomain = "yourdomain.com"
    DKIMSelector = "default"
    DKIMPrivateKeyPath = "/etc/gosmtpd/dkim/private.key"
    # DKIMHeaders = ["From", "To", "Cc", "Subject", "Date", "Message-ID"]

    # --- Outbound Connection Security & Relay ---
    # Policy for using STARTTLS when sending to direct MX servers or a relay.
    OutboundSTARTTLSPolicy = "opportunistic" # "opportunistic", "mandatory", or "disabled"
    # Whether to verify the TLS certificate of the remote server during outbound STARTTLS. Highly recommended.
    OutboundTLSVerifyCert = true

    # Optional: Configure an outbound relay (smarthost).
    # If OutboundRelayHost is set, all outgoing emails bypass MX lookup and use this relay.
    OutboundRelayHost = "" # e.g., "smtp.yourprovider.com:587"
    OutboundRelayUsername = ""
    OutboundRelayPassword = ""
    # Note: Storing passwords in plaintext here has security implications. Restrict config file access.
    ```

## Email Authentication Setup (For Mail Sent *By* GoSMTPServer)

Properly setting up email authentication (DKIM and SPF) for emails *sent by* GoSMTPServer is crucial for deliverability and sender reputation.

### Setting up DKIM Signing (for Outgoing Email)
(This section remains largely as is - it describes how to set up DKIM keys and DNS records.)
...
**4. Verifying DKIM Signing:**
*   After enabling DKIM and configuring it correctly, GoSMTPServer will attempt to sign outgoing emails processed by the queue.
*   Send an email from a client through GoSMTPServer that will then be relayed or delivered outbound.
*   Check the email headers at the final destination (e.g., Gmail, Outlook.com). Look for `DKIM-Signature` and `Authentication-Results` headers indicating `dkim=pass`.
*   Use online DKIM validators by sending an email to a test address they provide.

### SPF (Sender Policy Framework) Considerations
(This section remains largely as is - it describes how to set up SPF DNS records.)
...

## Setting up TLS (STARTTLS) for *Incoming* Connections
(This section remains largely as is)
...

## Setting up SMTP Authentication for *Incoming* Connections
(This section remains largely as is)
...

## Mail Queuing and Outbound Delivery

(Overview, Queue Structure, Queue Processor, Monitoring the Queue sections remain largely as is, with minor wording updates to reflect implemented features.)
...

### Outbound Connection Security (STARTTLS and Relay Authentication)
GoSMTPServer's outbound client (used by the Queue Processor) now implements policies for using STARTTLS and authenticating to a relay/smarthost.

*   **Outbound STARTTLS Policy (`OutboundSTARTTLSPolicy`)**:
    *   `"opportunistic"` (Default): The outbound client attempts STARTTLS if the remote server (direct MX or relay) advertises it. If the handshake fails, the client **falls back to sending in plaintext**.
    *   `"mandatory"`: STARTTLS is required. If the remote server doesn't support STARTTLS or the handshake fails, the delivery attempt to that server fails.
    *   `"disabled"`: STARTTLS is never attempted. Mail is sent in plaintext. **Insecure and not recommended.**

*   **Remote Server Certificate Verification (`OutboundTLSVerifyCert`)**:
    *   `true` (Default): The client verifies the remote server's TLS certificate. If verification fails (e.g., self-signed, expired, mismatched hostname), the TLS handshake fails. This is crucial for preventing MITM attacks.
    *   `false`: **Highly insecure.** Disables certificate verification. Only for specific testing with trusted networks or known self-signed certificates.

*   **Using an Outbound Relay (Smarthost)**:
    *   If `OutboundRelayHost` is configured, all mail is sent to this host, bypassing MX lookups.
    *   The `OutboundSTARTTLSPolicy` and `OutboundTLSVerifyCert` settings apply to the connection with the relay.
    *   If `OutboundRelayUsername` is also configured, the client will attempt SMTP AUTH (PLAIN or LOGIN, preferring PLAIN) **only if the connection to the relay has been successfully upgraded to TLS**. This protects credentials.
    *   The port in `OutboundRelayHost` (e.g., 587) is important. Port 587 typically expects STARTTLS then AUTH.

### Limitations of the Current Queuing & Delivery System
*   **Retry Logic**: Basic fixed interval. No exponential backoff or per-domain policies.
*   **Single Queue**: No prioritization or per-domain separation.
*   **Outbound Recipient Handling**: Processes one recipient per transaction for direct MX. Relays handle all recipients.
*   **Outbound Client Security**:
    *   STARTTLS policies (opportunistic, mandatory, disabled) and certificate verification are implemented for direct MX and relay connections.
    *   SMTP AUTH (PLAIN, LOGIN) to a configured relay is implemented and only attempted over TLS.
    *   **SMTP AUTH for direct MX deliveries (non-relay) is not implemented.**
*   **Not for High Volume/Critical Deliveries**.

## Testing Outbound Security Features

Verifying outbound security features often requires observing server logs and potentially inspecting emails at the recipient end or using external tools.

1.  **Testing Opportunistic STARTTLS (Direct MX)**:
    *   Set `OutboundSTARTTLSPolicy = "opportunistic"` and `OutboundTLSVerifyCert = true`.
    *   Send an email to a major provider (e.g., Gmail). Check GoSMTPServer logs for:
        *   `SMTP_CLIENT: ... Server supports STARTTLS. Policy: 'opportunistic'. Attempting to upgrade.`
        *   `SMTP_CLIENT: ... TLS handshake successful. Connection upgraded.`
        *   `SMTP_CLIENT: ... Re-issuing EHLO over secure channel.`
    *   At the recipient, inspect headers for `Received` lines. The hop from your server to the next should indicate TLS was used (e.g., `ESMTPS` or similar).

2.  **Testing Mandatory STARTTLS (Direct MX)**:
    *   Set `OutboundSTARTTLSPolicy = "mandatory"`.
    *   **Test A (Failure)**: Attempt to send to a mail server known *not* to support STARTTLS (e.g., a simple local test server without TLS). Delivery should fail. Check logs for:
        *   `SMTP_CLIENT: ... Server does not support STARTTLS and policy is mandatory. Aborting connection.` OR
        *   `SMTP_CLIENT: ... TLS handshake failed ... Policy is mandatory. Aborting connection.`
    *   **Test B (Success)**: Send to a server known to support STARTTLS (like Gmail). It should proceed as in opportunistic mode but would have failed if STARTTLS wasn't available/successful.

3.  **Testing Disabled STARTTLS (Direct MX)**:
    *   Set `OutboundSTARTTLSPolicy = "disabled"`.
    *   Send an email. Logs should show: `SMTP_CLIENT: ... Outbound STARTTLS is disabled by policy. Proceeding in plaintext.`
    *   Received headers at the destination will likely show a non-TLS connection from your server.

4.  **Testing Outbound Relay (Smarthost) with STARTTLS and AUTH**:
    *   Configure `OutboundRelayHost` (e.g., to a service like SendGrid, Mailgun, or a local Postfix/Exim setup requiring AUTH on port 587), `OutboundRelayUsername`, `OutboundRelayPassword`.
    *   Set `OutboundSTARTTLSPolicy` to `"opportunistic"` or `"mandatory"` (most relays on 587 will require STARTTLS).
    *   Set `OutboundTLSVerifyCert = true`.
    *   Send an email. Check GoSMTPServer logs for:
        *   Connection to the relay host.
        *   STARTTLS handshake with the relay.
        *   Re-EHLO to the relay.
        *   `SMTP_CLIENT: ... Attempting SMTP AUTH as relay user ...`
        *   `SMTP_CLIENT: ... Relay supports AUTH PLAIN/LOGIN. Attempting.`
        *   `SMTP_CLIENT: ... AUTH PLAIN/LOGIN successful for user ...`
        *   Successful `MAIL FROM`, `RCPT TO`, `DATA` sequence with the relay.
    *   Verify the email is delivered via the relay.

## Security Notes
*   ... (existing notes remain relevant)
*   **Outbound Relay Password**: Stored in `config.toml`. Restrict file access.
*   **Outbound STARTTLS Verification**: `OutboundTLSVerifyCert = true` is crucial. Setting to `false` is insecure.

## Limitations
*   ... (update based on new implementations)
*   **Queuing & Outbound Delivery System**: Basic retry, single queue. Outbound client implements STARTTLS policies and relay AUTH (PLAIN/LOGIN over TLS). **No SMTP AUTH for direct MX deliveries.**

## Future Enhancements (Potential)
*   **Outbound Delivery**:
    *   Implement SMTP AUTH for direct outbound connections (non-relay).
    *   Support for more outbound AUTH mechanisms (e.g., CRAM-MD5).
    *   Client certificate authentication for outbound TLS.
*   ... (other existing future enhancements)

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
