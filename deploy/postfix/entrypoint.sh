#!/bin/sh
set -eu
: "${LANQIN_PUBLIC_HOSTNAME:=mail.example.com}"
postconf -e "myhostname = ${LANQIN_PUBLIC_HOSTNAME}"
postconf -e "myorigin = ${LANQIN_PUBLIC_HOSTNAME}"
postconf -e "smtpd_milters = inet:opendkim:8891"
postconf -e "non_smtpd_milters = inet:opendkim:8891"
postfix check
exec postfix start-fg
