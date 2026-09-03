package config

import (
	"crypto/ed25519"
	"crypto/rand"
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
	DatabaseMode                  string
	DatabaseURL                   string
	NATSMode                      string
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
	DatabaseStorageCapacityBytes  int64
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
	userApproval, err := envBool("REQUIRE_USER_APPROVAL", true)
	if err != nil {
		return Config{}, err
	}
	csrfOrigins := parseOrigins(os.Getenv("CSRF_ORIGINS"))
	if len(csrfOrigins) == 0 {
		csrfOrigins = parseOrigins(os.Getenv("WEB_ORIGIN"))
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	databaseMode := normalizeDatabaseMode(os.Getenv("GLYPHFLOW_DATABASE"), databaseURL)
	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
	natsMode := normalizeNATSMode(os.Getenv("GLYPHFLOW_NATS"), natsURL)
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" && databaseMode == "sqlite" {
		dataDir, _ = filepath.Abs(".dev-data")
	}
	if databaseMode == "sqlite" && databaseURL == "" {
		databaseURL = filepath.Join(dataDir, "controlplane.sqlite")
	}
	config := Config{
		Role:                          role,
		DatabaseMode:                  databaseMode,
		DatabaseURL:                   databaseURL,
		NATSMode:                      natsMode,
		NATSURL:                       natsURL,
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
		DataDir:                       dataDir,
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
		if config.DatabaseStorageCapacityBytes, err = envInt64Default("DATABASE_STORAGE_CAPACITY_BYTES", 0); err != nil {
			return Config{}, err
		}
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	if role == ControlPlane {
		if config.SecretEncryptionKeyFile == "" {
			config.SecretEncryptionKeyFile = filepath.Join(config.DataDir, "secret-encryption.key")
		}
		config.InstallationEncryptionKey, err = loadInstallationEncryptionKey(config.SecretEncryptionKeyFile)
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
	databaseMode := normalizeDatabaseMode(c.DatabaseMode, c.DatabaseURL)
	if databaseMode == "" {
		return errors.New("GLYPHFLOW_DATABASE must be sqlite, psql, or postgresql")
	}
	natsMode := normalizeNATSMode(c.NATSMode, c.NATSURL)
	if natsMode == "" {
		return errors.New("GLYPHFLOW_NATS must be embed or remote")
	}
	if err := c.validateNATS(natsMode); err != nil {
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
	if c.Role == ControlPlane && c.DatabaseStorageCapacityBytes < 0 {
		return errors.New("DATABASE_STORAGE_CAPACITY_BYTES must not be negative")
	}
	if c.Role == ControlPlane {
		return c.validateControlPlane(databaseMode, natsMode)
	}
	return c.validateWorker(natsMode)
}

func (c Config) validateNATS(natsMode string) error {
	if natsMode == "remote" || c.Role == Worker {
		if err := requireURL("NATS_URL", c.NATSURL, "nats", "tls"); err != nil {
			return err
		}
	}
	if c.Role == Worker && natsMode == "embedded" && c.Environment != "development" {
		return errors.New("GLYPHFLOW_NATS=embed for workers is supported only in development")
	}
	return nil
}

func (c Config) validateControlPlane(databaseMode, natsMode string) error {
	if c.Environment != "development" && databaseMode == "sqlite" {
		return errors.New("GLYPHFLOW_DATABASE=sqlite is supported only in development")
	}
	if c.Environment == "production" && natsMode == "embedded" {
		return errors.New("GLYPHFLOW_NATS=embed is not supported in production")
	}
	if len([]byte(c.AccessTokenSecret)) < 32 {
		return errors.New("ACCESS_TOKEN_SECRET must contain at least 32 bytes")
	}
	if c.PasswordLoginEnabled && len([]byte(c.PasswordPepper)) < 16 {
		return errors.New("PASSWORD_PEPPER must contain at least 16 bytes when password login is enabled")
	}
	if err := c.validateControlPlaneSecurity(natsMode); err != nil {
		return err
	}
	return c.validateDatabase(databaseMode)
}

func (c Config) validateControlPlaneSecurity(natsMode string) error {
	if err := requireURL("WEB_ORIGIN", c.WebOrigin, "http", "https"); err != nil {
		return err
	}
	if !c.AllowInsecureTransport || (c.Environment != "local" && c.Environment != "development") {
		if err := requireURL("WEB_ORIGIN", c.WebOrigin, "https"); err != nil {
			return err
		}
		if natsMode != "remote" || !strings.HasPrefix(c.NATSURL, "tls://") {
			return errors.New("NATS_URL must use TLS outside development")
		}
	}
	if c.Environment != "development" && c.ControlPlaneSigningPrivateKey == "" {
		return errors.New("CONTROL_PLANE_SIGNING_PRIVATE_KEY is required outside development")
	}
	if c.Environment != "development" && c.DatabaseStorageCapacityBytes <= 0 {
		return errors.New("DATABASE_STORAGE_CAPACITY_BYTES must be greater than zero outside development")
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
	return nil
}

func (c Config) validateDatabase(databaseMode string) error {
	if databaseMode == "postgresql" {
		if err := requireURL("DATABASE_URL", c.DatabaseURL, "postgres", "postgresql"); err != nil {
			return err
		}
		if c.Environment != "development" && databaseSSLMode(c.DatabaseURL) != "verify-full" {
			return errors.New("DATABASE_URL must use sslmode=verify-full outside development")
		}
		return nil
	}
	path := strings.TrimSpace(c.DatabaseURL)
	if path == "" {
		return errors.New("DATABASE_URL must contain the SQLite database path")
	}
	if strings.Contains(path, "://") && !strings.HasPrefix(strings.ToLower(path), "file://") {
		return errors.New("DATABASE_URL must contain a SQLite database path")
	}
	return nil
}

func (c Config) validateWorker(natsMode string) error {
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

func normalizeDatabaseMode(value, databaseURL string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(databaseURL)), "postgres") {
			return "postgresql"
		}
		return "sqlite"
	}
	if value == "psql" {
		return "postgresql"
	}
	if value == "sqlite" || value == "postgresql" {
		return value
	}
	return ""
}

func normalizeNATSMode(value, natsURL string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		if strings.TrimSpace(natsURL) != "" {
			return "remote"
		}
		return "embedded"
	}
	if value == "embed" || value == "embedded" {
		return "embedded"
	}
	if value == "remote" {
		return value
	}
	return ""
}

const installationEncryptionKeySize = 32

const installationEncryptionKeyWarning = `WARNING: The encryption key file was not found and a new random encryption key is being generated.
This file is required to decrypt secrets stored by the application.
If the file is deleted, lost, or replaced, secrets encrypted with the previous key cannot be recovered.
Affected secrets must be replaced or re-entered and encrypted using the new key.
Existing encrypted secrets may no longer be decryptable.`

func loadInstallationEncryptionKey(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("SECRET_ENCRYPTION_KEY_FILE path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_FILE: %w", err)
		}
		key := make([]byte, installationEncryptionKeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate SECRET_ENCRYPTION_KEY_FILE: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create SECRET_ENCRYPTION_KEY_FILE directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			file, err = os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_FILE: %w", err)
			}
			defer file.Close()
			return readInstallationEncryptionKey(file)
		}
		if err != nil {
			return nil, fmt.Errorf("create SECRET_ENCRYPTION_KEY_FILE: %w", err)
		}
		if _, err := file.WriteString(base64.StdEncoding.EncodeToString(key) + "\n"); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return nil, fmt.Errorf("write SECRET_ENCRYPTION_KEY_FILE: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return nil, fmt.Errorf("close SECRET_ENCRYPTION_KEY_FILE: %w", err)
		}
		fmt.Fprintln(os.Stderr, installationEncryptionKeyWarning)
		return key, nil
	}
	defer file.Close()
	return readInstallationEncryptionKey(file)
}

func readInstallationEncryptionKey(file *os.File) ([]byte, error) {
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

func envInt64Default(name string, fallback int64) (int64, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
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
