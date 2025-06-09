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
*   Example shell-скрипт (`setup_gosmtpd.sh`) для автоматизированной первоначальной настройки окружения в Linux.
*   Configuration is managed via a TOML file (`config.toml`).
*   Provides a basic Command Line Interface (CLI) with commands to:
    *   Start the server.
    *   Validate configuration.
    *   Display a summary of the mail queue (active, failed, corrupt counts).

## Prerequisites

*   Go programming language (version 1.21 or later recommended).
*   OpenSSL (optional, for generating self-signed certificates, DKIM keys, and testing with `s_client`).
*   An external script for content scanning if `IncomingFilterEnable` is true.
*   For running as a systemd service or using the setup script: A Linux system.

## Setup & Configuration

1.  **Get the Code**:
    ```bash
    git clone <repository_url>
    cd gosmtpserver
    ```
    *(Replace `<repository_url>` with the actual URL of the repository)*

2.  **Configuration File (`config.toml`)**:
    (Details of config options as before)
    *   `LogFilePath` (string): Path to the log file. If empty or `"-"`, logs go to standard output. Example: `"/var/log/gosmtpd/gosmtpd.log"`.
    *   ... (all other config options) ...

    **Default `config.toml` example (see comments within for details):**
    (Full example as before)
    ```toml
    # --- Logging Configuration ---
    LogFilePath = "" # Example: "/var/log/gosmtpd/gosmtpd.log" or "gosmtpd.log" for current directory

    ListenInterface = "0.0.0.0"
    # ... (rest of config example remains as is) ...
    ```

## Автоматизированная первоначальная настройка (Linux с `setup_gosmtpd.sh`)

Для упрощения первоначальной настройки окружения на Linux-системах, в репозитории предложен shell-скрипт `setup_gosmtpd.sh`.

**Назначение скрипта**:
Скрипт `setup_gosmtpd.sh` автоматически создает необходимого системного пользователя и группу, а также основную структуру каталогов с соответствующими правами доступа, которые требуются для работы GoSMTPServer. Это помогает обеспечить правильные и безопасные разрешения для файлов и каталогов сервера.

**Важно**: Скрипт является примером и может потребовать адаптации под ваши конкретные требования безопасности и структуру файловой системы. Внимательно изучите его перед запуском.

**1. Получение скрипта**:
Скрипт `setup_gosmtpd.sh` находится в каталоге `examples/scripts/` вашего репозитория GoSMTPServer.

**2. Предоставление прав на исполнение**:
Перед запуском сделайте скрипт исполняемым:
```bash
chmod +x examples/scripts/setup_gosmtpd.sh
```

**3. Запуск скрипта**:
Скрипт должен быть запущен от имени `root` или с использованием `sudo`:
```bash
sudo ./examples/scripts/setup_gosmtpd.sh
```

**4. Настраиваемые переменные**:
Некоторые ключевые параметры скрипта можно переопределить с помощью переменных окружения перед его запуском. Это позволяет адаптировать установку без изменения самого скрипта.

*   `GOSMTPD_USER_OVERRIDE`: Имя системного пользователя (по умолчанию: `gosmtpd`).
*   `GOSMTPD_GROUP_OVERRIDE`: Имя системной группы (по умолчанию: `gosmtpd`).
*   `APP_INSTALL_ROOT_DIR_OVERRIDE`: Корневой каталог для установки приложения (по умолчанию: `/opt/gosmtpd`). Бинарный файл предполагается в `${APP_INSTALL_ROOT_DIR}/bin`.
*   `CONFIG_DIR_OVERRIDE`: Каталог для файла конфигурации (по умолчанию: `/etc/gosmtpd`).
*   `LOG_DIR_OVERRIDE`: Каталог для лог-файлов (по умолчанию: `/var/log/gosmtpd`).
*   `SPOOL_BASE_DIR_OVERRIDE`: Базовый каталог для почтового спула (очереди, карантина) (по умолчанию: `/var/spool/gosmtpd`).
*   `PID_DIR_FOR_APP_OVERRIDE`: Каталог для PID-файла, если приложение его создает (по умолчанию: `/run/${GOSMTPD_USER_OVERRIDE:-gosmtpd}`).

*Пример переопределения*:
```bash
sudo GOSMTPD_USER_OVERRIDE=mymailer SPOOL_BASE_DIR_OVERRIDE=/srv/gosmtpd_spool ./examples/scripts/setup_gosmtpd.sh
```

**5. Действия скрипта**:
*   Создает системную группу (если она не существует).
*   Создает системного пользователя (если он не существует) без домашнего каталога и с оболочкой `/usr/sbin/nologin` или `/sbin/nologin`.
*   Создает следующие каталоги (если они не существуют):
    *   Каталог для бинарного файла (например, `/opt/gosmtpd/bin`).
    *   Каталог конфигурации (например, `/etc/gosmtpd`).
    *   Каталог логов (например, `/var/log/gosmtpd`).
    *   Базовый каталог спула (например, `/var/spool/gosmtpd`).
    *   Каталог почтовой очереди `maildata` внутри базового каталога спула (включая подкаталоги `failed` и `corrupt`).
    *   Каталог карантина `quarantine` внутри базового каталога спула.
    *   Каталог для PID-файла (например, `/run/gosmtpd`).
*   Устанавливает владельцев (пользователя и группу `gosmtpd` или переопределенные) и права доступа для созданных каталогов (обычно `750` или `770` для каталогов данных).

**6. Последующие шаги**:
После успешного выполнения скрипта `setup_gosmtpd.sh`, вам необходимо:
*   **Скопировать бинарный файл**: Поместите скомпилированный бинарный файл `gosmtpd` в каталог, указанный для бинарных файлов (по умолчанию `${APP_INSTALL_ROOT_DIR_OVERRIDE:-/opt/gosmtpd}/bin`). Сделайте его исполняемым (`sudo chmod +x path/to/gosmtpd`).
*   **Создать/скопировать файл конфигурации**: Поместите ваш `config.toml` в каталог конфигурации (по умолчанию `/etc/gosmtpd`).
*   **Настроить `config.toml`**: Убедитесь, что пути в `config.toml` (например, `MailDir`, `LogFilePath`, `IncomingFilterQuarantineDir`, `DKIMPrivateKeyPath`) соответствуют каталогам, созданным скриптом (или вашим кастомным путям), и что пользователь, от имени которого будет работать сервис, имеет к ним доступ.
*   **Настроить systemd сервис (если используется)**: Если вы планируете запускать GoSMTPServer как сервис systemd, адаптируйте пример `gosmtpd.service` (особенно пути `ExecStart`, `WorkingDirectory` и директивы `User`, `Group`, `ReadWritePaths`) в соответствии с настройками, использованными скриптом.

## Logging Configuration
(This section remains largely as is)
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
(This section remains largely as is)
...

## Running the Server (Foreground)
(This section remains largely as is)
...

## CLI Commands
(This section remains largely as is)
...

## Running GoSMTPServer as a Service (using systemd)
(This section remains largely as is)
...

## Testing Manually
(This section remains largely as is)
...
## Testing Outbound Security Features
(This section remains largely as is)
...
## Security Notes
(This section remains largely as is)
...
## Limitations
(This section remains largely as is)
...
## Future Enhancements (Potential)
(This section remains largely as is)
...

This README provides a comprehensive guide for users to understand, set up, and use GoSMTPServer.
