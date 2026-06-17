package app

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
)

type HTMLPolicy struct{ policy *bluemonday.Policy }

func NewHTMLPolicy() *HTMLPolicy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("style").OnElements("p", "span", "div", "table", "td", "th")
	return &HTMLPolicy{policy: p}
}

func (p *HTMLPolicy) Sanitize(s string) string {
	if p == nil || p.policy == nil {
		return s
	}
	return p.policy.Sanitize(s)
}

func newID(prefix string) string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf)
}

func randomToken() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	return s
}

var localPartRe = regexp.MustCompile(`[^a-z0-9._%+\-]`)

func normalizeLocalPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = localPartRe.ReplaceAllString(s, "")
	s = strings.Trim(s, ".")
	return s
}

func normalizeEmail(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.Contains(s, "@") {
		return s
	}
	parts := strings.SplitN(s, "@", 2)
	return normalizeLocalPart(parts[0]) + "@" + normalizeDomain(parts[1])
}

func dedupeEmails(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		email := normalizeEmail(item)
		if email == "" || !strings.Contains(email, "@") || seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, email)
	}
	return out
}

func jsonEncode(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func jsonDecodeSlice(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]any{"error": msg})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intBool(v int) bool { return v != 0 }

func nullableString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}

func nullableTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t := parseTime(v.String)
	return &t
}

func snippetFrom(text, html string) string {
	s := text
	if strings.TrimSpace(s) == "" {
		s = stripTags(html)
	}
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 160 {
		r := []rune(s)
		s = string(r[:160]) + "…"
	}
	return s
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				if unicode.IsSpace(r) {
					b.WriteRune(' ')
				} else {
					b.WriteRune(r)
				}
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func badRequest(w http.ResponseWriter, err error) {
	msg := "bad request"
	if err != nil {
		msg = err.Error()
	}
	respondError(w, http.StatusBadRequest, msg)
}

func requireString(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

var errNotFound = errors.New("not found")
