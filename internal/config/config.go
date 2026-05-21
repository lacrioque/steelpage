package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Repo     Repo     `yaml:"repo"`
	Server   Server   `yaml:"server"`
	DB       DB       `yaml:"db"`
	Auth     Auth     `yaml:"auth"`
	Render   Render   `yaml:"render"`
	Search   Search   `yaml:"search"`
	Frontend Frontend `yaml:"frontend"`
	Email    Email    `yaml:"email"`
}

type Repo struct {
	Path              string `yaml:"path"`
	Branch            string `yaml:"branch"`
	CommitAuthorName  string `yaml:"commit_author_name"`
	CommitAuthorEmail string `yaml:"commit_author_email"`
	AutoPush          bool   `yaml:"auto_push"`
	PushRemote        string `yaml:"push_remote"`
}

type Server struct {
	Bind    string `yaml:"bind"`
	BaseURL string `yaml:"base_url"`
}

type DB struct {
	Path string `yaml:"path"`
}

type Auth struct {
	Mode               string      `yaml:"mode"`
	AllowAnonymousRead bool        `yaml:"allow_anonymous_read"`
	LocalEnabled       bool        `yaml:"local_enabled"`
	Session            AuthSession `yaml:"session"`
	OIDC               AuthOIDC    `yaml:"oidc"`
}

type AuthSession struct {
	Secure    bool   `yaml:"secure"`
	CookieKey string `yaml:"cookie_key"`
}

type AuthOIDC struct {
	Enabled      bool     `yaml:"enabled"`
	Label        string   `yaml:"label"`
	IssuerURL    string   `yaml:"issuer_url"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"`
	Scopes       []string `yaml:"scopes"`
}

type Render struct {
	AllowRawHTML     bool `yaml:"allow_raw_html"`
	Mermaid          bool `yaml:"mermaid"`
	CodeHighlighting bool `yaml:"code_highlighting"`
	SanitizeHTML     bool `yaml:"sanitize_html"`
}

type Search struct {
	Engine string `yaml:"engine"`
}

type Frontend struct {
	EmbeddedDist string `yaml:"embedded_dist"`
}

// Email holds SMTP transport settings. When Host is empty the mailer becomes
// a no-op — flows that try to send mail just log and skip.
type Email struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	Encryption         string `yaml:"encryption"` // "none" | "starttls" | "tls"
	FromAddress        string `yaml:"from_address"`
	FromName           string `yaml:"from_name"`
	ReplyTo            string `yaml:"reply_to"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"` // dev only — accepts self-signed
}

// Enabled reports whether SMTP is configured enough to attempt sending.
func (e Email) Enabled() bool {
	return e.Host != "" && e.FromAddress != ""
}

func Load(path string) (*Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := Config{
		Auth: Auth{
			LocalEnabled:       true,
			AllowAnonymousRead: true,
			Session: AuthSession{
				Secure: true,
			},
		},
	}
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if cfg.Repo.Path == "" {
		return nil, fmt.Errorf("config: repo.path is required")
	}
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "127.0.0.1:8080"
	}
	if cfg.Repo.CommitAuthorName == "" {
		cfg.Repo.CommitAuthorName = "Steelpage"
	}
	if cfg.Repo.CommitAuthorEmail == "" {
		cfg.Repo.CommitAuthorEmail = "steelpage@local"
	}
	if len(cfg.Auth.OIDC.Scopes) == 0 {
		cfg.Auth.OIDC.Scopes = []string{"openid", "email", "profile"}
	}

	if cfg.Email.Encryption == "" {
		cfg.Email.Encryption = "starttls"
	}
	if cfg.Email.Port == 0 {
		switch cfg.Email.Encryption {
		case "tls":
			cfg.Email.Port = 465
		case "none":
			cfg.Email.Port = 25
		default:
			cfg.Email.Port = 587
		}
	}

	return &cfg, nil
}
