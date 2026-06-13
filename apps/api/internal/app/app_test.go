package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Addr:              ":0",
		DBPath:            filepath.Join(dir, "lanqin.db"),
		DataDir:           filepath.Join(dir, "data"),
		CookieName:        "lanqin_test",
		SessionTTLHours:   24,
		AdminEmail:        "admin@lanqin.local",
		AdminPassword:     "ChangeMe123!",
		PublicHostname:    "mail.example.test",
		PublicBaseURL:     "http://localhost:5173",
		AllowInsecureHTTP: true,
	}
	a, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

type testClient struct {
	t      *testing.T
	server *httptest.Server
	cookie *http.Cookie
}

func (c *testClient) do(method, path string, body any, out any) int {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.server.URL+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, cookie := range resp.Cookies() {
		if strings.Contains(cookie.Name, "lanqin") && cookie.Value != "" {
			c.cookie = cookie
		}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			c.t.Fatalf("decode %s %s: %v", method, path, err)
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode
}

func TestAuthAdminAndLocalDeliveryFlow(t *testing.T) {
	a := newTestApp(t)
	ts := httptest.NewServer(a.Router())
	defer ts.Close()
	admin := &testClient{t: t, server: ts}

	var login map[string]any
	if code := admin.do("POST", "/api/auth/login", map[string]string{"email": "admin@lanqin.local", "password": "ChangeMe123!"}, &login); code != http.StatusOK {
		t.Fatalf("login code=%d body=%v", code, login)
	}

	var domains struct {
		Items []Domain `json:"items"`
	}
	if code := admin.do("GET", "/api/admin/domains", nil, &domains); code != http.StatusOK || len(domains.Items) == 0 {
		t.Fatalf("domains code=%d items=%d", code, len(domains.Items))
	}
	domainID := domains.Items[0].ID

	var mb1 Mailbox
	if code := admin.do("POST", "/api/admin/mailboxes", map[string]any{"domainId": domainID, "localPart": "alice", "displayName": "Alice", "password": "Password123!"}, &mb1); code != http.StatusCreated {
		t.Fatalf("create alice code=%d mailbox=%+v", code, mb1)
	}
	var mb2 Mailbox
	if code := admin.do("POST", "/api/admin/mailboxes", map[string]any{"domainId": domainID, "localPart": "bob", "displayName": "Bob", "password": "Password123!"}, &mb2); code != http.StatusCreated {
		t.Fatalf("create bob code=%d mailbox=%+v", code, mb2)
	}

	var alias Alias
	if code := admin.do("POST", "/api/admin/aliases", map[string]any{"domainId": domainID, "source": "sales", "destination": mb1.Address}, &alias); code != http.StatusCreated {
		t.Fatalf("alias code=%d alias=%+v", code, alias)
	}

	alice := &testClient{t: t, server: ts}
	if code := alice.do("POST", "/api/auth/login", map[string]string{"email": mb1.Address, "password": "Password123!"}, &login); code != http.StatusOK {
		t.Fatalf("alice login=%d", code)
	}
	payload := map[string]any{
		"to":          []string{mb2.Address},
		"subject":     "hello bob",
		"html":        "<p>Hello <strong>Bob</strong></p><script>alert(1)</script>",
		"attachments": []map[string]string{{"filename": "note.txt", "contentType": "text/plain", "contentBase64": base64.StdEncoding.EncodeToString([]byte("hi"))}},
	}
	var sent MailMessage
	if code := alice.do("POST", "/api/mail/send", payload, &sent); code != http.StatusCreated || !sent.HasAttachments {
		t.Fatalf("send code=%d msg=%+v", code, sent)
	}

	bob := &testClient{t: t, server: ts}
	if code := bob.do("POST", "/api/auth/login", map[string]string{"email": mb2.Address, "password": "Password123!"}, &login); code != http.StatusOK {
		t.Fatalf("bob login=%d", code)
	}
	var list struct {
		Items      []MailMessage `json:"items"`
		NextCursor string        `json:"nextCursor"`
	}
	if code := bob.do("GET", "/api/mail/messages?folder=Inbox", nil, &list); code != http.StatusOK || len(list.Items) != 1 {
		t.Fatalf("bob inbox code=%d items=%d", code, len(list.Items))
	}
	if strings.Contains(list.Items[0].Snippet, "script") {
		t.Fatalf("message was not sanitized: %q", list.Items[0].Snippet)
	}

	var detail MailMessage
	if code := bob.do("GET", "/api/mail/messages/"+list.Items[0].ID, nil, &detail); code != http.StatusOK || len(detail.Attachments) != 1 || !detail.IsRead {
		t.Fatalf("detail code=%d detail=%+v", code, detail)
	}
	if strings.Contains(detail.BodyHTML, "script") {
		t.Fatalf("html was not sanitized: %s", detail.BodyHTML)
	}

	var ok map[string]any
	if code := bob.do("POST", "/api/mail/messages/"+detail.ID+"/star", map[string]bool{"starred": true}, &ok); code != http.StatusOK {
		t.Fatalf("star code=%d", code)
	}
	if code := bob.do("POST", "/api/mail/messages/"+detail.ID+"/move", map[string]string{"folder": "Archive"}, &ok); code != http.StatusOK {
		t.Fatalf("move code=%d", code)
	}
	if code := bob.do("DELETE", "/api/mail/messages/"+detail.ID, nil, &ok); code != http.StatusOK {
		t.Fatalf("delete code=%d", code)
	}
}

func TestUserCanSelectMultipleMailboxes(t *testing.T) {
	a := newTestApp(t)
	ts := httptest.NewServer(a.Router())
	defer ts.Close()
	admin := &testClient{t: t, server: ts}

	var login map[string]any
	if code := admin.do("POST", "/api/auth/login", map[string]string{"email": "admin@lanqin.local", "password": "ChangeMe123!"}, &login); code != http.StatusOK {
		t.Fatalf("login code=%d body=%v", code, login)
	}

	var domains struct {
		Items []Domain `json:"items"`
	}
	if code := admin.do("GET", "/api/admin/domains", nil, &domains); code != http.StatusOK || len(domains.Items) == 0 {
		t.Fatalf("domains code=%d items=%d", code, len(domains.Items))
	}
	domainID := domains.Items[0].ID

	var primary Mailbox
	if code := admin.do("POST", "/api/admin/mailboxes", map[string]any{"domainId": domainID, "localPart": "multi", "displayName": "Multi", "password": "Password123!"}, &primary); code != http.StatusCreated {
		t.Fatalf("create primary code=%d mailbox=%+v", code, primary)
	}
	var secondary Mailbox
	if code := admin.do("POST", "/api/admin/mailboxes", map[string]any{"domainId": domainID, "localPart": "multi-work", "displayName": "Multi Work", "password": "Password456!", "ownerEmail": primary.Address}, &secondary); code != http.StatusCreated {
		t.Fatalf("create secondary code=%d mailbox=%+v", code, secondary)
	}
	if primary.UserID != secondary.UserID {
		t.Fatalf("mailboxes were not bound to one user: primary=%s secondary=%s", primary.UserID, secondary.UserID)
	}

	userClient := &testClient{t: t, server: ts}
	if code := userClient.do("POST", "/api/auth/login", map[string]string{"email": primary.Address, "password": "Password123!"}, &login); code != http.StatusOK {
		t.Fatalf("user login=%d", code)
	}
	var mine struct {
		Items []Mailbox `json:"items"`
	}
	if code := userClient.do("GET", "/api/mail/mailboxes", nil, &mine); code != http.StatusOK || len(mine.Items) != 2 {
		t.Fatalf("my mailboxes code=%d items=%d", code, len(mine.Items))
	}
	if code := userClient.do("GET", "/api/mail/folders?mailboxId="+secondary.ID, nil, nil); code != http.StatusOK {
		t.Fatalf("folders for selected mailbox code=%d", code)
	}

	var sent MailMessage
	payload := map[string]any{
		"mailboxId": secondary.ID,
		"to":        []string{"admin@lanqin.local"},
		"subject":   "selected mailbox sender",
		"text":      "hello from selected mailbox",
	}
	if code := userClient.do("POST", "/api/mail/send", payload, &sent); code != http.StatusCreated || sent.From != secondary.Address {
		t.Fatalf("send with selected mailbox code=%d from=%q want=%q", code, sent.From, secondary.Address)
	}
	var adminInbox struct {
		Items []MailMessage `json:"items"`
	}
	if code := admin.do("GET", "/api/mail/messages?folder=Inbox&q=selected%20mailbox%20sender", nil, &adminInbox); code != http.StatusOK || len(adminInbox.Items) != 1 || adminInbox.Items[0].From != secondary.Address {
		t.Fatalf("admin inbox code=%d items=%d first=%+v", code, len(adminInbox.Items), adminInbox.Items)
	}
}

func TestProfileAndPasswordUpdate(t *testing.T) {
	a := newTestApp(t)
	ts := httptest.NewServer(a.Router())
	defer ts.Close()
	client := &testClient{t: t, server: ts}

	var login map[string]any
	if code := client.do("POST", "/api/auth/login", map[string]string{"email": "admin@lanqin.local", "password": "ChangeMe123!"}, &login); code != http.StatusOK {
		t.Fatalf("login code=%d body=%v", code, login)
	}

	var profile struct {
		User User `json:"user"`
	}
	if code := client.do("POST", "/api/me/profile", map[string]string{"displayName": "蓝钦管理员"}, &profile); code != http.StatusOK || profile.User.DisplayName != "蓝钦管理员" {
		t.Fatalf("profile code=%d user=%+v", code, profile.User)
	}

	var ok map[string]any
	if code := client.do("POST", "/api/me/password", map[string]string{"currentPassword": "wrong", "newPassword": "NewPassword123!"}, &ok); code != http.StatusUnauthorized {
		t.Fatalf("wrong password change code=%d", code)
	}
	if code := client.do("POST", "/api/me/password", map[string]string{"currentPassword": "ChangeMe123!", "newPassword": "NewPassword123!"}, &ok); code != http.StatusOK {
		t.Fatalf("password change code=%d body=%v", code, ok)
	}

	fresh := &testClient{t: t, server: ts}
	if code := fresh.do("POST", "/api/auth/login", map[string]string{"email": "admin@lanqin.local", "password": "ChangeMe123!"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("old password login code=%d", code)
	}
	if code := fresh.do("POST", "/api/auth/login", map[string]string{"email": "admin@lanqin.local", "password": "NewPassword123!"}, &login); code != http.StatusOK {
		t.Fatalf("new password login code=%d", code)
	}
}

func TestDNSRecords(t *testing.T) {
	a := newTestApp(t)
	d, err := a.domainByID(context.Background(), mustDefaultDomainID(t, a))
	if err != nil {
		t.Fatal(err)
	}
	records := a.dnsRecordsFor(d)
	if len(records) != 4 {
		t.Fatalf("records=%d", len(records))
	}
	if records[0].Type != "MX" || !strings.Contains(records[2].Value, "v=DKIM1") {
		t.Fatalf("unexpected records: %+v", records)
	}
}

func TestMaildirSyncImportsRFC822(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	root := t.TempDir()
	a.cfg.MaildirRoot = root

	mailboxes, err := a.maildirMailboxes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var admin maildirMailbox
	for _, mb := range mailboxes {
		if mb.Address == "admin@lanqin.local" {
			admin = mb
			break
		}
	}
	if admin.ID == "" {
		t.Fatal("admin mailbox not found")
	}

	dir := filepath.Join(root, admin.Domain, admin.LocalPart, "Maildir", "new")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"From: sender@example.test",
		"To: admin@lanqin.local",
		"Subject: Maildir import test",
		"Message-Id: <maildir-import@example.test>",
		"Date: Sat, 13 Jun 2026 13:00:00 +0000",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello from maildir",
	}, "\r\n")
	if err := os.WriteFile(filepath.Join(dir, "1749819600.M1P1.test"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := a.syncMaildirOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("imported=%d, want 1", count)
	}
	count, err = a.syncMaildirOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("second import=%d, want duplicate skip", count)
	}

	var subject, body string
	err = a.db.QueryRow(`SELECT subject, body_text FROM messages WHERE mailbox_id=? AND message_id='<maildir-import@example.test>'`, admin.ID).Scan(&subject, &body)
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Maildir import test" || !strings.Contains(body, "hello from maildir") {
		t.Fatalf("unexpected imported message subject=%q body=%q", subject, body)
	}
}

func mustDefaultDomainID(t *testing.T, a *App) string {
	t.Helper()
	var id string
	if err := a.db.QueryRow(`SELECT id FROM domains LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
