# GoSMTPServer - A Basic SMTP Server in Go

## Overview

GoSMTPServer is a simple, lightweight SMTP server written in Go, designed primarily for development, testing, or small-scale applications where emails are received and stored locally as files for later outbound delivery. It supports graceful shutdown on `SIGINT` (Ctrl+C) or `SIGTERM`.

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
*   Configurable log output path (defaults to standard output).
*   Graceful shutdown on `SIGINT` (Ctrl+C) and `SIGTERM` signals.
*   Example systemd unit file and documentation for running as a background service.
*   Configuration is managed via a TOML file (`config.toml`).
*   Provides a basic Command Line Interface (CLI).

## Prerequisites

*   Go programming language (version 1.21 or later recommended).
*   OpenSSL (optional, for generating self-signed certificates, DKIM keys, and testing with `s_client`).
*   An external script for content scanning if `IncomingFilterEnable` is true.
*   For running as a systemd service: A Linux system with systemd.

## Setup & Configuration
(This section remains largely as is - ensure all config options are listed including LogFilePath)
...
    *   `LogFilePath` (string): Path to the log file. If empty or `"-"`, logs go to standard output. Example: `"/var/log/gosmtpd/gosmtpd.log"`.
...
**Default `config.toml` example (see comments within for details):**
    ```toml
    # --- Logging Configuration ---
    LogFilePath = "" # Example: "/var/log/gosmtpd/gosmtpd.log" or "gosmtpd.log" for current directory

    ListenInterface = "0.0.0.0"
    # ... (rest of config example remains as is) ...
    ```

## Logging Configuration
(This section remains largely as is, but will be complemented by the systemd logging section)
...

## Rate Limiting (Incoming Connections)
(This section remains largely as is)
...

## Incoming Email Filtering (Content Scanning)
(This section remains largely as is)
...

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
(This section remains largely as is)
...

## Building the Server

```bash
go build -o gosmtpd .
```
This creates the `gosmtpd` executable in the current directory.

## Running the Server (Foreground)

To run the server directly in the foreground (e.g., for testing or development):
```bash
./gosmtpd start
# With a custom config file:
./gosmtpd start --config /path/to/your/config.toml
```
Logs are printed to standard output or the configured `LogFilePath`. Press `Ctrl+C` to stop the server (which will trigger a graceful shutdown).

For running as a background service, see the "Running GoSMTPServer as a Service (using systemd)" section below.

### Graceful Shutdown (Manual Foreground)
When running in the foreground, GoSMTPServer listens for `SIGINT` (Ctrl+C) to initiate a graceful shutdown. The process involves:
1.  Stopping new incoming SMTP connections.
2.  Signaling the queue processor to stop (it attempts to finish any active processing cycle).
3.  Waiting for a brief period (e.g., 30 seconds) for these components.
4.  Logging the shutdown sequence.
5.  Closing the log file if one was opened.

## CLI Commands
*   `./gosmtpd start [-c <config_path>]`: Starts the server and the queue processor.
*   `./gosmtpd validate-config [-c <config_path>]`: Validates configuration.
*   `./gosmtpd help`: Shows help.
*   `./gosmtpd version`: Shows the application version (if version information is compiled in).

## Running GoSMTPServer as a Service (using systemd)

For production-like environments on Linux systems using systemd, it's recommended to run GoSMTPServer as a systemd service. This allows it to run in the background, start automatically on boot, and be managed using `systemctl` commands.

### Prerequisites for systemd Service
*   A Linux system with systemd.
*   The `gosmtpd` binary built and placed in a suitable location (e.g., `/usr/local/bin/gosmtpd`).
*   A `config.toml` file prepared and placed in a suitable location (e.g., `/etc/gosmtpd/config.toml`).

### Logging Configuration for Service Mode
When running as a systemd service, you have a few options for logging, managed by `LogFilePath` in `config.toml` and the `StandardOutput`/`StandardError` directives in the unit file:
*   **Recommended: Logging via journald**:
    *   Set `LogFilePath = ""` or `LogFilePath = "-"` in `config.toml`. This makes GoSMTPServer log to its standard output/error.
    *   In your `gosmtpd.service` file, set `StandardOutput=journal` and `StandardError=journal`.
    *   Systemd will then capture all log output into the system journal, which can be viewed with `journalctl -u gosmtpd.service`.
*   **Application-Managed File Logging**:
    *   Set `LogFilePath` to a specific file path in `config.toml` (e.g., `/var/log/gosmtpd/gosmtpd.log`). GoSMTPServer will write its logs there.
    *   In `gosmtpd.service`, you can set `StandardOutput=null` and `StandardError=null` if you only want logs in the application's file. Alternatively, you can still use `StandardOutput=journal` to capture any messages that might still go to stdout/stderr (like initial Go runtime errors before logging is fully set up by GoSMTPServer).

### Example `gosmtpd.service` Unit File
Below is an example unit file. You'll likely need to customize paths (like `ExecStart`, `WorkingDirectory`, `ReadWritePaths`) and `User`/`Group` settings for your environment. Save this content to `/etc/systemd/system/gosmtpd.service`.

```ini
[Unit]
Description=GoSMTPServer - A basic SMTP server in Go
Documentation=https://github.com/your-repo/gosmtpd/blob/main/README.md <--- UPDATE THIS URL
After=network.target auditd.service
# If your server relies on specific mounts being available (e.g. for MailDir), add them here:
# RequiresMountsFor=/var/spool/gosmtpd

[Service]
# --- Paths and User ---
# Adjust User, Group, ExecStart, WorkingDirectory, and config path as needed.
# It's recommended to run GoSMTPServer as a dedicated non-root user.
# Example: Create a user and group 'gosmtpd':
#   sudo groupadd --system gosmtpd
#   sudo useradd --system -g gosmtpd -d /opt/gosmtpd -s /usr/sbin/nologin -c "GoSMTPServer Service User" gosmtpd

User=gosmtpd
Group=gosmtpd

# Path to the GoSMTPServer executable and its configuration file.
# Ensure the binary is executable (chmod +x gosmtpd).
ExecStart=/usr/local/bin/gosmtpd start --config /etc/gosmtpd/config.toml

# Working directory for the service.
# If using relative paths in config.toml (e.g., for MailDir), they will be relative to this.
# Absolute paths in config.toml are generally safer for services.
WorkingDirectory=/opt/gosmtpd

# --- Logging ---
# If LogFilePath in config.toml is empty or "-", GoSMTPServer logs to its stdout/stderr.
# Systemd will capture this into the journal with these settings:
StandardOutput=journal
StandardError=journal

# --- Restart Behavior ---
Restart=on-failure
RestartSec=5s # Time to wait before restarting the service

# --- Process Management ---
Type=simple

# If GoSMTPServer needs to bind to privileged ports (e.g., 25) as a non-root user:
# AmbientCapabilities=CAP_NET_BIND_SERVICE (Requires systemd >= 229)
# Alternatively, use setcap: sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/gosmtpd

# --- Security Hardening (Optional but Recommended) ---
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
# CRITICAL: Adjust ReadWritePaths to include your MailDir, QuarantineDir, and LogFilePath's directory (if app file logging).
# Example: ReadWritePaths=/opt/gosmtpd /etc/gosmtpd /var/spool/gosmtpd_maildir /var/log/gosmtpd
# ProtectKernelTunables=true
# ProtectKernelModules=true
# ProtectControlGroups=true
# RestrictAddressFamilies=AF_INET AF_INET6

[Install]
WantedBy=multi-user.target
```

### Setup and Management Instructions

1.  **Create User and Group (Recommended)**:
    ```bash
    sudo groupadd --system gosmtpd
    sudo useradd --system --gid gosmtpd --home-dir /opt/gosmtpd --shell /usr/sbin/nologin --create-home gosmtpd
    ```
    *(Adjust `--home-dir` if your `WorkingDirectory` is different, e.g. if you don't want a home dir created, use `--no-create-home` and a non-existent dir like `/var/empty/gosmtpd`)*

2.  **Prepare Directories and Files**:
    *   `sudo mkdir -p /opt/gosmtpd` (Set as `WorkingDirectory` in the unit file)
    *   `sudo mkdir -p /etc/gosmtpd` (For `config.toml`)
    *   **Mail & Log Directories (adjust paths as per your `config.toml`)**:
        *   `sudo mkdir -p /var/spool/gosmtpd/maildata` (Example `MailDir`)
        *   `sudo mkdir -p /var/spool/gosmtpd/quarantine` (Example `IncomingFilterQuarantineDir`)
        *   `sudo mkdir -p /var/log/gosmtpd` (Example directory for `LogFilePath`)
    *   **Set Ownership**:
        *   `sudo chown -R gosmtpd:gosmtpd /opt/gosmtpd`
        *   `sudo chown -R gosmtpd:gosmtpd /etc/gosmtpd` (Ensure config is readable by user)
        *   `sudo chown -R gosmtpd:gosmtpd /var/spool/gosmtpd`
        *   `sudo chown -R gosmtpd:gosmtpd /var/log/gosmtpd`
    *   **Copy Files**:
        *   Copy your compiled `gosmtpd` binary to `/usr/local/bin/gosmtpd` (or the path in `ExecStart`) and make it executable: `sudo chmod +x /usr/local/bin/gosmtpd`.
        *   Copy your `config.toml` to `/etc/gosmtpd/config.toml` (or the path in `ExecStart`). Ensure it's readable by the `gosmtpd` user (e.g., `sudo chmod 640 /etc/gosmtpd/config.toml`).

3.  **Install the systemd Unit File**:
    *   Create the file: `sudo nano /etc/systemd/system/gosmtpd.service`
    *   Paste the (customized) unit file content from the example above. Save and exit.
    *   Set permissions: `sudo chmod 644 /etc/systemd/system/gosmtpd.service`

4.  **Reload systemd, Enable, and Start**:
    *   `sudo systemctl daemon-reload`
    *   `sudo systemctl enable gosmtpd.service` (To start on boot)
    *   `sudo systemctl start gosmtpd.service`

5.  **Check Status**:
    *   `sudo systemctl status gosmtpd.service`
    *   Look for "active (running)". Check recent log lines shown.

### Viewing Logs with systemd
*   If using `StandardOutput=journal` (recommended if `LogFilePath` is empty/stdout):
    *   View live logs: `sudo journalctl -u gosmtpd.service -f`
    *   View all logs for the service: `sudo journalctl -u gosmtpd.service`
*   If `LogFilePath` is set in `config.toml` to a file:
    *   `tail -f /path/to/your/gosmtpd.log` (or the path you configured).

### Graceful Shutdown with systemd
*   `sudo systemctl stop gosmtpd.service` sends a `SIGTERM` signal to the `gosmtpd` process.
*   GoSMTPServer will then initiate its graceful shutdown procedure as described in the "Running the Server (Foreground)" -> "Graceful Shutdown" section (stopping the listener, allowing the queue processor to finish its current cycle, within a timeout).

## Testing Manually
(This section remains largely as is)
...
## Testing Outbound Security Features
(This section remains largely as is)
...
## Security Notes
(This section remains largely as is, but the systemd section adds security context)
...
## Limitations
(This section remains largely as is)
...
## Future Enhancements (Potential)
(This section remains largely as is)
...

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
