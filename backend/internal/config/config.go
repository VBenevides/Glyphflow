package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type Role string

const (
	ControlPlane Role = "control-plane"
	Worker       Role = "worker"
)

type Config struct {
	Role                          Role
	DatabaseURL                   string
	NATSURL                       string
	NATSCertFile                  string
	NATSKeyFile                   string
	NATSCAFile                    string
	AccessTokenSecret             string
	ControlPlaneSigningPrivateKey string
	SecretEncryptionKeyFile       string
	InstallationEncryptionKey     []byte
	PasswordPepper                string
	WebOrigin                     string
	CORSOrigins                   []string
	CSRFOrigins                   []string
	PasswordLoginEnabled          bool
	PasswordRegistrationEnabled   bool
	RequireUserApproval           bool
	DefaultRoleID                 string
	LockdownScheduler             bool
	BootstrapUsername             string
	BootstrapPassword             string
	BootstrapOIDCProvider         string
	BootstrapOIDCSubject          string
	SystemAdminEmails             []string
	Environment                   string
	AllowInsecureTransport        bool
	DataDir                       string
	LogMonthsKeep                 int
	AuditMonthsKeep               int
	RunnerMetricsMonthsKeep       int
	RunnerID                      string
	MaxMessageBytes               int
	MaxOutputBytes                int
}

func FromEnv(role Role) (Config, error) {
	systemAdminEmails, err := platform.ParseEmailList(os.Getenv("GLYPHFLOW_SYSTEM_ADMINS"))
	if err != nil {
		return Config{}, fmt.Errorf("GLYPHFLOW_SYSTEM_ADMINS: %w", err)
	}
	passwordLogin, err := envBool("ENABLE_PASSWORD_LOGIN", true)
	if err != nil {
		return Config{}, err
	}
	passwordRegistration, err := envBool("ENABLE_PASSWORD_REGISTRATION", true)
	if err != nil {
		return Config{}, err
	}
	userApproval, err := envBool("REQUIRE_USER_APPROVAL", false)
	if err != nil {
		return Config{}, err
	}
	csrfOrigins := parseOrigins(os.Getenv("CSRF_ORIGINS"))
	if len(csrfOrigins) == 0 {
		csrfOrigins = parseOrigins(os.Getenv("WEB_ORIGIN"))
	}
	config := Config{
		Role:                          role,
		DatabaseURL:                   os.Getenv("DATABASE_URL"),
		NATSURL:                       os.Getenv("NATS_URL"),
		NATSCertFile:                  os.Getenv("NATS_CERT_FILE"),
		NATSKeyFile:                   os.Getenv("NATS_KEY_FILE"),
		NATSCAFile:                    os.Getenv("NATS_CA_FILE"),
		AccessTokenSecret:             os.Getenv("ACCESS_TOKEN_SECRET"),
		ControlPlaneSigningPrivateKey: strings.TrimSpace(os.Getenv("CONTROL_PLANE_SIGNING_PRIVATE_KEY")),
		SecretEncryptionKeyFile:       strings.TrimSpace(os.Getenv("SECRET_ENCRYPTION_KEY_FILE")),
		PasswordPepper:                os.Getenv("PASSWORD_PEPPER"),
		WebOrigin:                     os.Getenv("WEB_ORIGIN"),
		CORSOrigins:                   parseOrigins(os.Getenv("CORS_ORIGIN")),
		CSRFOrigins:                   csrfOrigins,
		PasswordLoginEnabled:          passwordLogin,
		PasswordRegistrationEnabled:   passwordRegistration,
		RequireUserApproval:           userApproval,
		DefaultRoleID:                 strings.TrimSpace(envStringDefault("DEFAULT_ROLE_ID", "system-user")),
		BootstrapUsername:             strings.TrimSpace(os.Getenv("GLYPHFLOW_BOOTSTRAP_EMAIL")),
		BootstrapPassword:             os.Getenv("GLYPHFLOW_BOOTSTRAP_PASSWORD"),
		BootstrapOIDCProvider:         strings.TrimSpace(os.Getenv("GLYPHFLOW_BOOTSTRAP_OIDC_PROVIDER")),
		BootstrapOIDCSubject:          strings.TrimSpace(os.Getenv("GLYPHFLOW_BOOTSTRAP_OIDC_SUBJECT")),
		SystemAdminEmails:             systemAdminEmails,
		Environment:                   strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT"))),
		DataDir:                       os.Getenv("DATA_DIR"),
		LogMonthsKeep:                 3,
		AuditMonthsKeep:               12,
		RunnerMetricsMonthsKeep:       3,
		RunnerID:                      os.Getenv("RUNNER_ID"),
	}
	if config.AllowInsecureTransport, err = envBool("ALLOW_INSECURE_TRANSPORT", false); err != nil {
		return Config{}, err
	}
	if config.MaxMessageBytes, err = envInt("MAX_MESSAGE_BYTES"); err != nil {
		return Config{}, err
	}
	if role == Worker {
		if config.MaxOutputBytes, err = envInt("MAX_OUTPUT_BYTES"); err != nil {
			return Config{}, err
		}
	}
	if role == ControlPlane {
		if config.LogMonthsKeep, err = envIntDefault("LOG_MONTHS_KEEP", 3); err != nil {
			return Config{}, err
		}
		if config.AuditMonthsKeep, err = envIntDefault("AUDIT_MONTHS_KEEP", 12); err != nil {
			return Config{}, err
		}
		if config.RunnerMetricsMonthsKeep, err = envIntDefault("RUNNER_METRICS_MONTHS_KEEP", 3); err != nil {
			return Config{}, err
		}
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	if role == ControlPlane {
		config.InstallationEncryptionKey, err = loadInstallationEncryptionKey(config.Environment, config.SecretEncryptionKeyFile, config.AccessTokenSecret)
		if err != nil {
			return Config{}, err
		}
	}
	return config, nil
}

func parseOrigins(value string) []string {
	var origins []string
	for _, origin := range strings.Split(value, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func (c Config) Validate() error {
	if c.Role != ControlPlane && c.Role != Worker {
		return fmt.Errorf("role must be %q or %q", ControlPlane, Worker)
	}
	for _, email := range c.SystemAdminEmails {
		if _, err := platform.NormalizeEmail(email); err != nil {
			return fmt.Errorf("system admin email: %w", err)
		}
	}
	if err := requireURL("NATS_URL", c.NATSURL, "nats", "tls"); err != nil {
		return err
	}
	if !filepath.IsAbs(c.DataDir) {
		return errors.New("DATA_DIR must be an absolute path")
	}
	if c.MaxMessageBytes <= 0 {
		return errors.New("MAX_MESSAGE_BYTES must be greater than zero")
	}
	if c.Role == ControlPlane && (c.LogMonthsKeep <= 0 || c.AuditMonthsKeep <= 0 || c.RunnerMetricsMonthsKeep <= 0) {
		return errors.New("LOG_MONTHS_KEEP, AUDIT_MONTHS_KEEP, and RUNNER_METRICS_MONTHS_KEEP must be greater than zero")
	}
	if c.Role == ControlPlane {
		if len([]byte(c.AccessTokenSecret)) < 32 {
			return errors.New("ACCESS_TOKEN_SECRET must contain at least 32 bytes")
		}
		if c.PasswordLoginEnabled && len([]byte(c.PasswordPepper)) < 16 {
			return errors.New("PASSWORD_PEPPER must contain at least 16 bytes when password login is enabled")
		}
		if err := requireURL("WEB_ORIGIN", c.WebOrigin, "http", "https"); err != nil {
			return err
		}
		if !c.AllowInsecureTransport || c.Environment != "development" {
			if err := requireURL("WEB_ORIGIN", c.WebOrigin, "https"); err != nil {
				return err
			}
			if !strings.HasPrefix(c.NATSURL, "tls://") {
				return errors.New("NATS_URL must use TLS outside development")
			}
		}
		if c.Environment != "development" && c.ControlPlaneSigningPrivateKey == "" {
			return errors.New("CONTROL_PLANE_SIGNING_PRIVATE_KEY is required outside development")
		}
		if c.Environment != "development" && strings.TrimSpace(c.SecretEncryptionKeyFile) == "" {
			return errors.New("SECRET_ENCRYPTION_KEY_FILE is required outside development")
		}
		if c.ControlPlaneSigningPrivateKey != "" {
			raw, err := base64.RawStdEncoding.DecodeString(c.ControlPlaneSigningPrivateKey)
			if err != nil || len(raw) != ed25519.PrivateKeySize {
				return errors.New("CONTROL_PLANE_SIGNING_PRIVATE_KEY is invalid")
			}
		}
		if c.Environment == "production" && (c.NATSCertFile == "" || c.NATSKeyFile == "" || c.NATSCAFile == "") {
			return errors.New("production requires NATS client certificate, key, and CA files")
		}
		if err := requireURL("DATABASE_URL", c.DatabaseURL, "postgres", "postgresql"); err != nil {
			return err
		}
		if c.Environment != "development" && databaseSSLMode(c.DatabaseURL) != "verify-full" {
			return errors.New("DATABASE_URL must use sslmode=verify-full outside development")
		}
		return nil
	}
	if !runnerIDPattern.MatchString(c.RunnerID) {
		return errors.New("RUNNER_ID must contain only letters, digits, dot, underscore, or hyphen")
	}
	if c.MaxOutputBytes <= 0 || c.MaxOutputBytes > c.MaxMessageBytes {
		return errors.New("MAX_OUTPUT_BYTES must be greater than zero and no larger than MAX_MESSAGE_BYTES")
	}
	if c.Environment == "production" && (c.NATSCertFile == "" || c.NATSKeyFile == "" || c.NATSCAFile == "") {
		return errors.New("production requires NATS client certificate, key, and CA files")
	}
	if parsed, err := url.Parse(c.NATSURL); err != nil || parsed.User != nil {
		return errors.New("NATS_URL must not contain credentials")
	}
	if !c.AllowInsecureTransport || c.Environment != "development" {
		if !strings.HasPrefix(c.NATSURL, "tls://") {
			return errors.New("NATS_URL must use TLS outside development")
		}
	}
	return nil
}

const installationEncryptionKeySize = 32

func loadInstallationEncryptionKey(environment, path, fallbackSecret string) ([]byte, error) {
	if environment == "development" && path == "" {
		return []byte(fallbackSecret), nil
	}
	if path == "" {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE is required outside development")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_FILE: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_FILE: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 || info.Mode().Perm()&0o400 == 0 {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE must be readable only by its owner")
	}
	encoded, err := io.ReadAll(io.LimitReader(file, 129))
	if err != nil {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_FILE: %w", err)
	}
	if len(encoded) > 128 {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE is too large")
	}
	value := strings.TrimSpace(string(encoded))
	key, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		key, err = base64.RawStdEncoding.Strict().DecodeString(value)
	}
	if err != nil {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE must contain valid base64")
	}
	if len(key) != installationEncryptionKeySize {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE must decode to exactly 32 bytes")
	}
	return key, nil
}

var runnerIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func envInt(name string) (int, error) {
	value := os.Getenv(name)
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func envIntDefault(name string, fallback int) (int, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return envInt(name)
}

func envBool(name string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}

func envStringDefault(name, fallback string) string {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func requireURL(name, value string, schemes ...string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s must be a URL with a host", name)
	}
	for _, scheme := range schemes {
		if parsed.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported scheme %q", name, parsed.Scheme)
}

func databaseSSLMode(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("sslmode")
}
