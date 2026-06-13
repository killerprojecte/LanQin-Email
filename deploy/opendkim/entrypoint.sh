#!/bin/sh
set -eu
mkdir -p /etc/opendkim/keys
: > /etc/opendkim/KeyTable
: > /etc/opendkim/SigningTable
if [ -f /data/lanqin.db ]; then
  sqlite3 -separator '|' /data/lanqin.db "SELECT name, dkim_selector, dkim_private_key FROM domains WHERE status='active';" | while IFS='|' read -r domain selector private_key; do
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
exec opendkim -f -x /etc/opendkim.conf
