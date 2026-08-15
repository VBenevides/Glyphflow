package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

type RunnerRecord struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	PoolID          string `json:"poolId,omitempty"`
	DesiredState    string `json:"desiredState"`
	ObservedState   string `json:"observedState"`
	Pool            string `json:"pool"`
	Capacity        int    `json:"capacity"`
	CurrentCapacity int    `json:"currentCapacity,omitempty"`
	ActiveCount     int    `json:"activeCount"`
	HeartbeatAt     string `json:"heartbeatAt,omitempty"`
	Platform        string `json:"platform,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
}
type RunnerPoolRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}
type ResourceRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Enabled          bool   `json:"enabled"`
	Holder           string `json:"holder,omitempty"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	FencingToken     int64  `json:"fencingToken"`
	ActiveReferences int    `json:"activeReferences,omitempty"`
}
type enrollment struct {
	Token    string
	RunnerID string
	Expires  time.Time
	Used     bool
}
type runnerKey struct {
	ID     string
	Public ed25519.PublicKey
}
type InfrastructureService struct {
	mu                       sync.RWMutex
	runners                  map[string]RunnerRecord
	pools                    map[string]RunnerPoolRecord
	resources                map[string]ResourceRecord
	enrollments              map[string]*enrollment
	runnerKeys               map[string]runnerKey
	next                     int
	runnerRepository         store.RunnerRepository
	resourceRepository       store.ResourceRepository
	runnerBinaryDir          string
	runnerNATSURL            string
	runnerMaxMessageBytes    int
	controlPlanePublicKey    string
	runnerCapacityPublisher  queue.Publisher
	runnerCapacitySigningKey protocol.SigningKey
}

func NewInfrastructureService() *InfrastructureService {
	return &InfrastructureService{runners: map[string]RunnerRecord{}, pools: map[string]RunnerPoolRecord{"default": {ID: "default", Name: "default", Enabled: true}}, resources: map[string]ResourceRecord{}, enrollments: map[string]*enrollment{}, runnerKeys: map[string]runnerKey{}, runnerBinaryDir: "runner-binaries"}
}

func (s *InfrastructureService) SetRunnerRepository(repository store.RunnerRepository) {
	if repository != nil {
		s.mu.Lock()
		s.runnerRepository = repository
		s.mu.Unlock()
	}
}

func (s *InfrastructureService) SetResourceRepository(repository store.ResourceRepository) {
	if repository != nil {
		s.mu.Lock()
		s.resourceRepository = repository
		s.mu.Unlock()
	}
}

func (s *InfrastructureService) SetRunnerBinaryDirectory(directory string) {
	if strings.TrimSpace(directory) == "" {
		return
	}
	s.mu.Lock()
	s.runnerBinaryDir = directory
	s.mu.Unlock()
}

func (s *InfrastructureService) SetRunnerArtifactConfig(natsURL string, maxMessageBytes int) {
	s.mu.Lock()
	s.runnerNATSURL = strings.TrimSpace(natsURL)
	s.runnerMaxMessageBytes = maxMessageBytes
	s.mu.Unlock()
}

func (s *InfrastructureService) SetControlPlanePublicKey(publicKey string) {
	s.mu.Lock()
	s.controlPlanePublicKey = strings.TrimSpace(publicKey)
	s.mu.Unlock()
}

func (s *InfrastructureService) SetRunnerCapacityPublisher(publisher queue.Publisher, signingKey protocol.SigningKey) {
	s.mu.Lock()
	s.runnerCapacityPublisher = publisher
	s.runnerCapacitySigningKey = signingKey
	s.mu.Unlock()
}

func runnerRecordFromStore(runner store.RunnerRecord) RunnerRecord {
	heartbeat := ""
	if runner.HeartbeatAt != nil {
		heartbeat = runner.HeartbeatAt.UTC().Format(time.RFC3339)
	}
	return RunnerRecord{ID: runner.ID, Name: runner.Name, PoolID: runner.PoolID, DesiredState: runner.DesiredState, ObservedState: runner.ObservedState, Pool: runner.Pool, Capacity: runner.Capacity, CurrentCapacity: runner.CurrentCapacity, ActiveCount: runner.ActiveCount, HeartbeatAt: heartbeat, Platform: runner.Platform, Architecture: runner.Architecture}
}

func runnerPoolRecordFromStore(pool store.RunnerPoolRecord) RunnerPoolRecord {
	return RunnerPoolRecord{ID: pool.ID, Name: pool.Name, Description: pool.Description, Enabled: pool.Enabled}
}

func (s *InfrastructureService) poolCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		repository := s.runnerRepository
		s.mu.RUnlock()
		if repository != nil {
			items, err := repository.ListPools(r.Context())
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "runner pool storage unavailable", err)
				return
			}
			result := make([]RunnerPoolRecord, 0, len(items))
			for _, item := range items {
				result = append(result, runnerPoolRecordFromStore(item))
			}
			writePage(w, r, result)
			return
		}
		s.mu.RLock()
		items := make([]RunnerPoolRecord, 0, len(s.pools))
		for _, item := range s.pools {
			items = append(items, item)
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
		writePage(w, r, items)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner pool name is required"})
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	id, err := randomID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "runner pool creation failed", err)
		return
	}
	item := RunnerPoolRecord{ID: "pool-" + id, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Enabled: enabled}
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		if err := repository.CreatePool(r.Context(), store.RunnerPoolRecord{ID: item.ID, Name: item.Name, Description: item.Description, Enabled: item.Enabled}); err != nil {
			writeError(w, http.StatusConflict, "runner pool creation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
		return
	}
	s.mu.Lock()
	s.pools[item.ID] = item
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, item)
}

func (s *InfrastructureService) poolPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 5 || parts[4] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner pool not found"})
		return
	}
	id := parts[4]
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if r.Method == http.MethodGet {
		if repository != nil {
			item, found, err := repository.FindPool(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "runner pool storage unavailable", err)
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner pool not found"})
			} else {
				writeJSON(w, http.StatusOK, runnerPoolRecordFromStore(item))
			}
			return
		}
		s.mu.RLock()
		item, found := s.pools[id]
		s.mu.RUnlock()
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner pool not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if r.Method == http.MethodDelete {
		if repository != nil {
			if err := repository.DeletePool(r.Context(), id); err != nil {
				if errors.Is(err, store.ErrRunnerPoolInUse) {
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
					return
				}
				writeError(w, http.StatusConflict, "runner pool deletion failed", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mu.Lock()
		if _, found := s.pools[id]; !found {
			s.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner pool not found"})
			return
		}
		delete(s.pools, id)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner pool name is required"})
		return
	}
	item := RunnerPoolRecord{ID: id, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Enabled: input.Enabled}
	if repository != nil {
		updated, found, err := repository.UpdatePool(r.Context(), store.RunnerPoolRecord{ID: item.ID, Name: item.Name, Description: item.Description, Enabled: item.Enabled})
		if err != nil {
			writeError(w, http.StatusConflict, "runner pool update failed", err)
		} else if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner pool not found"})
		} else {
			writeJSON(w, http.StatusOK, runnerPoolRecordFromStore(updated))
		}
		return
	}
	s.mu.Lock()
	if _, found := s.pools[id]; !found {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner pool not found"})
		return
	}
	s.pools[id] = item
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, item)
}

func resourceRecordFromStore(resource store.ResourceRecord) ResourceRecord {
	expiresAt := ""
	if resource.ExpiresAt != nil {
		expiresAt = resource.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return ResourceRecord{ID: resource.ID, Name: resource.Name, Enabled: resource.Enabled, Holder: resource.Holder, ExpiresAt: expiresAt, FencingToken: resource.FencingToken}
}

func (s *InfrastructureService) runnerCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		items, err := repository.List(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "runner storage unavailable", err)
			return
		}
		result := make([]RunnerRecord, 0, len(items))
		for _, item := range items {
			result = append(result, runnerRecordFromStore(item))
		}
		result = filterRunners(result, r.URL.Query().Get("state"), r.URL.Query().Get("search"))
		sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
		writePage(w, r, result)
		return
	}
	s.mu.RLock()
	items := make([]RunnerRecord, 0, len(s.runners))
	for _, item := range s.runners {
		items = append(items, item)
	}
	s.mu.RUnlock()
	items = filterRunners(items, r.URL.Query().Get("state"), r.URL.Query().Get("search"))
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writePage(w, r, items)
}

func filterRunners(items []RunnerRecord, state string, searchValues ...string) []RunnerRecord {
	state = strings.TrimSpace(state)
	search := ""
	if len(searchValues) > 0 {
		search = searchValues[0]
	}
	search = strings.ToLower(strings.TrimSpace(search))
	if state == "" && search == "" {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		matchesSearch := search == "" || strings.Contains(strings.ToLower(item.ID), search) || strings.Contains(strings.ToLower(item.Name), search) || strings.Contains(strings.ToLower(item.Pool), search)
		if (state == "" || strings.EqualFold(item.ObservedState, state)) && matchesSearch {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
func (s *InfrastructureService) runnerPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	if len(parts) == 4 && r.Method == http.MethodPost && parts[3] == "enrollments" {
		s.enroll(w, r)
		return
	}
	if len(parts) == 4 && r.Method == http.MethodDelete {
		s.deleteRunner(w, r, parts[3])
		return
	}
	if len(parts) == 4 && r.Method == http.MethodGet {
		s.mu.RLock()
		repository := s.runnerRepository
		s.mu.RUnlock()
		if repository != nil {
			item, found, err := repository.Find(r.Context(), parts[3])
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "runner storage unavailable", err)
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
			} else {
				writeJSON(w, http.StatusOK, runnerRecordFromStore(item))
			}
			return
		}
		s.mu.RLock()
		item, ok := s.runners[parts[3]]
		s.mu.RUnlock()
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "runner not found"})
			return
		}
		writeJSON(w, 200, item)
		return
	}
	if len(parts) == 4 && r.Method == http.MethodPut {
		var input struct {
			Capacity int `json:"capacity"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Capacity < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "capacity must be at least 1"})
			return
		}
		s.mu.RLock()
		repository, publisher, signingKey := s.runnerRepository, s.runnerCapacityPublisher, s.runnerCapacitySigningKey
		s.mu.RUnlock()
		var item RunnerRecord
		if repository != nil {
			updated, found, err := repository.UpdateCapacity(r.Context(), parts[3], input.Capacity)
			if err != nil {
				writeError(w, http.StatusConflict, "runner capacity update failed", err)
				return
			}
			if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
				return
			}
			item = runnerRecordFromStore(updated)
		} else {
			s.mu.Lock()
			var found bool
			item, found = s.runners[parts[3]]
			if found {
				item.Capacity = input.Capacity
				s.runners[parts[3]] = item
			}
			s.mu.Unlock()
			if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
				return
			}
		}
		if publisher != nil {
			var err error
			if len(signingKey.Private) != ed25519.PrivateKeySize {
				err = errors.New("runner capacity signing key is unavailable")
			} else {
				payload, encodeErr := protocol.EncodeRunnerControlPayload(protocol.RunnerControlPayload{Version: protocol.ProtocolVersion, Type: protocol.RunnerControlCapacity, RunnerID: parts[3], Capacity: input.Capacity, IssuedAt: time.Now().UTC()})
				err = encodeErr
				if err == nil {
					envelope, signErr := signingKey.SignEvent(payload)
					err = signErr
					if err == nil {
						raw, encodeErr := protocol.EncodeEnvelope(envelope)
						err = encodeErr
						if err == nil {
							err = publisher.Publish(r.Context(), queue.Message{Subject: queue.Subject("control", parts[3]), ID: "runner-capacity:" + parts[3] + ":" + strconv.FormatInt(time.Now().UnixNano(), 10), Data: raw})
						}
					}
				}
			}
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "runner capacity command failed", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if len(parts) == 5 && r.Method == http.MethodPost {
		s.mu.RLock()
		repository := s.runnerRepository
		s.mu.RUnlock()
		if repository != nil {
			states := map[string]string{"enable": "ENABLED", "disable": "DISABLED", "drain": "DRAINING", "reset": "ENABLED", "revoke": "REVOKED"}
			state, valid := states[parts[4]]
			if !valid {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner action not found"})
				return
			}
			item, found, err := repository.SetDesiredState(r.Context(), parts[3], state)
			if err != nil {
				writeError(w, http.StatusConflict, "runner state update failed", err)
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
			} else {
				writeJSON(w, http.StatusOK, runnerRecordFromStore(item))
			}
			return
		}
		s.mu.Lock()
		item, ok := s.runners[parts[3]]
		states := map[string]string{"enable": "ENABLED", "disable": "DISABLED", "drain": "DRAINING", "reset": "ENABLED", "revoke": "REVOKED"}
		state, valid := states[parts[4]]
		if ok && valid {
			item.DesiredState = state
			s.runners[parts[3]] = item
		}
		s.mu.Unlock()
		if !valid {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner action not found"})
		} else if !ok {
			writeJSON(w, 404, map[string]string{"error": "runner not found"})
		} else {
			writeJSON(w, 200, item)
		}
		return
	}
	writeJSON(w, 404, map[string]string{"error": "runner route not found"})
}

func (s *InfrastructureService) enrollRunner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		RunnerID  string `json:"runner_id"`
		Token     string `json:"token"`
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid runner enrollment request", err)
		return
	}
	if !runnerIDPattern.MatchString(input.RunnerID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_id must contain only letters, digits, dot, underscore, or hyphen"})
		return
	}
	if input.Token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "runner enrollment token is required"})
		return
	}
	publicKey, err := base64.RawStdEncoding.DecodeString(input.PublicKey)
	if err != nil || input.KeyID == "" || len(publicKey) != ed25519.PublicKeySize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner key_id and public_key are required"})
		return
	}
	s.mu.RLock()
	natsURL, maxMessageBytes := s.runnerNATSURL, s.runnerMaxMessageBytes
	s.mu.RUnlock()
	if natsURL == "" || maxMessageBytes <= 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner connection is not configured"})
		return
	}
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	var runner RunnerRecord
	if repository != nil {
		keyRepository, ok := repository.(interface {
			ConsumeEnrollmentWithKey(context.Context, string, time.Time, string, []byte) (store.RunnerRecord, error)
		})
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner enrollment key binding is unavailable"})
			return
		}
		stored, consumeErr := keyRepository.ConsumeEnrollmentWithKey(r.Context(), platform.HashToken(input.Token), time.Now().UTC(), input.KeyID, publicKey)
		if consumeErr != nil {
			writeError(w, http.StatusUnauthorized, "runner enrollment rejected", consumeErr)
			return
		}
		runner = runnerRecordFromStore(stored)
	} else {
		var consumeErr error
		runner, consumeErr = s.consumeEnrollmentWithKey(input.Token, time.Now().UTC(), input.KeyID, publicKey)
		if consumeErr != nil {
			writeError(w, http.StatusUnauthorized, "runner enrollment rejected", consumeErr)
			return
		}
	}
	if runner.ID != input.RunnerID {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "runner enrollment belongs to a different runner"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runner_id": runner.ID, "nats_url": natsURL, "max_message_bytes": maxMessageBytes, "capacity": runner.Capacity})
}

func (s *InfrastructureService) deleteRunner(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		deleted, err := repository.Delete(r.Context(), id)
		if err != nil {
			recordRequestError(r, err)
			writeError(w, http.StatusConflict, "runner deletion failed", err)
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mu.Lock()
	if _, ok := s.runners[id]; !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
		return
	}
	delete(s.runners, id)
	for token, item := range s.enrollments {
		if item.RunnerID == id {
			delete(s.enrollments, token)
		}
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
func (s *InfrastructureService) enroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		RunnerID     string `json:"runner_id"`
		PoolID       string `json:"pool_id"`
		Platform     string `json:"platform"`
		Architecture string `json:"architecture"`
		Capacity     *int   `json:"capacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid runner enrollment request", err)
		return
	}
	if strings.TrimSpace(input.RunnerID) == "" {
		writeJSON(w, 400, map[string]string{"error": "runner_id is required"})
		return
	}
	input.RunnerID = strings.TrimSpace(input.RunnerID)
	input.PoolID = strings.TrimSpace(input.PoolID)
	if input.PoolID == "" {
		input.PoolID = "default"
	}
	input.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
	input.Architecture = strings.ToLower(strings.TrimSpace(input.Architecture))
	if !runnerIDPattern.MatchString(input.RunnerID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_id must contain only letters, digits, dot, underscore, or hyphen"})
		return
	}
	if input.Platform != "linux" && input.Platform != "windows" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform must be linux or windows"})
		return
	}
	if input.Architecture != "amd64" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "architecture must be amd64"})
		return
	}
	if input.Capacity != nil && *input.Capacity < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "capacity must be at least 1"})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusInternalServerError, "enrollment failed while generating a credential", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiry := time.Now().Add(15 * time.Minute)
	artifact, filename, err := s.buildRunnerArtifact(r, input.Platform, input.Architecture, input.RunnerID, token)
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "runner binary is unavailable", err)
		return
	}
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		pool, found, err := repository.FindPool(r.Context(), input.PoolID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "runner pool lookup failed", err)
			return
		}
		if !found || !pool.Enabled {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "selected runner pool does not exist or is disabled"})
			return
		}
		requester := "system"
		if claims, ok := r.Context().Value(requestClaimsContextKey{}).(Claims); ok && claims.UserID != "" {
			requester = claims.UserID
		}
		capacity := 0
		if input.Capacity != nil {
			capacity = *input.Capacity
		}
		if err := repository.CreateEnrollment(r.Context(), store.RunnerRecord{ID: input.RunnerID, Name: input.RunnerID, PoolID: input.PoolID, Capacity: capacity, Platform: input.Platform, Architecture: input.Architecture}, store.RunnerEnrollmentRecord{ID: "enrollment-" + input.RunnerID + "-" + platform.HashToken(token), RunnerID: input.RunnerID, TokenHash: platform.HashToken(token), ExpiresAt: expiry, Requester: requester, Target: input.RunnerID, Artifact: map[string]any{"platform": input.Platform, "architecture": input.Architecture}}); err != nil {
			recordRequestError(r, err)
			writeError(w, http.StatusConflict, "enrollment could not be saved", err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]string{"artifact": base64.StdEncoding.EncodeToString(artifact), "expires_at": expiry.UTC().Format(time.RFC3339), "filename": filename})
		return
	}
	s.mu.RLock()
	pool, found := s.pools[input.PoolID]
	s.mu.RUnlock()
	if !found || !pool.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "selected runner pool does not exist or is disabled"})
		return
	}
	s.mu.Lock()
	for existingToken, item := range s.enrollments {
		if item.RunnerID == input.RunnerID && !item.Used {
			delete(s.enrollments, existingToken)
		}
	}
	s.enrollments[token] = &enrollment{Token: token, RunnerID: input.RunnerID, Expires: expiry}
	item, ok := s.runners[input.RunnerID]
	if !ok {
		item = RunnerRecord{ID: input.RunnerID, Name: input.RunnerID, PoolID: input.PoolID, DesiredState: "ENABLED", ObservedState: "PENDING", Pool: pool.Name, Platform: input.Platform, Architecture: input.Architecture}
	}
	if input.Capacity != nil || !ok {
		item.Capacity = store.DefaultRunnerCapacity
		if input.Capacity != nil {
			item.Capacity = *input.Capacity
		}
	}
	s.runners[input.RunnerID] = item
	s.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"artifact": base64.StdEncoding.EncodeToString(artifact), "expires_at": expiry.UTC().Format(time.RFC3339), "filename": filename})
}

func (s *InfrastructureService) buildRunnerArtifact(r *http.Request, platformName, architecture, runnerID, token string) ([]byte, string, error) {
	s.mu.RLock()
	directory, natsURL, maxMessageBytes, controlPlanePublicKey := s.runnerBinaryDir, s.runnerNATSURL, s.runnerMaxMessageBytes, s.controlPlanePublicKey
	s.mu.RUnlock()
	binaryName := "glyphflow-runner-" + platformName + "-" + architecture
	filename := runnerID + "-" + binaryName
	if platformName == "windows" {
		binaryName += ".exe"
		filename += ".exe"
	}
	raw, err := os.ReadFile(filepath.Join(directory, binaryName))
	if err != nil {
		return nil, "", err
	}
	controlPlaneURL := requestBaseURL(r)
	packed, err := worker.PackBootstrap(raw, worker.Bootstrap{Token: token, RunnerID: runnerID, ControlPlaneURL: controlPlaneURL, ControlPublicKey: controlPlanePublicKey, NATSURL: natsURL, MaxMessageBytes: maxMessageBytes})
	return packed, filename, err
}

func requestBaseURL(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}
func (s *InfrastructureService) resourceCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var input struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource name is required"})
			return
		}
		if strings.TrimSpace(input.Kind) == "" {
			input.Kind = "exclusive"
		}
		id, err := randomID()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "resource creation failed", err)
			return
		}
		s.mu.RLock()
		repository := s.resourceRepository
		s.mu.RUnlock()
		if repository != nil {
			if err := repository.Create(r.Context(), "resource-"+id, strings.TrimSpace(input.Name), strings.TrimSpace(input.Kind)); err != nil {
				writeError(w, http.StatusBadRequest, "resource creation failed", err)
				return
			}
			item, found, err := repository.Find(r.Context(), "resource-"+id)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "resource storage unavailable", err)
				return
			}
			if !found {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resource was created but could not be read back"})
				return
			}
			writeJSON(w, http.StatusCreated, resourceRecordFromStore(item))
			return
		}
		item := ResourceRecord{ID: "resource-" + id, Name: strings.TrimSpace(input.Name), Enabled: true}
		s.mu.Lock()
		s.resources[item.ID] = item
		s.mu.Unlock()
		writeJSON(w, http.StatusCreated, item)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	s.mu.RLock()
	repository := s.resourceRepository
	s.mu.RUnlock()
	if repository != nil {
		items, err := repository.List(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "resource storage unavailable", err)
			return
		}
		result := make([]ResourceRecord, 0, len(items))
		for _, item := range items {
			result = append(result, resourceRecordFromStore(item))
		}
		result = filterResources(result, r.URL.Query().Get("search"))
		writePage(w, r, result)
		return
	}
	s.mu.RLock()
	items := make([]ResourceRecord, 0, len(s.resources))
	for _, item := range s.resources {
		items = append(items, item)
	}
	s.mu.RUnlock()
	items = filterResources(items, r.URL.Query().Get("search"))
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writePage(w, r, items)
}

func filterResources(items []ResourceRecord, search string) []ResourceRecord {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID), search) || strings.Contains(strings.ToLower(item.Name), search) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
func (s *InfrastructureService) resourcePath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[4] == "lease" {
		s.mu.RLock()
		repository := s.resourceRepository
		s.mu.RUnlock()
		if r.Method == http.MethodPost {
			var input struct {
				Holder     string `json:"holder"`
				TTLSeconds int    `json:"ttl_seconds"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Holder) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "holder is required"})
				return
			}
			if input.TTLSeconds == 0 {
				input.TTLSeconds = 30
			}
			if repository != nil {
				item, err := repository.Acquire(r.Context(), parts[3], input.Holder, time.Duration(input.TTLSeconds)*time.Second, time.Now().UTC())
				if err != nil {
					writeError(w, http.StatusConflict, "resource lease could not be acquired", err)
					return
				}
				writeJSON(w, http.StatusCreated, resourceRecordFromStore(item))
				return
			}
			item, err := s.AcquireLease(parts[3], input.Holder, time.Duration(input.TTLSeconds)*time.Second)
			if err != nil {
				status := http.StatusConflict
				if errors.Is(err, errResourceNotFound) || errors.Is(err, errInvalidLease) {
					status = http.StatusBadRequest
				}
				writeError(w, status, "resource lease could not be acquired", err)
				return
			}
			writeJSON(w, http.StatusCreated, item)
			return
		}
		if r.Method == http.MethodDelete {
			var input struct {
				Holder       string `json:"holder"`
				FencingToken int64  `json:"fencing_token"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Holder) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "holder is required"})
				return
			}
			if repository != nil {
				if err := repository.Release(r.Context(), parts[3], input.Holder, input.FencingToken); err != nil {
					writeError(w, http.StatusConflict, "resource lease could not be released", err)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if err := s.ReleaseLease(parts[3], input.Holder, input.FencingToken); err != nil {
				writeError(w, http.StatusConflict, "resource lease could not be released", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	if len(parts) != 4 {
		writeJSON(w, 404, map[string]string{"error": "resource not found"})
		return
	}
	id := parts[3]
	s.mu.RLock()
	repository := s.resourceRepository
	s.mu.RUnlock()
	if repository != nil {
		item, found, err := repository.Find(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "resource storage unavailable", err)
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, resourceRecordFromStore(item))
		} else if r.Method == http.MethodDelete {
			if err := repository.Delete(r.Context(), id); err != nil {
				writeError(w, http.StatusConflict, "resource deletion failed", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	}
	s.mu.Lock()
	item, ok := s.resources[id]
	if ok && r.Method == http.MethodDelete {
		if item.ActiveReferences > 0 || resourceLeaseActive(item, time.Now()) {
			s.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"error": "resource is in use"})
			return
		}
		delete(s.resources, id)
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "resource not found"})
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, item)
	} else if r.Method == http.MethodDelete {
		writeJSON(w, 204, nil)
	} else {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

var (
	errEnrollmentNotFound = errors.New("enrollment not found")
	errEnrollmentExpired  = errors.New("enrollment expired")
	errEnrollmentUsed     = errors.New("enrollment already used")
	errResourceNotFound   = errors.New("resource not found")
	errInvalidLease       = errors.New("invalid lease")
	errLeaseConflict      = errors.New("resource lease is active")
	errLeaseOwner         = errors.New("lease owner or fencing token does not match")
)

var runnerIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ConsumeEnrollment atomically validates and consumes a runner enrollment artifact.
func (s *InfrastructureService) ConsumeEnrollment(token string, now time.Time) (RunnerRecord, error) {
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		runner, err := repository.ConsumeEnrollment(context.Background(), platform.HashToken(token), now)
		return runnerRecordFromStore(runner), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.enrollments[token]
	if !ok {
		return RunnerRecord{}, errEnrollmentNotFound
	}
	if item.Used {
		return RunnerRecord{}, errEnrollmentUsed
	}
	if !now.Before(item.Expires) {
		return RunnerRecord{}, errEnrollmentExpired
	}
	runner, ok := s.runners[item.RunnerID]
	if !ok {
		return RunnerRecord{}, errEnrollmentNotFound
	}
	item.Used = true
	runner.ObservedState = "PENDING"
	runner.HeartbeatAt = ""
	s.runners[item.RunnerID] = runner
	return runner, nil
}

func (s *InfrastructureService) consumeEnrollmentWithKey(token string, now time.Time, keyID string, publicKey []byte) (RunnerRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.enrollments[token]
	if !ok {
		return RunnerRecord{}, errEnrollmentNotFound
	}
	if item.Used {
		return RunnerRecord{}, errEnrollmentUsed
	}
	if !now.Before(item.Expires) {
		return RunnerRecord{}, errEnrollmentExpired
	}
	runner, ok := s.runners[item.RunnerID]
	if !ok {
		return RunnerRecord{}, errEnrollmentNotFound
	}
	for runnerID, existing := range s.runnerKeys {
		if runnerID != item.RunnerID && existing.ID == keyID {
			return RunnerRecord{}, errors.New("runner enrollment key is already bound")
		}
	}
	item.Used = true
	runner.ObservedState = "PENDING"
	runner.HeartbeatAt = ""
	s.enrollments[token] = item
	s.runners[item.RunnerID] = runner
	s.runnerKeys[item.RunnerID] = runnerKey{ID: keyID, Public: append(ed25519.PublicKey(nil), publicKey...)}
	return runner, nil
}

// MarkStale moves runners with an old heartbeat to the offline observed state.
func (s *InfrastructureService) MarkStale(now time.Time, maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, runner := range s.runners {
		seen, err := time.Parse(time.RFC3339, runner.HeartbeatAt)
		if err == nil && now.Sub(seen) > maxAge {
			runner.ObservedState = "OFFLINE"
			s.runners[id] = runner
		}
	}
}

func (s *InfrastructureService) AcquireLease(id, holder string, ttl time.Duration) (ResourceRecord, error) {
	if strings.TrimSpace(holder) == "" || ttl <= 0 {
		return ResourceRecord{}, errInvalidLease
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.resources[id]
	if !ok {
		return ResourceRecord{}, errResourceNotFound
	}
	if resourceLeaseActive(item, now) {
		return ResourceRecord{}, errLeaseConflict
	}
	item.Holder = holder
	item.ExpiresAt = now.Add(ttl).UTC().Format(time.RFC3339)
	item.FencingToken++
	s.resources[id] = item
	return item, nil
}

func (s *InfrastructureService) ReleaseLease(id, holder string, fencingToken int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.resources[id]
	if !ok {
		return errResourceNotFound
	}
	if item.Holder != holder || item.FencingToken != fencingToken {
		return errLeaseOwner
	}
	item.Holder, item.ExpiresAt = "", ""
	s.resources[id] = item
	return nil
}

func resourceLeaseActive(item ResourceRecord, now time.Time) bool {
	if item.Holder == "" {
		return false
	}
	expires, err := time.Parse(time.RFC3339, item.ExpiresAt)
	return err != nil || now.Before(expires)
}
