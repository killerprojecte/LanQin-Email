#!/bin/sh
set -eu
addgroup --system --gid 5000 vmail 2>/dev/null || true
adduser --system --uid 5000 --gid 5000 --home /var/mail/vhosts --no-create-home vmail 2>/dev/null || true
mkdir -p /var/mail/vhosts
chown -R 5000:5000 /var/mail/vhosts
exec dovecot -F
