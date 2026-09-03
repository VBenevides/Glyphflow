package store

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestStoreValueHelpers(t *testing.T) {
	for _, test := range []struct {
		email, want string
	}{
		{"ada.lovelace@example.com", "Ada Lovelace"},
		{"  --  @example.com", "User"},
	} {
		if got := DefaultUserDisplayName(test.email); got != test.want {
			t.Errorf("DefaultUserDisplayName(%q) = %q, want %q", test.email, got, test.want)
		}
	}
	for _, test := range []struct {
		status string
		want   bool
	}{
		{StatusActive, true}, {" PENDING ", true}, {"unknown", false},
	} {
		if got := ValidUserStatus(test.status); got != test.want {
			t.Errorf("ValidUserStatus(%q) = %v, want %v", test.status, got, test.want)
		}
	}
	for _, test := range []struct {
		user UserRecord
		want string
	}{
		{UserRecord{Enabled: true}, StatusActive},
		{UserRecord{}, StatusDisabled},
		{UserRecord{Status: " PENDING "}, StatusPending},
	} {
		if got, err := userStatus(test.user); err != nil || got != test.want {
			t.Errorf("userStatus(%+v) = %q, %v; want %q", test.user, got, err, test.want)
		}
	}
	if _, err := userStatus(UserRecord{Status: "invalid"}); err == nil {
		t.Fatal("userStatus accepted an invalid status")
	}

	for _, test := range []struct {
		event        RunEventInput
		initial, max int
		multiplier   float64
		want         float64
	}{
		{RunEventInput{Attempt: 1}, 5, 0, 0.5, 5},
		{RunEventInput{Attempt: 4}, 5, 20, 2, 20},
		{RunEventInput{Attempt: 2}, 15, 20, 2, 20},
	} {
		if got := retryDelaySeconds(test.event, test.initial, test.max, test.multiplier); got != test.want {
			t.Errorf("retryDelaySeconds(%+v) = %v, want %v", test.event, got, test.want)
		}
	}
	if got := maxInt(4, 9); got != 9 {
		t.Fatalf("maxInt = %d, want 9", got)
	}

	if got, err := decodeEnvironment([]byte(`{"TEXT":"value","NUMBER":42}`)); err != nil || got["TEXT"] != "value" || got["NUMBER"] != "42" {
		t.Fatalf("decodeEnvironment = %#v, %v", got, err)
	}
	if got, err := decodeSecretReferences([]byte(`null`)); err != nil || got == nil {
		t.Fatalf("decodeSecretReferences = %#v, %v", got, err)
	}
	if !sameSecretReferences(map[string]string{"A": "one"}, map[string]string{"A": "one"}) {
		t.Fatal("sameSecretReferences rejected equal maps")
	}
	if sameSecretReferences(map[string]string{"A": "one"}, map[string]string{"A": "two"}) {
		t.Fatal("sameSecretReferences accepted different maps")
	}
}

func TestSQLiteAdapterHelpers(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	var text string
	var flag bool
	var number int
	var decimal float64
	var timestamp time.Time
	var object map[string]string
	var raw json.RawMessage
	var bytes []byte
	row := sqliteDatabase{db: db}.QueryRow(ctx, `SELECT 'hello', 1, 42, 2.5, '2026-09-03T12:00:00Z', '{"name":"glyphflow"}', '{"ok":true}', x'0102'`)
	if err := row.Scan(&text, &flag, &number, &decimal, &timestamp, &object, &raw, &bytes); err != nil {
		t.Fatal(err)
	}
	if text != "hello" || !flag || number != 42 || decimal != 2.5 || object["name"] != "glyphflow" || string(raw) != `{"ok":true}` || !reflect.DeepEqual(bytes, []byte{1, 2}) {
		t.Fatalf("SQLite values = %q, %v, %d, %v, %s, %#v, %s, %#v", text, flag, number, decimal, timestamp, object, raw, bytes)
	}

	var empty string
	if err := (sqliteDatabase{db: db}).QueryRow(ctx, `SELECT NULL`).Scan(&empty); err != nil || empty != "" {
		t.Fatalf("SQLite NULL = %q, %v", empty, err)
	}
	var pointer *int
	if err := assignSQLite(&pointer, int64(7)); err != nil || pointer == nil || *pointer != 7 {
		t.Fatalf("pointer assignment = %v, %v", pointer, err)
	}
	if err := assignSQLite(7, int64(1)); err == nil {
		t.Fatal("assignSQLite accepted a non-pointer")
	}

	query, args, err := sqliteQuery(`SELECT id FROM items WHERE id = ANY($1) AND value = $2::text FOR UPDATE`, []any{[]string{"a", "b"}, time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if query != "SELECT id FROM items WHERE id IN (?,?) AND value = ? " {
		t.Fatalf("sqliteQuery = %q", query)
	}
	if len(args) != 3 || args[0] != "a" || args[1] != "b" || args[2] != "2026-09-03 12:00:00" {
		t.Fatalf("sqliteQuery args = %#v", args)
	}

	decoded := []any{"00ff"}
	if err := decodeSQLiteArguments(decoded, "decode($1, 'hex')"); err != nil || !reflect.DeepEqual(decoded[0], []byte{0, 255}) {
		t.Fatalf("decodeSQLiteArguments = %#v, %v", decoded, err)
	}
	if _, _, err := bindSQLitePlaceholders("$2", []any{"one"}); err == nil {
		t.Fatal("bindSQLitePlaceholders accepted an invalid index")
	}
	if err := decodeSQLiteArguments([]any{"not-hex"}, "decode($1, 'hex')"); err == nil {
		t.Fatal("decodeSQLiteArguments accepted invalid hex")
	}
	if got := sqliteSlice([]any{[]string{"a", "b"}}, "1"); len(got) != 2 || got[1] != "b" {
		t.Fatalf("sqliteSlice = %#v", got)
	}
	if got := sqliteSlice([]any{"scalar"}, "1"); got != nil {
		t.Fatalf("sqliteSlice scalar = %#v, want nil", got)
	}
	if _, err := sqliteTime("invalid"); err == nil {
		t.Fatal("sqliteTime accepted invalid input")
	}
}

func TestSQLiteMigrationAndSSOHelpers(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	if err := ApplySQLiteMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	states := NewOIDCAuthorizationStateRepository(db)
	expires := time.Now().UTC().Add(time.Hour)
	if err := states.Create(ctx, OIDCAuthorizationStateRecord{ID: "state-any", ProviderID: "provider", StateHash: "state-hash", NonceHash: "nonce", EncryptedPKCEVerifier: []byte("verifier"), Purpose: "login", Callback: "https://app/callback", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	if got, err := states.ConsumeAny(ctx, "state-hash", "nonce", "provider", "https://app/callback", time.Now().UTC()); err != nil || got.Purpose != "login" {
		t.Fatalf("ConsumeAny = %#v, %v", got, err)
	}

	roles := NewRoleRepository(db)
	if err := roles.Ensure(ctx, "role-default", "Default", "", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := roles.Ensure(ctx, "role-admin", "Admin", "", false, nil); err != nil {
		t.Fatal(err)
	}
	user := UserRecord{ID: "oidc-user", Username: "oidc", Email: "oidc@example.com", Enabled: true}
	identity := SSOIdentityRecord{ID: "oidc-identity", UserID: user.ID, ProviderID: "provider", Subject: "subject"}
	if err := NewOIDCProviderRepository(db).ProvisionOIDC(ctx, user, "role-default", "role-admin", identity); err != nil {
		t.Fatal(err)
	}
	if _, found, err := NewUserRepository(db).FindByID(ctx, user.ID); err != nil || !found {
		t.Fatalf("provisioned user found=%v, err=%v", found, err)
	}
}
