package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr               string
	DBPath             string
	DataDir            string
	CookieName         string
	SessionTTLHours    int
	AdminEmail         string
	AdminPassword      string
	PublicHostname     string
	PublicBaseURL      string
	SMTPHost           string
	SMTPPort           string
	SMTPUsername       string
	SMTPPassword       string
	SMTPRequireTLS     bool
	MaildirRoot        string
	MaildirScanSeconds int
	AllowInsecureHTTP  bool
}

func LoadConfig() Config {
	dataDir := getenv("LANQIN_DATA_DIR", "./data")
	return Config{
		Addr:               getenv("LANQIN_ADDR", ":8080"),
		DBPath:             getenv("LANQIN_DB_PATH", filepath.Join(dataDir, "lanqin.db")),
		DataDir:            dataDir,
		CookieName:         getenv("LANQIN_COOKIE_NAME", "lanqin_session"),
		SessionTTLHours:    getenvInt("LANQIN_SESSION_TTL_HOURS", 24*7),
		AdminEmail:         strings.ToLower(getenv("LANQIN_ADMIN_EMAIL", "admin@lanqin.local")),
		AdminPassword:      getenv("LANQIN_ADMIN_PASSWORD", "ChangeMe123!"),
		PublicHostname:     getenv("LANQIN_PUBLIC_HOSTNAME", "mail.lanqin.local"),
		PublicBaseURL:      getenv("LANQIN_PUBLIC_BASE_URL", "http://localhost:5173"),
		SMTPHost:           getenv("LANQIN_SMTP_HOST", ""),
		SMTPPort:           getenv("LANQIN_SMTP_PORT", "25"),
		SMTPUsername:       getenv("LANQIN_SMTP_USERNAME", ""),
		SMTPPassword:       getenv("LANQIN_SMTP_PASSWORD", ""),
		SMTPRequireTLS:     getenvBool("LANQIN_SMTP_REQUIRE_TLS", false),
		MaildirRoot:        getenv("LANQIN_MAILDIR_ROOT", ""),
		MaildirScanSeconds: getenvInt("LANQIN_MAILDIR_SCAN_SECONDS", 30),
		AllowInsecureHTTP:  getenvBool("LANQIN_ALLOW_INSECURE_HTTP", true),
	}
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
