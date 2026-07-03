package auth_test

// Defense-in-depth checks: machine identities (role='machine') must never be
// able to log in or receive password-reset tokens — even when a row was
// tampered into having an email and password hash.

import (
	"database/sql"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/markusfluer/steelpage/internal/auth"
	"github.com/markusfluer/steelpage/internal/config"
	"github.com/markusfluer/steelpage/internal/configsvc"
	"github.com/markusfluer/steelpage/internal/db"
	"github.com/markusfluer/steelpage/internal/mailer"
	"github.com/markusfluer/steelpage/internal/users"
)

func machineAuthSetup(t *testing.T) (*httptest.Server, *http.Client, *users.Store, *sql.DB) {
	t.Helper()

	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	cfg := &config.Config{
		Auth: config.Auth{
			LocalEnabled: true,
			Session:      config.AuthSession{Secure: false},
		},
		Server: config.Server{Bind: "127.0.0.1:0", BaseURL: "http://test.local"},
	}

	ustore := users.New(d)
	sm := scs.New()
	sm.Cookie.Name = "steelpage_session"
	sm.Lifetime = time.Hour

	cfgsvc, err := configsvc.New(d, cfg)
	if err != nil {
		t.Fatalf("configsvc: %v", err)
	}
	svc := auth.New(cfg, ustore, sm, mailer.New(cfg.Email), d, cfgsvc)

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Post("/api/auth/login", svc.Login)
	r.Post("/api/auth/forgot", svc.ForgotPassword)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	return ts, &http.Client{Jar: jar}, ustore, d
}

// machineAuthTamper creates a machine user and force-sets an email plus a
// valid bcrypt hash, so only the explicit role guards can stop the flows.
func machineAuthTamper(t *testing.T, ustore *users.Store, d *sql.DB, email, password string) *users.User {
	t.Helper()
	m, err := ustore.CreateMachine("rogue-bot")
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if _, err := d.Exec(`UPDATE users SET email = ?, password_hash = ? WHERE id = ?`, email, string(hash), m.ID); err != nil {
		t.Fatalf("tamper machine user: %v", err)
	}
	return m
}

func TestLoginRefusesMachineUser(t *testing.T) {
	ts, client, ustore, d := machineAuthSetup(t)
	machineAuthTamper(t, ustore, d, "bot@example.com", "password123")

	res := postJSON(t, client, ts.URL+"/api/auth/login", map[string]string{
		"email":    "bot@example.com",
		"password": "password123",
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("machine login: status = %d, want 401", res.StatusCode)
	}
}

func TestForgotPasswordRefusesMachineUser(t *testing.T) {
	ts, client, ustore, d := machineAuthSetup(t)
	machineAuthTamper(t, ustore, d, "bot@example.com", "password123")

	res := postJSON(t, client, ts.URL+"/api/auth/forgot", map[string]string{
		"email": "bot@example.com",
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("forgot: status = %d, want 204", res.StatusCode)
	}

	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM password_reset_tokens`).Scan(&n); err != nil {
		t.Fatalf("count reset tokens: %v", err)
	}
	if n != 0 {
		t.Fatalf("machine user got %d reset token(s), want 0", n)
	}
}
