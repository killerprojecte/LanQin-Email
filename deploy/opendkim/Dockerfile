FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends opendkim opendkim-tools sqlite3 ca-certificates && rm -rf /var/lib/apt/lists/*
COPY opendkim.conf /etc/opendkim.conf
COPY TrustedHosts /etc/opendkim/TrustedHosts
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 8891
CMD ["/entrypoint.sh"]
