FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends postfix postfix-sqlite ca-certificates rsyslog && rm -rf /var/lib/apt/lists/*
COPY main.cf /etc/postfix/main.cf
COPY master.cf /etc/postfix/master.cf
COPY sqlite-*.cf /etc/postfix/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 25 587
CMD ["/entrypoint.sh"]
