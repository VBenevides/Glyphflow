package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type SecretMetadata struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	LastValidatedAt *time.Time        `json:"lastValidatedAt,omitempty"`
	Tasks           []SecretTaskUsage `json:"tasks"`
	CanDelete       bool              `json:"canDelete"`
}

type SecretTaskUsage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SecretAdminService struct {
	repository store.EncryptedSecretRepository
	key        []byte
}

func NewSecretAdminService(repository store.EncryptedSecretRepository, key []byte) *SecretAdminService {
	return &SecretAdminService{repository: repository, key: append([]byte(nil), key...)}
}

func validateStoredSecret(ctx context.Context, repository store.EncryptedSecretRepository, key []byte, id string) error {
	record, found, err := repository.Find(ctx, id)
	if err != nil || !found {
		return errors.New("secret validation failed")
	}
	if _, err := platform.DecryptSecret(key, record.EncryptedValue); err != nil {
		status := store.SecretIntegrityFailed
		if errors.Is(err, platform.ErrSecretDecryption) {
			status = store.SecretIntegrityDecryptionFailed
		}
		_ = repository.SetIntegrityStatus(ctx, id, status, time.Now().UTC())
		return errors.New("secret validation failed")
	}
	if err := repository.SetIntegrityStatus(ctx, id, store.SecretIntegrityValid, time.Now().UTC()); err != nil {
		return errors.New("secret validation failed")
	}
	return nil
}

func (s *SecretAdminService) hasDurableRepository() bool { return s != nil && s.repository != nil }

func (s *SecretAdminService) List(ctx context.Context) ([]SecretMetadata, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("secret storage is unavailable")
	}
	statuses, err := s.repository.ListStatuses(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]SecretMetadata, 0, len(statuses))
	for _, status := range statuses {
		name := strings.TrimSpace(status.Name)
		if name == "" {
			name = status.ID
		}
		tasks := make([]SecretTaskUsage, len(status.Tasks))
		for index, task := range status.Tasks {
			tasks[index] = SecretTaskUsage{ID: task.ID, Name: task.Name}
		}
		items = append(items, SecretMetadata{ID: status.ID, Name: name, Status: status.IntegrityStatus, CreatedAt: status.CreatedAt, UpdatedAt: status.UpdatedAt, LastValidatedAt: status.LastValidatedAt, Tasks: tasks, CanDelete: status.CanDelete})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) || strings.ToLower(items[i].Name) == strings.ToLower(items[j].Name) && items[i].ID < items[j].ID
	})
	return items, nil
}

func (s *SecretAdminService) Upsert(ctx context.Context, id, name, value string) error {
	if s == nil || s.repository == nil || len(s.key) != 32 {
		return errors.New("secret storage is unavailable")
	}
	name = strings.TrimSpace(name)
	if id == "" || name == "" || value == "" {
		return errors.New("secret name and value are required")
	}
	encrypted, err := platform.EncryptSecret(s.key, value)
	if err != nil {
		return err
	}
	if err := s.repository.Upsert(ctx, store.EncryptedSecretRecord{ID: id, Name: name, EncryptedValue: encrypted}); err != nil {
		return err
	}
	return validateStoredSecret(ctx, s.repository, s.key, id)
}

func (s *SecretAdminService) Delete(ctx context.Context, id string) error {
	if s == nil || s.repository == nil {
		return errors.New("secret storage is unavailable")
	}
	if id == "" {
		return errors.New("secret id is required")
	}
	return s.repository.Delete(ctx, id)
}

type secretInput struct {
	Name  string `json:"name"`
	Value string `json:"secret_value"`
}

func (s Server) secretRoutes(mux routeRegistrar) {
	mux.Handle("/api/v1/admin/secrets", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "secrets.read|secrets.manage"
		}
		return "secrets.manage"
	}, http.HandlerFunc(s.secretCollection)))
	mux.Handle("/api/v1/admin/secrets/", s.require("secrets.manage", http.HandlerFunc(s.secretPath)))
}

func (s Server) secretCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.Secrets.List(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "secret list unavailable", err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var input secretInput
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret request", err)
			return
		}
		id, err := randomID()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "secret creation failed", err)
			return
		}
		id = "secret-" + id
		if err := s.Secrets.Upsert(r.Context(), id, input.Name, input.Value); err != nil {
			writeError(w, http.StatusBadRequest, "secret creation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id, "name": strings.TrimSpace(input.Name)})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s Server) secretPath(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/secrets/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorSecretNotFound})
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.Secrets.Delete(r.Context(), id); err != nil {
			switch {
			case errors.Is(err, store.ErrEncryptedSecretInUse):
				writeJSON(w, http.StatusConflict, map[string]string{"error": "secret is still in use"})
			case errors.Is(err, store.ErrEncryptedSecretNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": errorSecretNotFound})
			default:
				writeError(w, http.StatusServiceUnavailable, "secret deletion unavailable", err)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorSecretNotFound})
		return
	}
	var input secretInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid secret request", err)
		return
	}
	if err := s.Secrets.Upsert(r.Context(), id, input.Name, input.Value); err != nil {
		writeError(w, http.StatusBadRequest, "secret update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "name": strings.TrimSpace(input.Name)})
}

func (s Server) secretAttention(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.Secrets != nil && s.Secrets.hasDurableRepository() {
		items, err := s.Secrets.List(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, errorSecretStatusUnavailable, err)
			return
		}
		attention := make([]SecretStatusView, 0, len(items))
		for _, item := range items {
			if item.Status != store.SecretIntegrityValid {
				attention = append(attention, SecretStatusView{ID: item.ID, Name: item.Name, Status: item.Status})
			}
		}
		writeJSON(w, http.StatusOK, attention)
		return
	}
	if s.OIDC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errorSecretStatusUnavailable})
		return
	}
	secrets, err := s.OIDC.SecretAttention()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errorSecretStatusUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, secrets)
}
