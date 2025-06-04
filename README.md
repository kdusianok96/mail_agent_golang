# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files for later outbound delivery.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features like robust TLS certificate validation, comprehensive spam filtering, and advanced performance optimizations found in production-grade mail servers. The queuing and delivery system is also rudimentary.

## Features

*   Supports common SMTP commands: `HELO`/`EHLO`, `MAIL FROM`, `RCPT TO`, `DATA`, `RSET`, `QUIT`, `NOOP`.
*   STARTTLS support for opportunistic TLS encryption for incoming connections.
*   SMTP Authentication (`AUTH PLAIN`, `AUTH LOGIN`) over TLS to control mail sending.
*   On-disk mail queuing for asynchronous outbound delivery.
*   Background queue processor with configurable retries and permanent failure handling.
*   Basic outbound SMTP client with MX lookup.
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
    *   `ServerHostname`: Used in SMTP greetings and as the `EHLO` name for outbound mail. Critical for SPF alignment.
    *   `MailDir`: Directory for the mail queue and its subdirectories (`corrupt`, `failed`).
    *   `TLSCertPath`, `TLSKeyPath`: For enabling STARTTLS on incoming connections.
    *   `RequireAuth`: If `true`, enforces SMTP AUTH for incoming mail that is to be relayed/sent.
    *   `[Users]`: Defines usernames and bcrypt-hashed passwords for SMTP AUTH.
    *   `QueueScanInterval`, `DefaultRetryInterval`, `MaxDeliveryAttempts`: Control queue processing behavior.
    *   `DKIMEnable`, `DKIMDomain`, `DKIMSelector`, `DKIMPrivateKeyPath`, `DKIMHeaders`: For DKIM signing of outgoing mail.

    **Default `config.toml` example (see comments within for details):**
    ```toml
    ListenInterface = "0.0.0.0"
    ListenPort = 2525
    ServerHostname = "gosmtp.example.com" # Should be an FQDN resolving to your server's IP
    MailDir = "maildata"

    TLSCertPath = "server.crt"
    TLSKeyPath = "server.key"

    [Users]
    # testuser1 = "$2a$10$YourBcryptHashForUser1PasswordGoesHere"

    RequireAuth = false

    QueueScanInterval = "30s"
    DefaultRetryInterval = "5m"
    MaxDeliveryAttempts = 5

    DKIMEnable = false
    DKIMDomain = "yourdomain.com"
    DKIMSelector = "default"
    DKIMPrivateKeyPath = "/etc/gosmtpd/dkim/private.key"
    # DKIMHeaders = ["From", "To", "Cc", "Subject", "Date", "Message-ID"]
    ```

## Email Authentication Setup

Properly setting up email authentication (SPF and DKIM) is crucial for deliverability and sender reputation.

### Setting up DKIM Signing (for Outgoing Email)

DKIM allows GoSMTPServer to digitally sign outgoing emails. Recipient servers can verify this signature using a public key published in your domain's DNS.

**1. Configuration (in `config.toml`):**
*   `DKIMEnable = true`: Enables signing.
*   `DKIMDomain`: The domain you're signing for (e.g., `example.com` if sending from `user@example.com`).
*   `DKIMSelector`: A name to identify the key (e.g., `default`, `dkim2024`). Used in DNS.
*   `DKIMPrivateKeyPath`: Path to the RSA private key file.
*   `DKIMHeaders` (Optional): Specific headers to sign. The `github.com/toorop/go-dkim` library used has sensible defaults if this is omitted.

**2. Generate DKIM Key Pair:**
   Use OpenSSL:
   ```bash
   # Generate a 2048-bit RSA private key
   openssl genrsa -out dkim.private.key 2048

   # Extract the public key
   openssl rsa -in dkim.private.key -pubout -out dkim.public.key
   ```
   *   Secure `dkim.private.key` and set its path in `DKIMPrivateKeyPath`.
   *   The content of `dkim.public.key` is needed for DNS.

**3. Publish Public Key in DNS:**
   Create a TXT record in your DNS for the `DKIMDomain`:
   *   **Record Name:** `selector._domainkey.yourdomain.com` (e.g., `default._domainkey.example.com`).
   *   **Record Value (TXT Content):** `v=DKIM1; k=rsa; p=<base64_public_key>`
     The `p=` tag value is the base64-encoded content of `dkim.public.key`, with `-----BEGIN PUBLIC KEY-----`, `-----END PUBLIC KEY-----`, and all newlines removed, concatenated into a single string.
     *Example `p=` value (truncated):* `MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAy...AQAB`
     *Note:* Some DNS providers require splitting long `p=` values into multiple quoted strings within the same TXT record.

**4. Verifying DKIM Signing:**
*   After enabling DKIM and configuring it correctly, send an email from GoSMTPServer to an external email service (e.g., Gmail, Outlook.com, or a dedicated DKIM validator like those found on dmarcian, AppMailDev, etc.).
*   **Check Email Headers**: In the received email, view the original message or full headers.
    *   Look for a `DKIM-Signature` header. Its presence indicates the email was signed by GoSMTPServer.
    *   Look for an `Authentication-Results` header. This header is often added by the receiving mail system and will show the DKIM verification status (e.g., `dkim=pass header.d=yourdomain.com`).
*   **Online Validators**: Use an online DKIM record checker or email validator to test your setup by sending an email to a test address they provide.

### SPF (Sender Policy Framework) Considerations

SPF allows recipient servers to verify that an email from your domain was sent by an authorized IP address.

**1. `ServerHostname` in `config.toml`**:
    *   The hostname used by GoSMTPServer in its `EHLO` command for outbound mail is set by `ServerHostname`.
    *   This should be an FQDN (e.g., `mail.yourdomain.com`) that has an A (and/or AAAA) record in DNS pointing to your server's public sending IP.

**2. SPF TXT Record for Sending Domains**:
    *   The domain part of your `MAIL FROM` addresses (e.g., `yourdomain.com`) needs an SPF TXT record in DNS.
    *   This record must authorize your GoSMTPServer's IP address.
    *   **Example SPF TXT Record**: For `yourdomain.com`, if your server's IP is `198.51.100.123` and `ServerHostname` is `mail.yourserver.com`:
        ```dns
        yourdomain.com. IN TXT "v=spf1 ip4:198.51.100.123 a:mail.yourserver.com -all"
        ```
        *   `ip4:198.51.100.123`: Directly authorizes the IP.
        *   `a:mail.yourserver.com`: Authorizes the IP `mail.yourserver.com` resolves to.
        *   `-all`: Hard fail for non-authorized sources. `~all` (soft fail) is another option.
    *   Consult SPF documentation (RFC7208) for complex setups (e.g., including third-party senders).

**3. HELO/EHLO Hostname Check**:
    *   Some receivers check the `EHLO` hostname. An SPF record on the domain of `ServerHostname` (e.g., `yourserver.com` if hostname is `mail.yourserver.com`) authorizing the server's IP can be beneficial.

**No Incoming SPF Checks**: GoSMTPServer does not perform SPF checks on *incoming* mail.

## Setting up TLS (STARTTLS)
(This section remains largely as is, focusing on incoming STARTTLS)
...

## Setting up SMTP Authentication
(This section remains largely as is, focusing on incoming AUTH)
...

## Mail Queuing and Outbound Delivery
(This section remains largely as is)
...

## Building the Server
...

## Running the Server
...

## CLI Commands
...

## Testing Manually
(This section remains largely as is, focusing on testing incoming features)
...

## Security Notes
*   **TLS for Incoming AUTH**: SMTP AUTH for incoming mail is only advertised after STARTTLS.
*   **Bcrypt Hashes**: Passwords in `config.toml` must be bcrypt hashes.
*   **Self-Signed Certificates**: For testing incoming TLS only.
*   **User Storage**: User credentials in `config.toml` is basic.
*   **Open Relay (Incoming)**: If `RequireAuth = false` or no users configured, the server can be an open relay.
*   **Outbound Security**: The outbound client currently does **not** use STARTTLS or SMTP AUTH. DKIM signing is available.
*   **DKIM Key Security**: Protect your DKIM private key file.

## Limitations
*   **Incoming Mail Security**: Opportunistic STARTTLS; `AUTH PLAIN`/`LOGIN` over TLS. No mTLS.
*   **Queuing & Outbound Delivery**: Basic retry, single queue. **Outbound client does not use STARTTLS/AUTH.**
*   **DKIM**: Uses `github.com/toorop/go-dkim`. Canonicalization is "relaxed/relaxed".
*   **General**: Basic error handling, simple concurrency.

## Future Enhancements (Potential)
*   **Outbound Delivery**: Implement STARTTLS and SMTP AUTH for the outbound client.
*   **Queuing**: More sophisticated retry (exponential backoff), per-domain queues.
*   **Incoming Mail**: Implicit TLS (SMTPS).
*   **General**: Spam/virus filter hooks, daemonization, advanced logging, rate limiting.

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
