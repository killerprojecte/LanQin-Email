#!/bin/sh
set -eu

: "${LANQIN_PUBLIC_HOSTNAME:=mail.example.com}"
: "${LANQIN_DATA_DIR:=/data}"
: "${LANQIN_DB_PATH:=/data/lanqin.db}"
: "${LANQIN_ADDR:=127.0.0.1:8080}"
: "${LANQIN_SMTP_HOST:=127.0.0.1}"
: "${LANQIN_SMTP_PORT:=25}"
: "${LANQIN_MAILDIR_ROOT:=/var/mail/vhosts}"

export LANQIN_DATA_DIR LANQIN_DB_PATH LANQIN_ADDR LANQIN_SMTP_HOST LANQIN_SMTP_PORT LANQIN_MAILDIR_ROOT

addgroup --system --gid 5000 vmail 2>/dev/null || true
adduser --system --uid 5000 --gid 5000 --home /var/mail/vhosts --no-create-home vmail 2>/dev/null || true
mkdir -p /data /var/mail/vhosts /etc/opendkim/keys /var/spool/postfix /var/run/dovecot
chown -R 5000:5000 /var/mail/vhosts

postconf -e "myhostname = ${LANQIN_PUBLIC_HOSTNAME}"
postconf -e "myorigin = ${LANQIN_PUBLIC_HOSTNAME}"
postconf -e "virtual_transport = lmtp:inet:127.0.0.1:24"
postconf -e "smtpd_sasl_path = inet:127.0.0.1:12345"
postconf -e "smtpd_milters = inet:127.0.0.1:8891"
postconf -e "non_smtpd_milters = inet:127.0.0.1:8891"

# OpenDKIM keys are generated after API seed/migrations create the SQLite DB.
/usr/local/bin/lanqin-api >/tmp/lanqin-api-bootstrap.log 2>&1 &
bootstrap_pid=$!
for i in $(seq 1 60); do
  if [ -f "$LANQIN_DB_PATH" ]; then
    users_count="$(sqlite3 "$LANQIN_DB_PATH" "SELECT COALESCE(COUNT(1),0) FROM users;" 2>/dev/null || echo 0)"
    domains_count="$(sqlite3 "$LANQIN_DB_PATH" "SELECT COALESCE(COUNT(1),0) FROM domains;" 2>/dev/null || echo 0)"
    if [ "${users_count:-0}" -gt 0 ] && [ "${domains_count:-0}" -gt 0 ]; then
      break
    fi
  fi
  sleep 1
done
kill "$bootstrap_pid" 2>/dev/null || true
wait "$bootstrap_pid" 2>/dev/null || true

: > /etc/opendkim/KeyTable
: > /etc/opendkim/SigningTable
if [ -f "$LANQIN_DB_PATH" ]; then
  sqlite3 -separator '|' "$LANQIN_DB_PATH" "SELECT name, dkim_selector, dkim_private_key FROM domains WHERE status='active';" | while IFS='|' read -r domain selector private_key; do
    [ -n "$domain" ] || continue
    dir="/etc/opendkim/keys/$domain"
    mkdir -p "$dir"
    keyfile="$dir/$selector.private"
    printf '%s' "$private_key" | base64 -d > "$keyfile"
    chmod 600 "$keyfile"
    echo "$selector._domainkey.$domain $domain:$selector:$keyfile" >> /etc/opendkim/KeyTable
    echo "*@${domain} ${selector}._domainkey.${domain}" >> /etc/opendkim/SigningTable
  done
fi
chown -R opendkim:opendkim /etc/opendkim

postfix check
exec /usr/bin/supervisord -c /etc/supervisor/conf.d/lanqin.conf
