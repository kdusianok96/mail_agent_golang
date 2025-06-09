#!/bin/bash
#
# setup_gosmtpd.sh - Скрипт для первоначальной настройки окружения GoSMTPServer
#
# Этот скрипт создает необходимого пользователя, группу и структуру каталогов
# с соответствующими правами доступа для работы GoSMTPServer.
#
# Запускать от имени root или с использованием sudo.
#

# Выход при ошибке, при обращении к неустановленной переменной, ошибка в пайпе
set -e
set -u
set -o pipefail

# --- Настраиваемые переменные ---
# Пользователи могут переопределить эти переменные перед запуском скрипта, например:
# GOSMTPD_USER_OVERRIDE=myuser GOSMTPD_GROUP_OVERRIDE=mygroup ./setup_gosmtpd.sh
GOSMTPD_USER="${GOSMTPD_USER_OVERRIDE:-gosmtpd}"
GOSMTPD_GROUP="${GOSMTPD_GROUP_OVERRIDE:-gosmtpd}"

# Корень установки приложения, где могут лежать бинарники, документация и т.д.
APP_INSTALL_ROOT_DIR="${APP_INSTALL_ROOT_DIR_OVERRIDE:-/opt/gosmtpd}"
# Каталог для бинарного файла GoSMTPServer
APP_BIN_DIR="${APP_BIN_DIR_OVERRIDE:-${APP_INSTALL_ROOT_DIR}/bin}"

# Каталог для файла конфигурации (config.toml)
CONFIG_DIR="${CONFIG_DIR_OVERRIDE:-/etc/gosmtpd}"

# Каталог для лог-файлов (если LogFilePath в config.toml указывает сюда)
LOG_DIR="${LOG_DIR_OVERRIDE:-/var/log/gosmtpd}"

# Базовый каталог для изменяемых данных приложения (почта, карантин)
SPOOL_BASE_DIR="${SPOOL_BASE_DIR_OVERRIDE:-/var/spool/gosmtpd}"
# Имена подкаталогов внутри SPOOL_BASE_DIR. Обычно не переопределяются, т.к. завязаны на config.toml.
MAIL_DIR_NAME="maildata"
QUARANTINE_DIR_NAME="quarantine"

# Каталог для PID файла, если приложение его создает (более стандартный путь для systemd Type=simple - /run/USERNAME/)
# Для Type=simple в systemd, PIDFile не указывается в unit-файле, systemd сам отслеживает процесс.
# Но если приложение само пишет PID, этот каталог должен быть доступен.
PID_DIR_FOR_APP="${PID_DIR_FOR_APP_OVERRIDE:-/run/${GOSMTPD_USER}}"


# Вычисляемые полные пути
MAIL_DIR="${SPOOL_BASE_DIR}/${MAIL_DIR_NAME}"
QUARANTINE_DIR="${SPOOL_BASE_DIR}/${QUARANTINE_DIR_NAME}"

# --- Функции ---
info() {
    echo "[INFO] $1"
}

warn() {
    echo "[WARN] $1" >&2
}

error_exit() {
    echo "[ERROR] $1" >&2
    exit 1
}

# --- Проверки ---
if [ "$(id -u)" -ne 0 ]; then
    error_exit "Этот скрипт должен быть запущен от имени root или с использованием sudo."
fi

# --- Основная логика ---

info "Начало настройки окружения для GoSMTPServer..."
info "Используемый пользователь: $GOSMTPD_USER, группа: $GOSMTPD_GROUP"

# 1. Создание группы
if ! getent group "$GOSMTPD_GROUP" >/dev/null; then
    info "Создание системной группы '$GOSMTPD_GROUP'..."
    groupadd --system "$GOSMTPD_GROUP" || error_exit "Не удалось создать группу '$GOSMTPD_GROUP'."
    info "Группа '$GOSMTPD_GROUP' успешно создана."
else
    info "Группа '$GOSMTPD_GROUP' уже существует."
fi

# 2. Создание пользователя
# Используем --no-create-home, так как APP_INSTALL_ROOT_DIR может быть не стандартным /home
# и мы создаем его сами с нужными правами.
if ! id "$GOSMTPD_USER" >/dev/null 2>&1; then
    info "Создание системного пользователя '$GOSMTPD_USER'..."
    useradd --system --gid "$GOSMTPD_GROUP" --home-dir "$APP_INSTALL_ROOT_DIR" \
            --no-create-home --shell /usr/sbin/nologin \
            -c "GoSMTPServer Service User" "$GOSMTPD_USER" || error_exit "Не удалось создать пользователя '$GOSMTPD_USER'."
    info "Пользователь '$GOSMTPD_USER' успешно создан."
else
    info "Пользователь '$GOSMTPD_USER' уже существует."
fi

# 3. Создание каталогов и установка прав
info "Создание и настройка каталогов..."

# Корневой каталог приложения и каталог для бинарного файла
info "Настройка каталога приложения: '$APP_INSTALL_ROOT_DIR' и бинарного каталога: '$APP_BIN_DIR'..."
mkdir -p "$APP_BIN_DIR" || error_exit "Не удалось создать каталог '$APP_BIN_DIR'."
# Владелец корневого каталога приложения - root, если бинарник просто копируется туда.
# Если пользователь gosmtpd должен иметь возможность писать в APP_INSTALL_ROOT_DIR (не рекомендуется для бинарников), тогда chown.
# Для безопасности, сам APP_INSTALL_ROOT_DIR может принадлежать root, а APP_BIN_DIR тоже.
# Но если предполагается, что пользователь gosmtpd будет управлять файлами в APP_INSTALL_ROOT_DIR (кроме bin):
chown -R "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$APP_INSTALL_ROOT_DIR"
chmod 750 "$APP_INSTALL_ROOT_DIR" # Владелец rwx, группа rx
chmod 750 "$APP_BIN_DIR"          # Владелец rwx, группа rx (для запуска бинарника)
info "Каталог приложения '$APP_BIN_DIR' настроен."

# Каталог конфигурации
info "Настройка каталога конфигурации: '$CONFIG_DIR'..."
mkdir -p "$CONFIG_DIR" || error_exit "Не удалось создать каталог '$CONFIG_DIR'."
chown "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$CONFIG_DIR"
chmod 750 "$CONFIG_DIR" # Владелец rwx, группа rx (чтобы процесс мог читать конфиг)
info "Каталог конфигурации '$CONFIG_DIR' настроен."

# Каталог логов
info "Настройка каталога логов: '$LOG_DIR'..."
mkdir -p "$LOG_DIR" || error_exit "Не удалось создать каталог '$LOG_DIR'."
chown "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$LOG_DIR"
chmod 750 "$LOG_DIR" # Владелец rwx, группа rx (процесс пишет логи)
info "Каталог логов '$LOG_DIR' настроен."

# Базовый каталог для спула
info "Настройка базового каталога спула: '$SPOOL_BASE_DIR'..."
mkdir -p "$SPOOL_BASE_DIR" || error_exit "Не удалось создать каталог '$SPOOL_BASE_DIR'."
chown "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$SPOOL_BASE_DIR"
chmod 750 "$SPOOL_BASE_DIR" # Владелец rwx, группа rx
info "Базовый каталог спула '$SPOOL_BASE_DIR' настроен."

# Каталог почтовой очереди (MailDir)
info "Настройка каталога почтовой очереди: '$MAIL_DIR'..."
mkdir -p "$MAIL_DIR" || error_exit "Не удалось создать каталог '$MAIL_DIR'."
# Также создаем подкаталоги, используемые приложением
mkdir -p "${MAIL_DIR}/failed" || error_exit "Не удалось создать подкаталог 'failed' в '$MAIL_DIR'."
mkdir -p "${MAIL_DIR}/corrupt" || error_exit "Не удалось создать подкаталог 'corrupt' в '$MAIL_DIR'."
chown -R "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$MAIL_DIR"
chmod -R 770 "$MAIL_DIR" # Процесс должен иметь полные права на чтение/запись/удаление файлов и создание подкаталогов
info "Каталог почтовой очереди '$MAIL_DIR' настроен."

# Каталог карантина
info "Настройка каталога карантина: '$QUARANTINE_DIR'..."
mkdir -p "$QUARANTINE_DIR" || error_exit "Не удалось создать каталог '$QUARANTINE_DIR'."
chown "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$QUARANTINE_DIR"
chmod 770 "$QUARANTINE_DIR" # Аналогично очереди
info "Каталог карантина '$QUARANTINE_DIR' настроен."

# Каталог для PID файла (если приложение само его создает)
info "Настройка каталога для PID файла: '$PID_DIR_FOR_APP'..."
mkdir -p "$PID_DIR_FOR_APP" || error_exit "Не удалось создать каталог '$PID_DIR_FOR_APP'."
chown "$GOSMTPD_USER":"$GOSMTPD_GROUP" "$PID_DIR_FOR_APP"
chmod 750 "$PID_DIR_FOR_APP" # Владелец rwx, группа rx, остальные ---
info "Каталог PID '$PID_DIR_FOR_APP' настроен."


info "---------------------------------------------------------------------"
info "Настройка окружения GoSMTPServer успешно завершена!"
info ""
info "Следующие шаги:"
info "  1. Поместите скомпилированный бинарный файл 'gosmtpd' в '$APP_BIN_DIR'."
info "     Пример: sudo cp ./gosmtpd $APP_BIN_DIR/"
info "     Пример: sudo chmod +x ${APP_BIN_DIR}/gosmtpd"
info "  2. Создайте или скопируйте файл конфигурации 'config.toml' в '$CONFIG_DIR'."
info "     Пример: sudo cp ./config.toml.example ${CONFIG_DIR}/config.toml"
info "     Обязательно настройте 'config.toml', особенно пути:"
info "       - MailDir = \"$MAIL_DIR\""
info "       - IncomingFilterQuarantineDir = \"$QUARANTINE_DIR\""
info "       - LogFilePath = \"${LOG_DIR}/gosmtpd.log\" (если используется файловое логирование)"
info "       - DKIMPrivateKeyPath (если DKIM включен)"
info "       - TLSCertPath, TLSKeyPath (если TLS включен)"
info "     Убедитесь, что пользователь '$GOSMTPD_USER' имеет права на чтение config.toml:"
info "       sudo chown ${GOSMTPD_USER}:${GOSMTPD_GROUP} ${CONFIG_DIR}/config.toml"
info "       sudo chmod 640 ${CONFIG_DIR}/config.toml"
info "  3. Если используете systemd, скопируйте и настройте 'gosmtpd.service' в /etc/systemd/system/."
info "     Обновите пути и пользователя/группу в unit-файле в соответствии с этой настройкой."
info "     Не забудьте `sudo systemctl daemon-reload` после создания/изменения unit-файла."
info "---------------------------------------------------------------------"

exit 0
