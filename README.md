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

    **Default `config.toml` example:**
    ```toml
    ListenInterface = "0.0.0.0"
    ListenPort = 2525
    ServerHostname = "gosmtp.example.com"
    MailDir = "maildata"
    ```

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

4.  Type the following SMTP commands, pressing Enter after each line:
    ```
    EHLO testclient.com
    MAIL FROM:<testsender@example.com>
    RCPT TO:<testrecipient@yourdomain.com>
    DATA
    Subject: My First Test Email
    Date: Tue, 26 Oct 2023 10:00:00 +0000
    From: Test Sender <testsender@example.com>
    To: Test Recipient <testrecipient@yourdomain.com>

    Hello,

    This is the body of my test email.
    Hope you receive it!
    .
    QUIT
    ```
    *   The server should respond with `250 OK` (or similar success codes) to most commands.
    *   After `DATA`, the server responds with `354 Start mail input...`.
    *   The email message (headers and body) is typed after `DATA`.
    *   The email message is terminated by a line containing only a single dot (`.`).
    *   The server should respond with `250 OK: message accepted for delivery (queued as <filename.eml>)`.

5.  **Check for the Email**:
    Look in the directory specified by `MailDir` in your `config.toml` (e.g., `maildata/`). You should find a new `.eml` file containing the email you just sent. The filename will be based on a timestamp and a random string.

## Limitations

*   **No TLS/SSL**: Communication is unencrypted. **Do not use for sensitive information.**
*   **No SMTP Authentication**: The server does not support user authentication. It's an open relay by design (though typically firewalled to local or development networks).
*   **Basic Error Handling**: Error handling for complex SMTP scenarios or edge cases is minimal.
*   **Single Goroutine Per Connection**: While it uses goroutines for concurrent connections, each connection's command processing is largely synchronous.
*   **No Mail Queue Persistence**: If the server crashes while processing, emails not yet written to disk might be lost.
*   **Not for High Volume**: Not designed for handling a large number of concurrent connections or high email throughput.
*   **Minimal Anti-Spam Measures**: Does not include any significant anti-spam or anti-virus capabilities.

## Future Enhancements (Potential)

*   [ ] Implement TLS (STARTTLS) support for encrypted communication.
*   [ ] Add SMTP AUTH (PLAIN, LOGIN) for user authentication.
*   [ ] Develop a more robust mail queuing system (e.g., retry mechanisms).
*   [ ] Add hooks or integration points for spam/virus filtering tools.
*   [ ] Options for daemonization or running as a background service.
*   [ ] More detailed and configurable logging levels.
*   [ ] Rate limiting and connection controls.

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
