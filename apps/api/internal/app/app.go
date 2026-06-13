package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type App struct {
	cfg          Config
	db           *sql.DB
	log          *slog.Logger
	now          func() time.Time
	policy       *HTMLPolicy
	workerCancel context.CancelFunc
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "attachments"), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	a := &App{cfg: cfg, db: db, log: logger, now: time.Now, policy: NewHTMLPolicy()}
	if err := a.configureSQLite(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := a.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := a.seed(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if strings.TrimSpace(cfg.MaildirRoot) != "" {
		workerCtx, cancel := context.WithCancel(context.Background())
		a.workerCancel = cancel
		go a.maildirWorker(workerCtx)
	}
	return a, nil
}

func (a *App) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	if a.workerCancel != nil {
		a.workerCancel()
	}
	return a.db.Close()
}

func (a *App) configureSQLite(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	}
	for _, q := range pragmas {
		if _, err := a.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('admin','user')),
			password_hash TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS domains (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			dkim_selector TEXT NOT NULL,
			dkim_public_key TEXT NOT NULL,
			dkim_private_key TEXT NOT NULL,
			dns_status TEXT NOT NULL DEFAULT 'unchecked',
			dns_checked_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS mailboxes (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			local_part TEXT NOT NULL,
			address TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			quota_mb INTEGER NOT NULL DEFAULT 1024,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(domain_id, local_part)
		)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			id TEXT PRIMARY KEY,
			domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			source TEXT NOT NULL UNIQUE,
			destination TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS folders (
			id TEXT PRIMARY KEY,
			mailbox_id TEXT NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(mailbox_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			mailbox_id TEXT NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
			folder_id TEXT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
			message_uid TEXT NOT NULL,
			message_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			from_addr TEXT NOT NULL,
			to_addrs TEXT NOT NULL,
			cc_addrs TEXT NOT NULL DEFAULT '[]',
			bcc_addrs TEXT NOT NULL DEFAULT '[]',
			sent_at TEXT NOT NULL,
			received_at TEXT NOT NULL,
			snippet TEXT NOT NULL,
			body_text TEXT NOT NULL,
			body_html TEXT NOT NULL,
			is_read INTEGER NOT NULL DEFAULT 0,
			is_starred INTEGER NOT NULL DEFAULT 0,
			has_attachments INTEGER NOT NULL DEFAULT 0,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			raw_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_mailbox_folder_received ON messages(mailbox_id, folder_id, received_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_search ON messages(mailbox_id, subject, from_addr, snippet)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_mailbox_raw_path ON messages(mailbox_id, raw_path) WHERE raw_path <> ''`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			storage_path TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, email)
		)`,
		`CREATE TABLE IF NOT EXISTS mail_rules (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			mailbox_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			from_contains TEXT NOT NULL DEFAULT '',
			subject_contains TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS blocked_senders (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			mailbox_id TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, mailbox_id, email)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_user ON contacts(user_id, email)`,
		`CREATE INDEX IF NOT EXISTS idx_mail_rules_user_mailbox ON mail_rules(user_id, mailbox_id, enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_blocked_senders_user_mailbox ON blocked_senders(user_id, mailbox_id, email)`,
	}
	for _, stmt := range stmts {
		if _, err := a.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) seed(ctx context.Context) error {
	var count int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	domainName := strings.Split(a.cfg.AdminEmail, "@")[1]
	domainID, err := a.createDomainTx(ctx, nil, domainName)
	if err != nil {
		return err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(a.cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := a.now().UTC().Format(time.RFC3339Nano)
	userID := newID("usr")
	_, err = a.db.ExecContext(ctx, `INSERT INTO users(id,email,display_name,role,password_hash,disabled,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, userID, a.cfg.AdminEmail, "LanQin Admin", "admin", string(passwordHash), 0, now, now)
	if err != nil {
		return err
	}

	local := strings.Split(a.cfg.AdminEmail, "@")[0]
	mailboxID, err := a.createMailbox(ctx, userID, domainID, local, "LanQin Admin", a.cfg.AdminPassword, 2048, "active")
	if err != nil {
		return err
	}
	if err := a.seedWelcomeMessage(ctx, mailboxID); err != nil {
		return err
	}
	a.log.Warn("created default administrator; change LANQIN_ADMIN_PASSWORD in production", "email", a.cfg.AdminEmail)
	return nil
}

func (a *App) createDomainTx(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	name = normalizeDomain(name)
	if name == "" || !strings.Contains(name, ".") {
		return "", errors.New("invalid domain")
	}
	selector := "lanqin"
	publicKey, privateKey, err := generateDKIMMaterial()
	if err != nil {
		return "", err
	}
	id := newID("dom")
	now := a.now().UTC().Format(time.RFC3339Nano)
	query := `INSERT INTO domains(id,name,status,dkim_selector,dkim_public_key,dkim_private_key,dns_status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`
	args := []any{id, name, "active", selector, publicKey, privateKey, "unchecked", now, now}
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
	} else {
		_, err = a.db.ExecContext(ctx, query, args...)
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func generateDKIMMaterial() (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return "", "", err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})
	return base64.StdEncoding.EncodeToString(pubDER), base64.StdEncoding.EncodeToString(privPEM), nil
}

func defaultFolderDefs() []struct{ name, role string } {
	return []struct{ name, role string }{
		{"Inbox", "inbox"},
		{"Sent", "sent"},
		{"Drafts", "drafts"},
		{"Archive", "archive"},
		{"Spam", "spam"},
		{"Trash", "trash"},
	}
}

func (a *App) createMailbox(ctx context.Context, userID, domainID, localPart, displayName, password string, quotaMB int, status string) (string, error) {
	localPart = normalizeLocalPart(localPart)
	if localPart == "" {
		return "", errors.New("invalid local part")
	}
	if quotaMB <= 0 {
		quotaMB = 1024
	}
	if status == "" {
		status = "active"
	}
	var domain string
	if err := a.db.QueryRowContext(ctx, `SELECT name FROM domains WHERE id=?`, domainID).Scan(&domain); err != nil {
		return "", err
	}
	address := localPart + "@" + domain
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if displayName == "" {
		displayName = address
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	id := newID("mbx")
	now := a.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO mailboxes(id,user_id,domain_id,local_part,address,display_name,password_hash,quota_mb,status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, userID, domainID, localPart, address, displayName, string(passwordHash), quotaMB, status, now, now)
	if err != nil {
		return "", err
	}
	for _, f := range defaultFolderDefs() {
		_, err = tx.ExecContext(ctx, `INSERT INTO folders(id,mailbox_id,name,role,created_at) VALUES(?,?,?,?,?)`, newID("fld"), id, f.name, f.role, now)
		if err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

func (a *App) seedWelcomeMessage(ctx context.Context, mailboxID string) error {
	folderID, err := a.ensureFolder(ctx, mailboxID, "Inbox")
	if err != nil {
		return err
	}
	now := a.now().UTC()
	msg := storedMessage{
		MailboxID:  mailboxID,
		FolderID:   folderID,
		MessageUID: newID("uid"),
		MessageID:  fmt.Sprintf("<%s@lanqin.local>", newID("msg")),
		Subject:    "欢迎使用 LanQin Email",
		From:       "system@lanqin.local",
		To:         []string{a.cfg.AdminEmail},
		SentAt:     now,
		ReceivedAt: now,
		Snippet:    "你的自建邮箱 Webmail 已经初始化完成。",
		BodyText:   "你的自建邮箱 Webmail 已经初始化完成。请尽快修改默认管理员密码，并配置 MX/SPF/DKIM/DMARC。",
		BodyHTML:   "<p>你的自建邮箱 Webmail 已经初始化完成。</p><p>请尽快修改默认管理员密码，并配置 MX/SPF/DKIM/DMARC。</p>",
		IsRead:     false,
	}
	_, err = a.insertMessage(ctx, msg, nil)
	return err
}
