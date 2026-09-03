package config

import "testing"

func TestConfigHelperEdges(t *testing.T) {
	if got := normalizeDatabaseMode("psql", ""); got != "postgresql" {
		t.Fatalf("database mode = %q", got)
	}
	if got := normalizeDatabaseMode("invalid", ""); got != "" {
		t.Fatalf("invalid database mode = %q", got)
	}
	if got := normalizeNATSMode("embed", ""); got != "embedded" {
		t.Fatalf("NATS mode = %q", got)
	}
	if err := (Config{DatabaseURL: "https://not-sqlite"}).validateDatabase("sqlite"); err == nil {
		t.Fatal("non-file SQLite URL accepted")
	}
	if err := (Config{DatabaseURL: "postgres://db/app?sslmode=verify-full"}).validateDatabase("postgresql"); err != nil {
		t.Fatal(err)
	}
	if databaseSSLMode("not a URL") != "" {
		t.Fatal("invalid database URL returned an SSL mode")
	}
	if _, err := loadInstallationEncryptionKey(""); err == nil {
		t.Fatal("empty encryption-key path accepted")
	}
	t.Setenv("GLYPHFLOW_TEST_INT", "bad")
	if _, err := envInt("GLYPHFLOW_TEST_INT"); err == nil {
		t.Fatal("invalid integer accepted")
	}
	t.Setenv("GLYPHFLOW_TEST_INT", "12")
	if value, err := envInt("GLYPHFLOW_TEST_INT"); err != nil || value != 12 {
		t.Fatalf("integer = %d, %v", value, err)
	}
	t.Setenv("GLYPHFLOW_TEST_STRING", "value")
	if envStringDefault("GLYPHFLOW_TEST_STRING", "fallback") != "value" || envStringDefault("GLYPHFLOW_MISSING", "fallback") != "fallback" {
		t.Fatal("string environment helper returned an unexpected value")
	}
	t.Setenv("GLYPHFLOW_TEST_INT64", "bad")
	if _, err := envInt64Default("GLYPHFLOW_TEST_INT64", 1); err == nil {
		t.Fatal("invalid int64 accepted")
	}
}
