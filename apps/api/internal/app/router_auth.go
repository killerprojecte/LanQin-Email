package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const userContextKey contextKey = "user"

func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(a.corsMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]any{"ok": true, "time": a.now().UTC()})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", a.handleLogin)
		r.Post("/auth/logout", a.handleLogout)
		r.With(a.requireAuth).Get("/me", a.handleMe)
		r.With(a.requireAuth).Post("/me/profile", a.handleUpdateProfile)
		r.With(a.requireAuth).Post("/me/password", a.handleChangePassword)
		r.With(a.requireAuth).Get("/me/contacts", a.handleListContacts)
		r.With(a.requireAuth).Post("/me/contacts", a.handleCreateContact)
		r.With(a.requireAuth).Delete("/me/contacts/{id}", a.handleDeleteContact)
		r.With(a.requireAuth).Get("/me/rules", a.handleListRules)
		r.With(a.requireAuth).Post("/me/rules", a.handleCreateRule)
		r.With(a.requireAuth).Delete("/me/rules/{id}", a.handleDeleteRule)
		r.With(a.requireAuth).Get("/me/blocked-senders", a.handleListBlockedSenders)
		r.With(a.requireAuth).Post("/me/blocked-senders", a.handleCreateBlockedSender)
		r.With(a.requireAuth).Delete("/me/blocked-senders/{id}", a.handleDeleteBlockedSender)
		r.With(a.requireAuth).Get("/me/stats", a.handleMailStats)
		r.With(a.requireAuth).Post("/me/cleanup", a.handleMailCleanup)
		r.With(a.requireAuth).Get("/events", a.handleEvents)

		r.Group(func(r chi.Router) {
			r.Use(a.requireAuth)
			r.Get("/mail/mailboxes", a.handleMyMailboxes)
			r.Get("/mail/folders", a.handleMailFolders)
			r.Get("/mail/messages", a.handleMailMessages)
			r.Get("/mail/messages/{id}", a.handleMailMessage)
			r.Post("/mail/send", a.handleMailSend)
			r.Post("/mail/messages/{id}/mark-read", a.handleMarkRead)
			r.Post("/mail/messages/{id}/star", a.handleStar)
			r.Post("/mail/messages/{id}/move", a.handleMove)
			r.Delete("/mail/messages/{id}", a.handleDeleteMessage)
			r.Get("/mail/attachments/{id}", a.handleAttachment)
		})

		r.Group(func(r chi.Router) {
			r.Use(a.requireAuth)
			r.Use(a.requireAdmin)
			r.Get("/admin/domains", a.handleListDomains)
			r.Post("/admin/domains", a.handleCreateDomain)
			r.Get("/admin/mailboxes", a.handleListMailboxes)
			r.Post("/admin/mailboxes", a.handleCreateMailbox)
			r.Get("/admin/aliases", a.handleListAliases)
			r.Post("/admin/aliases", a.handleCreateAlias)
			r.Get("/admin/domains/{id}/dns-records", a.handleDNSRecords)
			r.Post("/admin/domains/{id}/check-dns", a.handleDNSCheck)
		})
	})

	return r
}

func (a *App) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") || origin == a.cfg.PublicBaseURL) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	email := normalizeEmail(req.Email)
	user, passwordHash, err := a.userByEmail(r.Context(), email)
	if err != nil || user.Disabled {
		respondError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		respondError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	token := randomToken()
	sessionID := newID("ses")
	expires := a.now().UTC().Add(time.Duration(a.cfg.SessionTTLHours) * time.Hour)
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at) VALUES(?,?,?,?,?)`,
		sessionID, user.ID, hashToken(token), expires.Format(time.RFC3339Nano), a.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.cfg.AllowInsecureHTTP,
	})
	respondJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(a.cfg.CookieName); err == nil {
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash=?`, hashToken(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: a.cfg.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"user": currentUser(r)})
}

func (a *App) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		DisplayName string `json:"displayName"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		badRequest(w, errors.New("displayName is required"))
		return
	}
	if len([]rune(displayName)) > 80 {
		badRequest(w, errors.New("displayName must be at most 80 characters"))
		return
	}
	_, err := a.db.ExecContext(r.Context(), `UPDATE users SET display_name=?, updated_at=? WHERE id=?`,
		displayName, a.now().UTC().Format(time.RFC3339Nano), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	updated, err := a.userByID(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		badRequest(w, err)
		return
	}
	if len(req.NewPassword) < 8 {
		badRequest(w, errors.New("newPassword must be at least 8 characters"))
		return
	}
	row := a.db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=?`, user.ID)
	var currentHash string
	if err := row.Scan(&currentHash); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
		respondError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	now := a.now().UTC().Format(time.RFC3339Nano)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, string(newHash), now, user.ID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE mailboxes SET password_hash=?, updated_at=? WHERE user_id=?`, string(newHash), now, user.ID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update mailbox password")
		return
	}
	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save password")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := a.authenticateRequest(r)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (a *App) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if user == nil || user.Role != "admin" {
			respondError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) *User {
	user, _ := r.Context().Value(userContextKey).(*User)
	return user
}

func (a *App) authenticateRequest(r *http.Request) (*User, error) {
	cookie, err := r.Cookie(a.cfg.CookieName)
	if err != nil || cookie.Value == "" {
		return nil, errors.New("no session")
	}
	row := a.db.QueryRowContext(r.Context(), `SELECT u.id,u.email,u.display_name,u.role,u.disabled,u.created_at
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=? AND s.expires_at > ?`, hashToken(cookie.Value), a.now().UTC().Format(time.RFC3339Nano))
	var u User
	var disabled int
	var created string
	if err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &disabled, &created); err != nil {
		return nil, err
	}
	u.Disabled = intBool(disabled)
	u.CreatedAt = parseTime(created)
	if u.Disabled {
		return nil, errors.New("disabled")
	}
	return &u, nil
}

func (a *App) userByEmail(ctx context.Context, email string) (*User, string, error) {
	row := a.db.QueryRowContext(ctx, `SELECT id,email,display_name,role,password_hash,disabled,created_at FROM users WHERE email=?`, email)
	var u User
	var passwordHash string
	var disabled int
	var created string
	if err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &passwordHash, &disabled, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", errNotFound
		}
		return nil, "", err
	}
	u.Disabled = intBool(disabled)
	u.CreatedAt = parseTime(created)
	return &u, passwordHash, nil
}

func (a *App) userByID(ctx context.Context, id string) (*User, error) {
	row := a.db.QueryRowContext(ctx, `SELECT id,email,display_name,role,disabled,created_at FROM users WHERE id=?`, id)
	var u User
	var disabled int
	var created string
	if err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &disabled, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errNotFound
		}
		return nil, err
	}
	u.Disabled = intBool(disabled)
	u.CreatedAt = parseTime(created)
	return &u, nil
}
