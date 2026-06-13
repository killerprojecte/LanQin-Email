# LanQin Email Docker 部署说明

## 启动

```bash
cd deploy
cp .env.example .env
# 修改 LANQIN_PUBLIC_HOSTNAME / LANQIN_ADMIN_EMAIL / LANQIN_ADMIN_PASSWORD
docker compose up -d --build
```

## DNS

进入 Web 管理后台后，在“DNS 记录”面板查看每个域名需要配置的：

- MX
- SPF TXT
- DKIM TXT
- DMARC TXT

配置完成后点击“检测”。

## 邮件服务边界

- Postfix 读取 `/data/lanqin.db` 中的 `domains`、`mailboxes`、`aliases`。
- Dovecot 读取同一个 SQLite 数据库进行邮箱认证，并使用 `/var/mail/vhosts` 作为 Maildir 根目录。
- OpenDKIM 启动时从 SQLite 导出域名 DKIM 私钥到容器内 `/etc/opendkim/keys`。
- Go API 是 Webmail 和管理后台唯一入口；浏览器不直接连接 SMTP/IMAP。
- Go API 会读取 `LANQIN_MAILDIR_ROOT=/var/mail/vhosts`，周期扫描 Maildir，把 Postfix/Dovecot 入站邮件同步成 Webmail 索引。

## 生产注意

- 替换 Dovecot 示例自签证书，建议在 Nginx 或边缘负载均衡终止 HTTPS。
- 云厂商通常默认封禁 25 端口，需要单独申请解封。
- SQLite 适合 V1 单机部署；多节点部署前迁移到 PostgreSQL，并把 Postfix/Dovecot maps 改为 PostgreSQL。 
