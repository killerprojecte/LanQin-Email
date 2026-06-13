package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func (a *App) handleListContacts(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,user_id,name,email,note,created_at FROM contacts WHERE user_id=? ORDER BY name,email`, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load contacts")
		return
	}
	defer rows.Close()
	items := []Contact{}
	for rows.Next() {
		item, err := scanContact(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan contacts")
			return
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleCreateContact(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		badRequest(w, errors.New("invalid email"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = email
	}
	id := newID("ctc")
	now := a.now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO contacts(id,user_id,name,email,note,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id,email) DO UPDATE SET name=excluded.name,note=excluded.note,updated_at=excluded.updated_at`,
		id, user.ID, name, email, strings.TrimSpace(req.Note), now, now)
	if err != nil {
		badRequest(w, err)
		return
	}
	row := a.db.QueryRowContext(r.Context(), `SELECT id,user_id,name,email,note,created_at FROM contacts WHERE user_id=? AND email=?`, user.ID, email)
	item, err := scanContact(row)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load contact")
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

func (a *App) handleDeleteContact(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM contacts WHERE id=? AND user_id=?`, chi.URLParam(r, "id"), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete contact")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		respondError(w, http.StatusNotFound, "contact not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleListRules(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,user_id,mailbox_id,name,from_contains,subject_contains,action,enabled,created_at FROM mail_rules WHERE user_id=? ORDER BY created_at DESC`, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load rules")
		return
	}
	defer rows.Close()
	items := []MailRule{}
	for rows.Next() {
		item, err := scanRule(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan rules")
			return
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		MailboxID       string `json:"mailboxId"`
		Name            string `json:"name"`
		FromContains    string `json:"fromContains"`
		SubjectContains string `json:"subjectContains"`
		Action          string `json:"action"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	mailboxID, ok := a.optionalMailboxIDForUser(r, req.MailboxID)
	if !ok {
		respondError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	action := strings.TrimSpace(req.Action)
	if action != "archive" && action != "trash" && action != "star" && action != "mark-read" {
		badRequest(w, errors.New("invalid rule action"))
		return
	}
	fromContains := strings.TrimSpace(req.FromContains)
	subjectContains := strings.TrimSpace(req.SubjectContains)
	if fromContains == "" && subjectContains == "" {
		badRequest(w, errors.New("rule condition is required"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "收件规则"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id := newID("rule")
	now := a.now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO mail_rules(id,user_id,mailbox_id,name,from_contains,subject_contains,action,enabled,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, id, user.ID, mailboxID, name, fromContains, subjectContains, action, boolInt(enabled), now, now)
	if err != nil {
		badRequest(w, err)
		return
	}
	row := a.db.QueryRowContext(r.Context(), `SELECT id,user_id,mailbox_id,name,from_contains,subject_contains,action,enabled,created_at FROM mail_rules WHERE id=?`, id)
	item, err := scanRule(row)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load rule")
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

func (a *App) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM mail_rules WHERE id=? AND user_id=?`, chi.URLParam(r, "id"), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		respondError(w, http.StatusNotFound, "rule not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleListBlockedSenders(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,user_id,mailbox_id,email,reason,created_at FROM blocked_senders WHERE user_id=? ORDER BY created_at DESC`, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load blocked senders")
		return
	}
	defer rows.Close()
	items := []BlockedSender{}
	for rows.Next() {
		item, err := scanBlockedSender(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan blocked senders")
			return
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleCreateBlockedSender(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		MailboxID string `json:"mailboxId"`
		Email     string `json:"email"`
		Reason    string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	mailboxID, ok := a.optionalMailboxIDForUser(r, req.MailboxID)
	if !ok {
		respondError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		badRequest(w, errors.New("invalid email"))
		return
	}
	id := newID("blk")
	now := a.now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO blocked_senders(id,user_id,mailbox_id,email,reason,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id,mailbox_id,email) DO UPDATE SET reason=excluded.reason,updated_at=excluded.updated_at`,
		id, user.ID, mailboxID, email, strings.TrimSpace(req.Reason), now, now)
	if err != nil {
		badRequest(w, err)
		return
	}
	row := a.db.QueryRowContext(r.Context(), `SELECT id,user_id,mailbox_id,email,reason,created_at FROM blocked_senders WHERE user_id=? AND mailbox_id=? AND email=?`, user.ID, mailboxID, email)
	item, err := scanBlockedSender(row)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load blocked sender")
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

func (a *App) handleDeleteBlockedSender(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM blocked_senders WHERE id=? AND user_id=?`, chi.URLParam(r, "id"), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete blocked sender")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		respondError(w, http.StatusNotFound, "blocked sender not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMailStats(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	mailboxID := strings.TrimSpace(r.URL.Query().Get("mailboxId"))
	args := []any{user.ID}
	where := `mb.user_id=?`
	if mailboxID != "" {
		if _, err := a.mailboxForCurrentUserWithID(r, mailboxID); err != nil {
			respondError(w, http.StatusNotFound, "mailbox not found")
			return
		}
		where += ` AND mb.id=?`
		args = append(args, mailboxID)
	}
	stats := MailStats{ByFolder: []MailStatsFolderCount{}}
	row := a.db.QueryRowContext(r.Context(), `SELECT COUNT(m.id),COALESCE(SUM(CASE WHEN m.is_read=0 THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN m.is_starred=1 THEN 1 ELSE 0 END),0),COALESCE(SUM(m.size_bytes),0)
		FROM mailboxes mb LEFT JOIN messages m ON m.mailbox_id=mb.id WHERE `+where, args...)
	if err := row.Scan(&stats.TotalMessages, &stats.UnreadMessages, &stats.StarredMessages, &stats.StorageBytes); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(a.id) FROM attachments a JOIN messages m ON m.id=a.message_id JOIN mailboxes mb ON mb.id=m.mailbox_id WHERE `+where, args...).Scan(&stats.AttachmentCount); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load attachment stats")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT f.name,f.role,COUNT(m.id),COALESCE(SUM(CASE WHEN m.is_read=0 THEN 1 ELSE 0 END),0),COALESCE(SUM(m.size_bytes),0)
		FROM mailboxes mb JOIN folders f ON f.mailbox_id=mb.id LEFT JOIN messages m ON m.folder_id=f.id
		WHERE `+where+` GROUP BY f.id,f.name,f.role ORDER BY CASE f.role WHEN 'inbox' THEN 1 WHEN 'sent' THEN 2 WHEN 'drafts' THEN 3 WHEN 'archive' THEN 4 WHEN 'spam' THEN 5 WHEN 'trash' THEN 6 ELSE 99 END`, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load folder stats")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item MailStatsFolderCount
		if err := rows.Scan(&item.Folder, &item.Role, &item.Count, &item.Unread, &item.Bytes); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan folder stats")
			return
		}
		stats.ByFolder = append(stats.ByFolder, item)
	}
	respondJSON(w, http.StatusOK, stats)
}

func (a *App) handleMailCleanup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MailboxID string `json:"mailboxId"`
		Target    string `json:"target"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	mb, err := a.mailboxForCurrentUserWithID(r, req.MailboxID)
	if err != nil {
		respondError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	target := strings.TrimSpace(req.Target)
	affected := int64(0)
	switch target {
	case "empty-trash":
		affected, err = a.deleteMessagesInFolder(r.Context(), mb.ID, "Trash")
	case "empty-spam":
		affected, err = a.deleteMessagesInFolder(r.Context(), mb.ID, "Spam")
	case "archive-read-inbox":
		affected, err = a.archiveReadInbox(r.Context(), mb.ID)
	default:
		badRequest(w, errors.New("invalid cleanup target"))
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to cleanup messages")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true, "affected": affected})
}

func (a *App) optionalMailboxIDForUser(r *http.Request, mailboxID string) (string, bool) {
	mailboxID = strings.TrimSpace(mailboxID)
	if mailboxID == "" || mailboxID == "all" {
		return "", true
	}
	_, err := a.mailboxForCurrentUserWithID(r, mailboxID)
	return mailboxID, err == nil
}

func (a *App) deleteMessagesInFolder(ctx context.Context, mailboxID, folder string) (int64, error) {
	folderID, err := a.ensureFolder(ctx, mailboxID, folder)
	if err != nil {
		return 0, err
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id FROM messages WHERE mailbox_id=? AND folder_id=?`, mailboxID, folderID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		a.deleteMessageFiles(ctx, id)
		if _, err := a.db.ExecContext(ctx, `DELETE FROM messages WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	return int64(len(ids)), nil
}

func (a *App) archiveReadInbox(ctx context.Context, mailboxID string) (int64, error) {
	inboxID, err := a.ensureFolder(ctx, mailboxID, "Inbox")
	if err != nil {
		return 0, err
	}
	archiveID, err := a.ensureFolder(ctx, mailboxID, "Archive")
	if err != nil {
		return 0, err
	}
	res, err := a.db.ExecContext(ctx, `UPDATE messages SET folder_id=?, updated_at=? WHERE mailbox_id=? AND folder_id=? AND is_read=1`,
		archiveID, a.now().UTC().Format(time.RFC3339Nano), mailboxID, inboxID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanContact(row messageSummaryScanner) (Contact, error) {
	var item Contact
	var created string
	err := row.Scan(&item.ID, &item.UserID, &item.Name, &item.Email, &item.Note, &created)
	item.CreatedAt = parseTime(created)
	return item, err
}

func scanRule(row messageSummaryScanner) (MailRule, error) {
	var item MailRule
	var enabled int
	var created string
	err := row.Scan(&item.ID, &item.UserID, &item.MailboxID, &item.Name, &item.FromContains, &item.SubjectContains, &item.Action, &enabled, &created)
	item.Enabled = intBool(enabled)
	item.CreatedAt = parseTime(created)
	return item, err
}

func scanBlockedSender(row messageSummaryScanner) (BlockedSender, error) {
	var item BlockedSender
	var created string
	err := row.Scan(&item.ID, &item.UserID, &item.MailboxID, &item.Email, &item.Reason, &created)
	item.CreatedAt = parseTime(created)
	return item, err
}

func (a *App) applyInboundControls(ctx context.Context, messageID, mailboxID, from, subject string) {
	var userID string
	if err := a.db.QueryRowContext(ctx, `SELECT user_id FROM mailboxes WHERE id=?`, mailboxID).Scan(&userID); err != nil {
		return
	}
	from = normalizeEmail(from)
	var blocked int
	_ = a.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM blocked_senders WHERE user_id=? AND (mailbox_id='' OR mailbox_id=?) AND email=?`, userID, mailboxID, from).Scan(&blocked)
	if blocked > 0 {
		if spamID, err := a.ensureFolder(ctx, mailboxID, "Spam"); err == nil {
			_, _ = a.db.ExecContext(ctx, `UPDATE messages SET folder_id=?, updated_at=? WHERE id=?`, spamID, a.now().UTC().Format(time.RFC3339Nano), messageID)
		}
		return
	}
	rows, err := a.db.QueryContext(ctx, `SELECT from_contains,subject_contains,action FROM mail_rules WHERE user_id=? AND (mailbox_id='' OR mailbox_id=?) AND enabled=1 ORDER BY created_at`, userID, mailboxID)
	if err != nil {
		return
	}
	defer rows.Close()
	lowerFrom := strings.ToLower(from)
	lowerSubject := strings.ToLower(subject)
	for rows.Next() {
		var fromContains, subjectContains, action string
		if rows.Scan(&fromContains, &subjectContains, &action) != nil {
			continue
		}
		if fromContains != "" && !strings.Contains(lowerFrom, strings.ToLower(fromContains)) {
			continue
		}
		if subjectContains != "" && !strings.Contains(lowerSubject, strings.ToLower(subjectContains)) {
			continue
		}
		switch action {
		case "archive":
			if folderID, err := a.ensureFolder(ctx, mailboxID, "Archive"); err == nil {
				_, _ = a.db.ExecContext(ctx, `UPDATE messages SET folder_id=?, updated_at=? WHERE id=?`, folderID, a.now().UTC().Format(time.RFC3339Nano), messageID)
			}
		case "trash":
			if folderID, err := a.ensureFolder(ctx, mailboxID, "Trash"); err == nil {
				_, _ = a.db.ExecContext(ctx, `UPDATE messages SET folder_id=?, updated_at=? WHERE id=?`, folderID, a.now().UTC().Format(time.RFC3339Nano), messageID)
			}
		case "star":
			_, _ = a.db.ExecContext(ctx, `UPDATE messages SET is_starred=1, updated_at=? WHERE id=?`, a.now().UTC().Format(time.RFC3339Nano), messageID)
		case "mark-read":
			_, _ = a.db.ExecContext(ctx, `UPDATE messages SET is_read=1, updated_at=? WHERE id=?`, a.now().UTC().Format(time.RFC3339Nano), messageID)
		}
	}
}
