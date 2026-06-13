FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends dovecot-core dovecot-imapd dovecot-lmtpd dovecot-sqlite ssl-cert ca-certificates && rm -rf /var/lib/apt/lists/*
COPY dovecot.conf /etc/dovecot/dovecot.conf
COPY dovecot-sql.conf.ext /etc/dovecot/dovecot-sql.conf.ext
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 993 24 12345
CMD ["/entrypoint.sh"]

