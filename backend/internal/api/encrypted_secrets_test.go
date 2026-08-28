package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type memoryEncryptedSecretRepository struct {
	mu      sync.Mutex
	records map[string]store.EncryptedSecretRecord
}

func (r *memoryEncryptedSecretRepository) Upsert(_ context.Context, secret store.EncryptedSecretRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = map[string]store.EncryptedSecretRecord{}
	}
	secret.IntegrityStatus = store.SecretIntegrityUnknown
	secret.EncryptedValue = append([]byte(nil), secret.EncryptedValue...)
	secret.CreatedAt, secret.UpdatedAt = time.Now(), time.Now()
	secret.LastValidatedAt = nil
	r.records[secret.ID] = secret
	return nil
}

func (r *memoryEncryptedSecretRepository) Find(_ context.Context, id string) (store.EncryptedSecretRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	secret, ok := r.records[id]
	secret.EncryptedValue = append([]byte(nil), secret.EncryptedValue...)
	return secret, ok, nil
}

func (r *memoryEncryptedSecretRepository) SetIntegrityStatus(_ context.Context, id, status string, validatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	secret, ok := r.records[id]
	if !ok {
		return errors.New("secret not found")
	}
	secret.IntegrityStatus, secret.LastValidatedAt, secret.UpdatedAt = status, &validatedAt, validatedAt
	r.records[id] = secret
	return nil
}

func (r *memoryEncryptedSecretRepository) ListStatuses(_ context.Context) ([]store.EncryptedSecretStatusRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	statuses := make([]store.EncryptedSecretStatusRecord, 0, len(r.records))
	for _, secret := range r.records {
		statuses = append(statuses, store.EncryptedSecretStatusRecord{ID: secret.ID, Name: secret.Name, IntegrityStatus: secret.IntegrityStatus, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt, LastValidatedAt: secret.LastValidatedAt})
	}
	return statuses, nil
}

func TestOIDCClientSecretValidatesAuthenticatedEncryption(t *testing.T) {
	repository := &memoryEncryptedSecretRepository{}
	service := NewOIDCService()
	service.SetSecretRepository(repository, []byte("01234567890123456789012345678901"))
	if err := service.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", ClientID: "client", ClientSecret: "client-secret", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	record := repository.records[oidcSecretID("corp")]
	if record.IntegrityStatus != store.SecretIntegrityUnknown || len(record.EncryptedValue) == 0 || strings.Contains(string(record.EncryptedValue), "client-secret") {
		t.Fatalf("stored secret = %#v", record)
	}
	if secret, configured, err := service.clientSecret("corp"); err != nil || !configured || secret != "client-secret" {
		t.Fatalf("decrypted secret = %q, configured = %t, err = %v", secret, configured, err)
	}
	if record := repository.records[oidcSecretID("corp")]; record.IntegrityStatus != store.SecretIntegrityValid || record.LastValidatedAt == nil {
		t.Fatalf("valid secret status = %#v", record)
	}

	repository.records[oidcSecretID("corp")].EncryptedValue[len(record.EncryptedValue)-1] ^= 1
	if _, _, err := service.clientSecret("corp"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("tampered secret error = %v", err)
	}
	if record := repository.records[oidcSecretID("corp")]; record.IntegrityStatus != store.SecretIntegrityFailed || record.LastValidatedAt == nil {
		t.Fatalf("tampered secret status = %#v", record)
	}
}

func TestOIDCSecretAttentionOmitsSecretMaterial(t *testing.T) {
	repository := &memoryEncryptedSecretRepository{}
	service := NewOIDCService()
	service.SetSecretRepository(repository, []byte("01234567890123456789012345678901"))
	if err := service.AddProvider(OIDCProvider{Key: "corp", Name: "Microsoft SSO", Issuer: "https://issuer.example", ClientSecret: "client-secret", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	attention, err := service.SecretAttention()
	if err != nil || len(attention) != 1 || attention[0].Name != "Microsoft SSO" || attention[0].Status != store.SecretIntegrityUnknown {
		t.Fatalf("attention = %#v, err = %v", attention, err)
	}
	encoded, _ := json.Marshal(attention)
	if strings.Contains(string(encoded), "client-secret") || strings.Contains(string(encoded), "encryptedValue") {
		t.Fatalf("attention response exposed secret material: %s", encoded)
	}
}

func TestSecretAttentionEndpointRequiresSecretsReadAndReturnsStatusOnly(t *testing.T) {
	repository := &memoryEncryptedSecretRepository{}
	service := NewOIDCService()
	service.SetSecretRepository(repository, []byte("01234567890123456789012345678901"))
	if err := service.AddProvider(OIDCProvider{Key: "corp", Name: "Microsoft SSO", Issuer: "https://issuer.example", ClientSecret: "client-secret", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := Server{OIDC: service, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"secrets.read": true}}, true
	}}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/secrets/attention", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Microsoft SSO") || strings.Contains(response.Body.String(), "client-secret") {
		t.Fatalf("secret attention response = %d %s", response.Code, response.Body.String())
	}
}

func TestSecretAdminAPIStoresAndListsNamedMetadata(t *testing.T) {
	repository := &memoryEncryptedSecretRepository{}
	key := []byte("01234567890123456789012345678901")
	server := Server{Secrets: NewSecretAdminService(repository, key), Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"secrets.read": true, "secrets.manage": true}}, true
	}}
	create := httptest.NewRecorder()
	server.Handler().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/admin/secrets", bytes.NewBufferString(`{"name":"GitHub Integration","secret_value":"runtime-secret"}`)))
	if create.Code != http.StatusCreated || strings.Contains(create.Body.String(), "runtime-secret") {
		t.Fatalf("secret create response = %d %s", create.Code, create.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created secret = %s, err = %v", create.Body.String(), err)
	}
	record := repository.records[created.ID]
	if strings.Contains(string(record.EncryptedValue), "runtime-secret") {
		t.Fatal("repository stored plaintext secret")
	}
	if value, err := platform.DecryptSecret(key, record.EncryptedValue); err != nil || value != "runtime-secret" {
		t.Fatalf("stored secret = %q, err = %v", value, err)
	}
	list := httptest.NewRecorder()
	server.Handler().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/admin/secrets", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "GitHub Integration") || strings.Contains(list.Body.String(), "runtime-secret") || strings.Contains(list.Body.String(), "encryptedValue") {
		t.Fatalf("secret list response = %d %s", list.Code, list.Body.String())
	}
	attention := httptest.NewRecorder()
	server.Handler().ServeHTTP(attention, httptest.NewRequest(http.MethodGet, "/api/v1/admin/secrets/attention", nil))
	if attention.Code != http.StatusOK || !strings.Contains(attention.Body.String(), "GitHub Integration") {
		t.Fatalf("secret attention response = %d %s", attention.Code, attention.Body.String())
	}
}

func TestOIDCExchangeUsesEncryptedClientSecret(t *testing.T) {
	repository := &memoryEncryptedSecretRepository{}
	service := NewOIDCService()
	service.SetSecretRepository(repository, []byte("01234567890123456789012345678901"))
	provider := OIDCProvider{Key: "corp", Issuer: "https://issuer.example", ClientID: "client", ClientSecret: "client-secret", Callback: "https://app.example/callback", Enabled: true}
	if err := service.AddProvider(provider); err != nil {
		t.Fatal(err)
	}
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]string{"id_token": "token"})
		if request.FormValue("client_secret") != "client-secret" {
			return nil, errors.New("client secret was not decrypted for exchange")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": []string{"application/json"}}, Request: request}, nil
	})})
	if _, err := service.exchangeCode(provider, "https://issuer.example/token", "code", provider.Callback, "verifier"); err != nil {
		t.Fatal(err)
	}
}
