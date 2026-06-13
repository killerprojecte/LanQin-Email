package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (a *App) handleListDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,name,status,dkim_selector,dkim_public_key,dns_status,dns_checked_at,created_at FROM domains ORDER BY name`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	defer rows.Close()
	items := []Domain{}
	for rows.Next() {
		var d Domain
		var checked sql.NullString
		var created string
		if err := rows.Scan(&d.ID, &d.Name, &d.Status, &d.DKIMSelector, &d.DKIMPublicKey, &d.DNSStatus, &checked, &created); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan domains")
			return
		}
		d.DNSCheckedAt = nullableTime(checked)
		d.CreatedAt = parseTime(created)
		items = append(items, d)
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	id, err := a.createDomainTx(r.Context(), nil, req.Name)
	if err != nil {
		badRequest(w, err)
		return
	}
	d, err := a.domainByID(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domain")
		return
	}
	respondJSON(w, http.StatusCreated, d)
}

func (a *App) handleListMailboxes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT mb.id,mb.user_id,u.email,mb.domain_id,mb.local_part,mb.address,mb.display_name,mb.quota_mb,mb.status,mb.created_at
		FROM mailboxes mb JOIN users u ON u.id=mb.user_id ORDER BY mb.address`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list mailboxes")
		return
	}
	defer rows.Close()
	items := []Mailbox{}
	for rows.Next() {
		var m Mailbox
		var created string
		if err := rows.Scan(&m.ID, &m.UserID, &m.UserEmail, &m.DomainID, &m.LocalPart, &m.Address, &m.DisplayName, &m.QuotaMB, &m.Status, &created); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan mailboxes")
			return
		}
		m.CreatedAt = parseTime(created)
		items = append(items, m)
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleCreateMailbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID    string `json:"domainId"`
		LocalPart   string `json:"localPart"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		QuotaMB     int    `json:"quotaMb"`
		Role        string `json:"role"`
		OwnerEmail  string `json:"ownerEmail"`
		UserID      string `json:"userId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	if err := requireString("domainId", req.DomainID); err != nil {
		badRequest(w, err)
		return
	}
	if err := requireString("localPart", req.LocalPart); err != nil {
		badRequest(w, err)
		return
	}
	if len(req.Password) < 8 {
		badRequest(w, errors.New("password must be at least 8 characters"))
		return
	}
	role := req.Role
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		badRequest(w, errors.New("invalid role"))
		return
	}

	domain, err := a.domainByID(r.Context(), req.DomainID)
	if err != nil {
		respondError(w, http.StatusNotFound, "domain not found")
		return
	}
	local := normalizeLocalPart(req.LocalPart)
	address := local + "@" + domain.Name

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()
	now := a.now().UTC().Format(time.RFC3339Nano)
	userID := strings.TrimSpace(req.UserID)
	displayName := req.DisplayName
	if displayName == "" {
		displayName = address
	}
	if userID != "" {
		var disabled int
		if err := tx.QueryRowContext(r.Context(), `SELECT disabled FROM users WHERE id=?`, userID).Scan(&disabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(w, http.StatusNotFound, "owner user not found")
			} else {
				respondError(w, http.StatusInternalServerError, "failed to load owner user")
			}
			return
		}
		if intBool(disabled) {
			badRequest(w, errors.New("owner user is disabled"))
			return
		}
	} else {
		ownerEmail := normalizeEmail(req.OwnerEmail)
		if ownerEmail == "" {
			ownerEmail = address
		}
		if !strings.Contains(ownerEmail, "@") {
			badRequest(w, errors.New("invalid owner email"))
			return
		}
		err = tx.QueryRowContext(r.Context(), `SELECT id FROM users WHERE email=? AND disabled=0`, ownerEmail).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to hash password")
				return
			}
			userID = newID("usr")
			ownerDisplayName := displayName
			if !strings.EqualFold(ownerEmail, address) {
				ownerDisplayName = ownerEmail
			}
			_, err = tx.ExecContext(r.Context(), `INSERT INTO users(id,email,display_name,role,password_hash,disabled,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?)`, userID, ownerEmail, ownerDisplayName, role, string(passwordHash), 0, now, now)
			if err != nil {
				badRequest(w, err)
				return
			}
		} else if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to load owner user")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to prepare owner user")
		return
	}

	mailboxID, err := a.createMailbox(r.Context(), userID, req.DomainID, local, displayName, req.Password, req.QuotaMB, "active")
	if err != nil {
		badRequest(w, err)
		return
	}
	m, err := a.mailboxByID(r.Context(), mailboxID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load mailbox")
		return
	}
	respondJSON(w, http.StatusCreated, m)
}

func (a *App) handleListAliases(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,domain_id,source,destination,enabled,created_at FROM aliases ORDER BY source`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list aliases")
		return
	}
	defer rows.Close()
	items := []Alias{}
	for rows.Next() {
		var item Alias
		var enabled int
		var created string
		if err := rows.Scan(&item.ID, &item.DomainID, &item.Source, &item.Destination, &enabled, &created); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan aliases")
			return
		}
		item.Enabled = intBool(enabled)
		item.CreatedAt = parseTime(created)
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleCreateAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DomainID    string `json:"domainId"`
		Source      string `json:"source"`
		Destination string `json:"destination"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	domain, err := a.domainByID(r.Context(), req.DomainID)
	if err != nil {
		respondError(w, http.StatusNotFound, "domain not found")
		return
	}
	source := normalizeEmail(req.Source)
	if !strings.Contains(source, "@") {
		source = normalizeLocalPart(source) + "@" + domain.Name
	}
	destination := normalizeEmail(req.Destination)
	if source == "" || destination == "" || !strings.Contains(destination, "@") {
		badRequest(w, errors.New("invalid alias"))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id := newID("als")
	now := a.now().UTC().Format(time.RFC3339Nano)
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO aliases(id,domain_id,source,destination,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		id, req.DomainID, source, destination, boolInt(enabled), now, now)
	if err != nil {
		badRequest(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, Alias{ID: id, DomainID: req.DomainID, Source: source, Destination: destination, Enabled: enabled, CreatedAt: parseTime(now)})
}

func (a *App) domainByID(ctx context.Context, id string) (*Domain, error) {
	row := a.db.QueryRowContext(ctx, `SELECT id,name,status,dkim_selector,dkim_public_key,dns_status,dns_checked_at,created_at FROM domains WHERE id=?`, id)
	var d Domain
	var checked sql.NullString
	var created string
	if err := row.Scan(&d.ID, &d.Name, &d.Status, &d.DKIMSelector, &d.DKIMPublicKey, &d.DNSStatus, &checked, &created); err != nil {
		return nil, err
	}
	d.DNSCheckedAt = nullableTime(checked)
	d.CreatedAt = parseTime(created)
	return &d, nil
}

func (a *App) mailboxByID(ctx context.Context, id string) (*Mailbox, error) {
	row := a.db.QueryRowContext(ctx, `SELECT id,user_id,domain_id,local_part,address,display_name,quota_mb,status,created_at FROM mailboxes WHERE id=?`, id)
	var m Mailbox
	var created string
	if err := row.Scan(&m.ID, &m.UserID, &m.DomainID, &m.LocalPart, &m.Address, &m.DisplayName, &m.QuotaMB, &m.Status, &created); err != nil {
		return nil, err
	}
	m.CreatedAt = parseTime(created)
	return &m, nil
}

func (a *App) mailboxForUser(ctx context.Context, userID string) (*Mailbox, error) {
	row := a.db.QueryRowContext(ctx, `SELECT id,user_id,domain_id,local_part,address,display_name,quota_mb,status,created_at FROM mailboxes WHERE user_id=? AND status='active' ORDER BY created_at LIMIT 1`, userID)
	var m Mailbox
	var created string
	if err := row.Scan(&m.ID, &m.UserID, &m.DomainID, &m.LocalPart, &m.Address, &m.DisplayName, &m.QuotaMB, &m.Status, &created); err != nil {
		return nil, err
	}
	m.CreatedAt = parseTime(created)
	return &m, nil
}

func (a *App) ensureFolder(ctx context.Context, mailboxID, folder string) (string, error) {
	var id string
	if err := a.db.QueryRowContext(ctx, `SELECT id FROM folders WHERE mailbox_id=? AND lower(name)=lower(?)`, mailboxID, folder).Scan(&id); err == nil {
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	role := strings.ToLower(folder)
	id = newID("fld")
	_, err := a.db.ExecContext(ctx, `INSERT INTO folders(id,mailbox_id,name,role,created_at) VALUES(?,?,?,?,?)`, id, mailboxID, folder, role, a.now().UTC().Format(time.RFC3339Nano))
	return id, err
}
