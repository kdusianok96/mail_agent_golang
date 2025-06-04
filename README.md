# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features such as TLS encryption and SMTP authentication, has no comprehensive spam filtering, and does not include performance optimizations found in production-grade mail servers.

## Features

*   Supports common SMTP commands:
    *   `HELO` / `EHLO`
    *   `MAIL FROM`
    *   `RCPT TO`
    *   `DATA` (including dot-stuffing removal)
    *   `RSET`
    *   `QUIT`
    *   `NOOP`
*   Receives emails and stores them as individual `.eml` files in a local directory.
*   Configuration is managed via a TOML file (`config.toml`).
*   Provides a basic Command Line Interface (CLI) for:
    *   Starting the server.
    *   Validating the configuration file.
*   STARTTLS support for opportunistic TLS encryption.

## Prerequisites

*   Go programming language (version 1.21 or later recommended). You can download it from [golang.org](https://golang.org/dl/).

## Setup & Configuration

1.  **Get the Code**:
    Clone the repository to your local machine:
    ```bash
    git clone <repository_url>
    cd gosmtpserver
    ```
    *(Replace `<repository_url>` with the actual URL of the repository)*

2.  **Configuration File (`config.toml`)**:
    The server is configured using a TOML file named `config.toml` located in the root of the project. If it's not found, the server will fail to start (unless a different path is specified via the CLI).

    Here are the available configuration options:

    *   `ListenInterface` (string): The IP address the server should listen on.
        *   Example: `"0.0.0.0"` to listen on all available network interfaces.
        *   Example: `"127.0.0.1"` to listen only on the local loopback interface.
    *   `ListenPort` (int): The port number the server should listen on.
        *   Example: `2525` (a common alternative to the standard SMTP port 25).
    *   `ServerHostname` (string): The hostname the server will use in its SMTP greetings (e.g., in the `220` welcome message and `EHLO` responses).
        *   Example: `"gosmtp.example.com"`
    *   `MailDir` (string): The directory where received emails will be stored as `.eml` files.
        *   Example: `"maildata"`
        *   If this directory does not exist, the server will attempt to create it when the first email is received.
    *   `TLSCertPath` (string): Path to the TLS certificate file (e.g., `server.crt`, `fullchain.pem`). If left empty, or if `TLSKeyPath` is empty, TLS/STARTTLS will be disabled.
        *   Example: `"server.crt"`
    *   `TLSKeyPath` (string): Path to the TLS private key file (e.g., `server.key`, `privkey.pem`). If left empty, or if `TLSCertPath` is empty, TLS/STARTTLS will be disabled.
        *   Example: `"server.key"`
    *   **Note**: If `TLSCertPath` or `TLSKeyPath` is not provided or left empty in the configuration, STARTTLS capability will be disabled, and the server will operate in plain text mode only.

    **Default `config.toml` example:**
    ```toml
    ListenInterface = "0.0.0.0"
    ListenPort = 2525
    ServerHostname = "gosmtp.example.com"
    MailDir = "maildata"

    # TLS Configuration (optional, leave empty or comment out to disable TLS)
    # To enable explicit TLS (on a different port usually) or STARTTLS (on the same port),
    # provide paths to your certificate and private key files.
    # If TLSCertPath or TLSKeyPath is empty, TLS/STARTTLS will be disabled.
    TLSCertPath = "server.crt" # Path to TLS certificate file
    TLSKeyPath = "server.key"  # Path to TLS private key file
    ```

## Setting up TLS (STARTTLS)

The server supports STARTTLS, which allows a plain text connection to be upgraded to an encrypted TLS connection if both the client and server agree.

To enable STARTTLS, you need to provide a TLS certificate and a private key file in the `config.toml`:
*   Set `TLSCertPath` to the path of your TLS certificate file (e.g., `server.crt`, `fullchain.pem`).
*   Set `TLSKeyPath` to the path of your TLS private key file (e.g., `server.key`, `privkey.pem`).

If both paths are correctly configured, the server will advertise the `STARTTLS` capability in its `EHLO` response.

### Generating Self-Signed Certificates (for testing only)

For testing purposes, you can generate a self-signed certificate. **Do not use self-signed certificates in a production environment**, as they are not trusted by default and will cause security warnings in email clients.

You can use `openssl` to generate a key and certificate:
```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"
```
This command creates two files:
*   `server.key`: Your private key. Set `TLSKeyPath = "server.key"` in `config.toml`.
*   `server.crt`: Your self-signed certificate. Set `TLSCertPath = "server.crt"` in `config.toml`.

Remember to replace `"localhost"` with the actual hostname if needed, although for local testing, `localhost` is usually appropriate.

For production use, obtain certificates from a trusted Certificate Authority (CA) like Let's Encrypt or a commercial CA.

## Building the Server

Navigate to the project's root directory and run the following command to build the server executable:

```bash
go build -o gosmtpd .
```
This will create an executable file named `gosmtpd` (or `gosmtpd.exe` on Windows) in the current directory.

## Running the Server

To start the SMTP server, use the `start` command:

```bash
./gosmtpd start
```

By default, it will look for `config.toml` in the current directory. To specify a different configuration file, use the `--config` (or `-c`) flag:

```bash
./gosmtpd start --config /path/to/your/custom_config.toml
```

The server will print log messages to the standard output, including information about incoming connections, received commands, and saved emails.

## CLI Commands

The server provides a few CLI commands:

*   **`gosmtpd start`**: Starts the SMTP server.
    *   Flags:
        *   `--config <path>` or `-c <path>`: Specifies the path to the configuration file (default: `"config.toml"`).

*   **`gosmtpd validate-config`**: Validates the syntax and content of the configuration file.
    *   Flags:
        *   `--config <path>` or `-c <path>`: Specifies the path to the configuration file (default: `"config.toml"`).

*   **`gosmtpd help`**: Shows help information for the application and its commands.
*   **`gosmtpd version`**: (Assuming cobra adds this by default, if not, it's a good future enhancement) Shows the application version.

## Testing Manually (Using Telnet)

You can test the server by connecting to it using a tool like `telnet`.

1.  Ensure the GoSMTPServer is running.
2.  Open a terminal or command prompt and connect:
    ```bash
    telnet 127.0.0.1 2525
    ```
    (Replace `127.0.0.1` and `2525` with your `ListenInterface` and `ListenPort` if they are different).

3.  You should see the server's welcome message (e.g., `220 gosmtp.example.com Welcome to GoSMTP`).

4.  Type `EHLO testclient.com` and press Enter. If TLS is configured on the server, you should see `250-STARTTLS` among the server's capabilities.
    ```
    EHLO testclient.com
    250-gosmtp.example.com Hello testclient.com
    250-STARTTLS
    250 PIPELINING
    ```

5.  If you type `STARTTLS`, the server will respond with `220 Ready to start TLS`.
    ```
    STARTTLS
    220 Ready to start TLS
    ```
    At this point, `telnet` cannot proceed with the TLS handshake. The subsequent communication would need to be TLS encrypted, which `telnet` does not support. You would need a different client (like `openssl s_client`) to continue.

6.  To send an email without TLS (if the server allows non-TLS connections or if TLS is disabled and `STARTTLS` was not advertised):
    ```
    MAIL FROM:<testsender@example.com>
    RCPT TO:<testrecipient@yourdomain.com>
    DATA
    Subject: My First Test Email (via Telnet)
    Date: Tue, 26 Oct 2023 10:00:00 +0000
    From: Test Sender <testsender@example.com>
    To: Test Recipient <testrecipient@yourdomain.com>

    Hello,

    This is the body of my test email sent via Telnet.
    .
    QUIT
    ```

7.  **Check for the Email**:
    Look in the directory specified by `MailDir` in your `config.toml`.

### Testing with OpenSSL s_client (for STARTTLS)

To test the STARTTLS functionality, you can use `openssl s_client`:

1.  Ensure the GoSMTPServer is running and configured with `TLSCertPath` and `TLSKeyPath`.
2.  Run the following command. If your `ListenInterface` is `0.0.0.0`, use `localhost` or the specific IP address your server is accessible on.
    ```bash
    openssl s_client -connect localhost:2525 -starttls smtp -crlf
    ```
    *(Replace `localhost:2525` with your server's address and port if different. For self-signed certificates, you might need to add `-ign_eof` to keep the s_client open after certain server responses, or use it in conjunction with `echo "QUIT" | ...`)*

3.  This command will connect, perform the STARTTLS handshake, and then show TLS session information. After the handshake information, you can type SMTP commands. The server should have responded with `220 Ready to start TLS` just before OpenSSL shows its own handshake details, and the session is now encrypted.
    You should then send `EHLO` again over the encrypted channel:
    ```
    EHLO testclient.tls.com
    ```
    The server will respond with its capabilities (which might be different now, e.g., it might advertise `AUTH` if it were implemented and required TLS). `STARTTLS` should *not* be advertised again.

4.  Proceed to send an email as you would with `telnet`, but now over the encrypted connection:
    ```
    MAIL FROM:<tls.sender@example.com>
    RCPT TO:<tls.recipient@yourdomain.com>
    DATA
    Subject: Test Email over STARTTLS
    From: TLS Sender <tls.sender@example.com>
    To: TLS Recipient <tls.recipient@yourdomain.com>

    This email was sent over an encrypted connection using STARTTLS!
    .
    QUIT
    ```

5.  **Check for the Email**:
    The email will be saved in your `MailDir`.

## Limitations

*   **Opportunistic TLS (STARTTLS)**: While STARTTLS is implemented, it relies on client initiation. Connections start in plaintext.
*   **No SMTP Authentication**: The server does not support user authentication (e.g., SMTP AUTH). It's an open relay by design if not firewalled properly.
*   **Client Certificate Authentication (mTLS)**: Not supported. The current STARTTLS implementation only authenticates the server to the client.
*   **Basic Error Handling**: Error handling for complex SMTP scenarios or edge cases is minimal.
*   **Single Goroutine Per Connection**: While it uses goroutines for concurrent connections, each connection's command processing is largely synchronous.
*   **No Mail Queue Persistence**: If the server crashes while processing, emails not yet written to disk might be lost.
*   **Not for High Volume**: Not designed for handling a large number of concurrent connections or high email throughput.
*   **Minimal Anti-Spam Measures**: Does not include any significant anti-spam or anti-virus capabilities.

## Future Enhancements (Potential)

*   [ ] Add SMTP AUTH (PLAIN, LOGIN) for user authentication (ideally advertised after STARTTLS).
*   [ ] Implement an option for an Implicit TLS listener (smtps on a dedicated port).
*   [ ] Develop a more robust mail queuing system (e.g., retry mechanisms).
*   [ ] Add hooks or integration points for spam/virus filtering tools.
*   [ ] Options for daemonization or running as a background service.
*   [ ] More detailed and configurable logging levels.
*   [ ] Rate limiting and connection controls.

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
