package config

import "testing"

func TestConfigHelperEdges(t *testing.T) {
	testConfigModeAndDatabaseHelpers(t)
	testConfigEnvironmentHelpers(t)
}

func testConfigModeAndDatabaseHelpers(t *testing.T) {
	if got := normalizeDatabaseMode("psql", ""); got != "postgresql" {
		t.Fatalf("database mode = %q", got)
	}
	if got := normalizeDatabaseMode("invalid", ""); got != "" {
		t.Fatalf("invalid database mode = %q", got)
	}
	if normalizeDatabaseMode("", "postgres://db/app") != "postgresql" || normalizeDatabaseMode("", "file://db.sqlite") != "sqlite" {
		t.Fatal("database mode inference failed")
	}
	if got := normalizeNATSMode("embed", ""); got != "embedded" {
		t.Fatalf("NATS mode = %q", got)
	}
	if normalizeNATSMode("", "nats://localhost") != "remote" || normalizeNATSMode("", "") != "embedded" || normalizeNATSMode("invalid", "") != "" {
		t.Fatal("NATS mode inference failed")
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
}

func testConfigEnvironmentHelpers(t *testing.T) {
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
	if value, err := envInt64Default("GLYPHFLOW_MISSING_INT64", 7); err != nil || value != 7 {
		t.Fatalf("int64 fallback = %d, %v", value, err)
	}
	t.Setenv("GLYPHFLOW_TEST_BOOL", "true")
	if value, err := envBool("GLYPHFLOW_TEST_BOOL", false); err != nil || !value {
		t.Fatalf("boolean = %v, %v", value, err)
	}
	t.Setenv("GLYPHFLOW_BAD_BOOL", "not-bool")
	if _, err := envBool("GLYPHFLOW_BAD_BOOL", false); err == nil {
		t.Fatal("invalid boolean accepted")
	}
	if value, err := envBool("GLYPHFLOW_MISSING_BOOL", true); err != nil || !value {
		t.Fatalf("boolean fallback = %v, %v", value, err)
	}
	if databaseSSLMode("postgres://db/app?sslmode=verify-full") != "verify-full" {
		t.Fatal("database SSL mode was not parsed")
	}
}
