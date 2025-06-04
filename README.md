# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files.

**⚠️ Important Note:** This is a basic implementation and is **NOT SUITABLE FOR PRODUCTION USE**. It lacks critical security features like robust TLS certificate validation beyond what the `crypto/tls` package provides by default, comprehensive spam filtering, and advanced performance optimizations found in production-grade mail servers.

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
*   SMTP Authentication (`AUTH PLAIN`, `AUTH LOGIN`) over TLS to control mail sending.

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
    *   `MailDir` (string): Directory to store received emails (e.g., `"maildata"`). Created if it doesn't exist.
    *   `TLSCertPath` (string): Path to TLS certificate file. Enables STARTTLS if both this and `TLSKeyPath` are set.
    *   `TLSKeyPath` (string): Path to TLS private key file. Enables STARTTLS if both this and `TLSCertPath` are set.
    *   `RequireAuth` (boolean): If `true`, requires clients to authenticate via SMTP AUTH (after STARTTLS) before `MAIL FROM` is accepted. This applies only if users are defined in the `[Users]` section. Default is `false`.
    *   `[Users]` (table): Defines usernames and their bcrypt hashed passwords for SMTP AUTH.

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
    ```

## Setting up TLS (STARTTLS)

The server supports STARTTLS for upgrading a plain text connection to encrypted TLS.
1.  Enable by setting `TLSCertPath` and `TLSKeyPath` in `config.toml`.
2.  If paths are not set, STARTTLS is disabled.
3.  The server advertises `STARTTLS` in `EHLO` if configured.

### Generating Self-Signed Certificates (for testing only)

**For testing only.** Browsers/clients will warn about self-signed certs. Use a CA like Let's Encrypt for production.
```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"
```
*   `server.key` -> `TLSKeyPath`
*   `server.crt` -> `TLSCertPath`

## Setting up SMTP Authentication

Enable SMTP AUTH by defining users in the `[Users]` section of `config.toml`.
*   Authentication is only advertised and processed over a TLS-secured connection (after STARTTLS).
*   If `RequireAuth = true` in `config.toml`, clients must authenticate before they can send mail.

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
./gosmtpd start --config /path/to/your/config.toml
```
Logs are printed to standard output.

## CLI Commands

*   `./gosmtpd start [-c <config_path>]`: Starts the server.
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

### OpenSSL s_client (Full STARTTLS and AUTH Testing)

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
    Check `MailDir` for the saved email.

## Security Notes
*   **TLS is Essential for AUTH**: SMTP AUTH (PLAIN/LOGIN) sends credentials in a way that is vulnerable if not protected by TLS. This server only advertises AUTH after STARTTLS.
*   **Bcrypt Hashes**: Passwords in `config.toml` must be bcrypt hashes. Plaintext is not supported.
*   **Self-Signed Certificates**: Suitable for testing only. They offer encryption but no trust. Use CA-issued certificates for any real-world scenario.
*   **User Storage**: Storing user credentials in the configuration file is basic. For more users or higher security needs, external user databases (LDAP, SQL DB) are recommended (outside current scope).
*   **Open Relay**: If `RequireAuth = false` or no users are configured, this server can act as an open relay. Configure carefully and use firewalls appropriately.

## Limitations

*   **Opportunistic TLS (STARTTLS)**: Relies on client initiation.
*   **SMTP Authentication Scope**: `AUTH PLAIN` and `AUTH LOGIN` are supported over TLS. Other mechanisms or unencrypted AUTH are not.
*   **Client Certificate Authentication (mTLS)**: Not supported.
*   **Basic Error Handling**: For complex SMTP scenarios.
*   **Single Goroutine Per Connection**: Simple concurrency model.
*   **No Mail Queue Persistence**: Emails are written directly; no retry for temporary failures.

## Future Enhancements (Potential)

*   [ ] Implement an option for an Implicit TLS listener (SMTPS on a dedicated port).
*   [ ] More robust mail queuing system (e.g., retry mechanisms).
*   [ ] Add hooks or integration points for spam/virus filtering tools.
*   [ ] Options for daemonization/background running.
*   [ ] More detailed and configurable logging levels.
*   [ ] Rate limiting and connection controls.

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
