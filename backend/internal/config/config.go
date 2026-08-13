package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

type Role string

const (
	ControlPlane Role = "control-plane"
	Worker       Role = "worker"
)

type Config struct {
	Role            Role
	DatabaseURL     string
	NATSURL         string
	DataDir         string
	RunnerID        string
	MaxMessageBytes int
	MaxOutputBytes  int
}

func FromEnv(role Role) (Config, error) {
	config := Config{
		Role:        role,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		NATSURL:     os.Getenv("NATS_URL"),
		DataDir:     os.Getenv("DATA_DIR"),
		RunnerID:    os.Getenv("RUNNER_ID"),
	}
	var err error
	if config.MaxMessageBytes, err = envInt("MAX_MESSAGE_BYTES"); err != nil {
		return Config{}, err
	}
	if role == Worker {
		if config.MaxOutputBytes, err = envInt("MAX_OUTPUT_BYTES"); err != nil {
			return Config{}, err
		}
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.Role != ControlPlane && c.Role != Worker {
		return fmt.Errorf("role must be %q or %q", ControlPlane, Worker)
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
	if c.Role == ControlPlane {
		return requireURL("DATABASE_URL", c.DatabaseURL, "postgres", "postgresql")
	}
	if !runnerIDPattern.MatchString(c.RunnerID) {
		return errors.New("RUNNER_ID must contain only letters, digits, dot, underscore, or hyphen")
	}
	if c.MaxOutputBytes <= 0 || c.MaxOutputBytes > c.MaxMessageBytes {
		return errors.New("MAX_OUTPUT_BYTES must be greater than zero and no larger than MAX_MESSAGE_BYTES")
	}
	return nil
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
