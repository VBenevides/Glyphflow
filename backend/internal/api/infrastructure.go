package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
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
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	PoolID          string              `json:"poolId,omitempty"`
	DesiredState    string              `json:"desiredState"`
	ObservedState   string              `json:"observedState"`
	Pool            string              `json:"pool"`
	Capacity        int                 `json:"capacity"`
	CurrentCapacity int                 `json:"currentCapacity,omitempty"`
	ActiveCount     int                 `json:"activeCount"`
	HeartbeatAt     string              `json:"heartbeatAt,omitempty"`
	CurrentMetrics  *RunnerMetricRecord `json:"currentMetrics,omitempty"`
	Platform        string              `json:"platform,omitempty"`
	Architecture    string              `json:"architecture,omitempty"`
	NATSEndpoint    string              `json:"natsEndpoint,omitempty"`
	ControlPlaneURL string              `json:"controlPlaneUrl,omitempty"`
	IsArchived      bool                `json:"isArchived"`
	IsDeleted       bool                `json:"isDeleted"`
}
type RunnerPoolRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	IsDeleted   bool   `json:"isDeleted"`
}
type ResourceRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
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
type runnerArtifact struct {
	data     []byte
	filename string
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
	runnerMetrics            map[string][]store.RunnerMetricsRecord
	next                     int
	runnerRepository         store.RunnerRepository
	resourceRepository       store.ResourceRepository
	runnerBinaryDir          string
	runnerControlPlaneURL    string
	runnerNATSURL            string
	runnerMaxMessageBytes    int
	allowInsecureTransport   bool
	enrollmentRateLimiter    *platform.RateLimiter
	controlPlanePublicKey    string
	runnerCapacityPublisher  queue.Publisher
	runnerCapacitySigningKey protocol.SigningKey
}

func NewInfrastructureService() *InfrastructureService {
	return &InfrastructureService{runners: map[string]RunnerRecord{}, pools: map[string]RunnerPoolRecord{"default": {ID: "default", Name: "default", Description: "Default Runner Pool", Enabled: true}}, resources: map[string]ResourceRecord{}, enrollments: map[string]*enrollment{}, runnerKeys: map[string]runnerKey{}, runnerMetrics: map[string][]store.RunnerMetricsRecord{}, runnerBinaryDir: "runner-binaries", allowInsecureTransport: true, enrollmentRateLimiter: platform.NewRateLimiter(10, time.Minute)}
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

func (s *InfrastructureService) hasDurableRepositories() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runnerRepository != nil && s.resourceRepository != nil
}

func (s *InfrastructureService) SetRunnerBinaryDirectory(directory string) {
	if strings.TrimSpace(directory) == "" {
		return
	}
	s.mu.Lock()
	s.runnerBinaryDir = directory
	s.mu.Unlock()
}

func (s *InfrastructureService) SetRunnerControlPlaneURL(url string) {
	s.mu.Lock()
	s.runnerControlPlaneURL = strings.TrimRight(strings.TrimSpace(url), "/")
	s.mu.Unlock()
}

func (s *InfrastructureService) SetRunnerArtifactConfig(natsURL string, maxMessageBytes int) {
	s.mu.Lock()
	s.runnerNATSURL = strings.TrimSpace(natsURL)
	s.runnerMaxMessageBytes = maxMessageBytes
	s.mu.Unlock()
}

func (s *InfrastructureService) SetRunnerEndpointPolicy(allowInsecure bool) {
	s.mu.Lock()
	s.allowInsecureTransport = allowInsecure
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
	var currentMetrics *RunnerMetricRecord
	if runner.CurrentMetrics != nil {
		mapped := runnerMetricRecordFromStore(*runner.CurrentMetrics)
		currentMetrics = &mapped
	}
	return RunnerRecord{ID: runner.ID, Name: runner.Name, PoolID: runner.PoolID, DesiredState: runner.DesiredState, ObservedState: runner.ObservedState, Pool: runner.Pool, Capacity: runner.Capacity, CurrentCapacity: runner.CurrentCapacity, ActiveCount: runner.ActiveCount, HeartbeatAt: heartbeat, CurrentMetrics: currentMetrics, Platform: runner.Platform, Architecture: runner.Architecture, NATSEndpoint: runner.NATSEndpoint, ControlPlaneURL: runner.ControlPlaneURL, IsArchived: runner.IsArchived, IsDeleted: runner.IsDeleted}
}

func runnerPoolRecordFromStore(pool store.RunnerPoolRecord) RunnerPoolRecord {
	return RunnerPoolRecord{ID: pool.ID, Name: pool.Name, Description: pool.Description, Enabled: pool.Enabled, IsDeleted: pool.IsDeleted}
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
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerPoolNotFound})
		return
	}
	id := parts[4]
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	switch r.Method {
	case http.MethodGet:
		s.poolGet(w, r, id, repository)
	case http.MethodDelete:
		s.poolDelete(w, r, id, repository)
	case http.MethodPut:
		s.poolUpdate(w, r, id, repository)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
	}
}

func (s *InfrastructureService) poolGet(w http.ResponseWriter, r *http.Request, id string, repository store.RunnerRepository) {
	if repository != nil {
		item, found, err := repository.FindPool(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "runner pool storage unavailable", err)
		} else if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerPoolNotFound})
		} else {
			writeJSON(w, http.StatusOK, runnerPoolRecordFromStore(item))
		}
		return
	}
	s.mu.RLock()
	item, found := s.pools[id]
	s.mu.RUnlock()
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerPoolNotFound})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *InfrastructureService) poolDelete(w http.ResponseWriter, r *http.Request, id string, repository store.RunnerRepository) {
	if repository != nil {
		if err := repository.DeletePool(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrRunnerPoolInUse) || errors.Is(err, store.ErrRunnerPoolHasTaskVersions) {
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerPoolNotFound})
		return
	}
	for runnerID, runner := range s.runners {
		if runner.PoolID != id {
			continue
		}
		if !runner.IsArchived && !runner.IsDeleted {
			s.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"error": store.ErrRunnerPoolInUse.Error()})
			return
		}
		runner.PoolID, runner.Pool = "", ""
		s.runners[runnerID] = runner
	}
	delete(s.pools, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *InfrastructureService) poolUpdate(w http.ResponseWriter, r *http.Request, id string, repository store.RunnerRepository) {
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
			writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerPoolNotFound})
		} else {
			writeJSON(w, http.StatusOK, runnerPoolRecordFromStore(updated))
		}
		return
	}
	s.mu.Lock()
	if _, found := s.pools[id]; !found {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerPoolNotFound})
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
	return ResourceRecord{ID: resource.ID, Name: resource.Name, Kind: resource.Kind, Enabled: resource.Enabled, Holder: resource.Holder, ExpiresAt: expiresAt, FencingToken: resource.FencingToken}
}

func (s *InfrastructureService) runnerCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		var items []store.RunnerRecord
		var err error
		if strings.EqualFold(r.URL.Query().Get("archived"), "true") {
			items, err = repository.ListArchived(r.Context())
		} else {
			items, err = repository.List(r.Context())
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "runner storage unavailable", err)
			return
		}
		result := make([]RunnerRecord, 0, len(items))
		for _, item := range items {
			result = append(result, runnerRecordFromStore(item))
		}
		result = filterRunners(result, r.URL.Query().Get("state"), r.URL.Query().Get("search"), r.URL.Query().Get("desired_state"))
		sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
		writePage(w, r, result)
		return
	}
	s.mu.RLock()
	items := make([]RunnerRecord, 0, len(s.runners))
	archived := strings.EqualFold(r.URL.Query().Get("archived"), "true")
	for _, item := range s.runners {
		if archived == (item.IsArchived || item.IsDeleted) {
			items = append(items, s.runnerWithCurrentMetrics(item))
		}
	}
	s.mu.RUnlock()
	items = filterRunners(items, r.URL.Query().Get("state"), r.URL.Query().Get("search"), r.URL.Query().Get("desired_state"))
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writePage(w, r, items)
}

func filterRunners(items []RunnerRecord, state string, searchValues ...string) []RunnerRecord {
	state = strings.TrimSpace(state)
	search := ""
	desiredState := ""
	if len(searchValues) > 0 {
		search = searchValues[0]
	}
	if len(searchValues) > 1 {
		desiredState = searchValues[1]
	}
	search = strings.ToLower(strings.TrimSpace(search))
	desiredState = strings.TrimSpace(desiredState)
	if state == "" && search == "" && desiredState == "" {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		matchesSearch := search == "" || strings.Contains(strings.ToLower(item.ID), search) || strings.Contains(strings.ToLower(item.Name), search) || strings.Contains(strings.ToLower(item.Pool), search)
		if (state == "" || strings.EqualFold(item.ObservedState, state)) && (desiredState == "" || strings.EqualFold(item.DesiredState, desiredState)) && matchesSearch {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
func (s *InfrastructureService) runnerPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		writeJSON(w, 404, map[string]string{"error": errorRunnerNotFound})
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
		s.runnerGet(w, r, parts[3])
		return
	}
	if len(parts) == 5 && r.Method == http.MethodGet && parts[4] == "metrics" {
		s.runnerMetricsPath(w, r, parts[3])
		return
	}
	if len(parts) == 4 && r.Method == http.MethodPut {
		s.runnerUpdate(w, r, parts[3])
		return
	}
	if len(parts) == 5 && r.Method == http.MethodPost {
		s.runnerAction(w, r, parts[3], parts[4])
		return
	}
	writeJSON(w, 404, map[string]string{"error": "runner route not found"})
}

func (s *InfrastructureService) runnerGet(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		item, found, err := repository.Find(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "runner storage unavailable", err)
		} else if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		} else {
			writeJSON(w, http.StatusOK, runnerRecordFromStore(item))
		}
		return
	}
	s.mu.RLock()
	item, ok := s.runners[id]
	if ok {
		item = s.runnerWithCurrentMetrics(item)
	}
	s.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type runnerUpdateInput struct {
	Capacity        *int    `json:"capacity"`
	NATSEndpoint    *string `json:"nats_endpoint"`
	ControlPlaneURL *string `json:"control_plane_url"`
}

func (s *InfrastructureService) runnerUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var input runnerUpdateInput
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runner update"})
		return
	}
	if input.Capacity == nil && input.NATSEndpoint == nil && input.ControlPlaneURL == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "capacity, nats_endpoint, or control_plane_url is required"})
		return
	}
	if input.Capacity != nil && *input.Capacity < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "capacity must be at least 1"})
		return
	}
	s.mu.RLock()
	repository, publisher, signingKey := s.runnerRepository, s.runnerCapacityPublisher, s.runnerCapacitySigningKey
	s.mu.RUnlock()
	item, handled := s.updateRunnerRecord(w, r, id, input, repository)
	if handled {
		return
	}
	if publisher != nil && input.Capacity != nil {
		if err := s.publishRunnerCapacity(r, id, *input.Capacity, publisher, signingKey); err != nil {
			writeError(w, http.StatusServiceUnavailable, "runner capacity command failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *InfrastructureService) updateRunnerRecord(w http.ResponseWriter, r *http.Request, id string, input runnerUpdateInput, repository store.RunnerRepository) (RunnerRecord, bool) {
	if repository != nil {
		return s.updateRunnerRepository(w, r, id, input, repository)
	}
	return s.updateRunnerInMemory(w, id, input)
}

func (s *InfrastructureService) loadRunnerForUpdate(w http.ResponseWriter, r *http.Request, id string, capacity *int, repository store.RunnerRepository) (RunnerRecord, bool, bool) {
	if capacity != nil {
		updated, found, err := repository.UpdateCapacity(r.Context(), id, *capacity)
		if err != nil {
			writeError(w, http.StatusConflict, "runner capacity update failed", err)
			return RunnerRecord{}, false, true
		}
		return runnerRecordFromStore(updated), found, false
	}
	stored, found, err := repository.Find(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusConflict, "runner update failed", err)
		return RunnerRecord{}, false, true
	}
	return runnerRecordFromStore(stored), found, false
}

func (s *InfrastructureService) updateRunnerRepository(w http.ResponseWriter, r *http.Request, id string, input runnerUpdateInput, repository store.RunnerRepository) (RunnerRecord, bool) {
	item, found, handled := s.loadRunnerForUpdate(w, r, id, input.Capacity, repository)
	if handled {
		return item, true
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		return item, true
	}
	if input.NATSEndpoint != nil {
		updated, exists, err := repository.UpdateNATSEndpoint(r.Context(), id, *input.NATSEndpoint)
		if err != nil {
			writeError(w, http.StatusConflict, "runner NATS endpoint update failed", err)
			return item, true
		}
		item, found = runnerRecordFromStore(updated), exists
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		return item, true
	}
	if input.ControlPlaneURL != nil {
		updated, exists, err := repository.UpdateControlPlaneURL(r.Context(), id, *input.ControlPlaneURL)
		if err != nil {
			writeError(w, http.StatusConflict, "runner control plane URL update failed", err)
			return item, true
		}
		item, found = runnerRecordFromStore(updated), exists
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		return item, true
	}
	return item, false
}

func (s *InfrastructureService) updateRunnerInMemory(w http.ResponseWriter, id string, input runnerUpdateInput) (RunnerRecord, bool) {
	s.mu.Lock()
	item, found := s.runners[id]
	if found && (item.IsArchived || item.IsDeleted) {
		found = false
	}
	if found {
		if input.Capacity != nil {
			item.Capacity = *input.Capacity
		}
		if input.NATSEndpoint != nil {
			item.NATSEndpoint = strings.TrimSpace(*input.NATSEndpoint)
		}
		if input.ControlPlaneURL != nil {
			item.ControlPlaneURL = strings.TrimRight(strings.TrimSpace(*input.ControlPlaneURL), "/")
		}
		s.runners[id] = item
	}
	s.mu.Unlock()
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		return item, true
	}
	return item, false
}

func (s *InfrastructureService) publishRunnerCapacity(r *http.Request, id string, capacity int, publisher queue.Publisher, signingKey protocol.SigningKey) error {
	if len(signingKey.Private) != ed25519.PrivateKeySize {
		return errors.New("runner capacity signing key is unavailable")
	}
	payload, err := protocol.EncodeRunnerControlPayload(protocol.RunnerControlPayload{Version: protocol.ProtocolVersion, Type: protocol.RunnerControlCapacity, RunnerID: id, Capacity: capacity, IssuedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	envelope, err := signingKey.SignEvent(payload)
	if err != nil {
		return err
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		return err
	}
	return publisher.Publish(r.Context(), queue.Message{Subject: queue.Subject("control", id), ID: "runner-capacity:" + id + ":" + strconv.FormatInt(time.Now().UnixNano(), 10), Data: raw})
}

func (s *InfrastructureService) runnerAction(w http.ResponseWriter, r *http.Request, id, action string) {
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	states := map[string]string{"enable": "ENABLED", "disable": "DISABLED", "drain": "DRAINING", "reset": "ENABLED", "revoke": "REVOKED"}
	state, valid := states[action]
	if repository != nil {
		s.runnerRepositoryAction(w, r, id, state, valid, repository)
		return
	}
	s.runnerInMemoryAction(w, id, state, valid)
}

func (s *InfrastructureService) runnerRepositoryAction(w http.ResponseWriter, r *http.Request, id, state string, valid bool, repository store.RunnerRepository) {
	if !valid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner action not found"})
		return
	}
	item, found, err := repository.SetDesiredState(r.Context(), id, state)
	if err != nil {
		writeError(w, http.StatusConflict, "runner state update failed", err)
	} else if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
	} else {
		writeJSON(w, http.StatusOK, runnerRecordFromStore(item))
	}
}

func (s *InfrastructureService) runnerInMemoryAction(w http.ResponseWriter, id, state string, valid bool) {
	s.mu.Lock()
	item, ok := s.runners[id]
	if ok && (item.IsArchived || item.IsDeleted) {
		ok = false
	}
	if ok && valid {
		item.DesiredState = state
		if state == "ENABLED" && item.ObservedState == "REVOKED" {
			item.ObservedState = "OFFLINE"
		}
		s.runners[id] = item
	}
	s.mu.Unlock()
	if !valid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner action not found"})
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
	} else {
		writeJSON(w, http.StatusOK, item)
	}
}

func (s *InfrastructureService) enrollRunner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	s.mu.RLock()
	limiter := s.enrollmentRateLimiter
	s.mu.RUnlock()
	if limiter != nil && !limiter.AllowSource("runner-bootstrap", remoteAddress(r), time.Now().UTC()) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "runner enrollment rate limit exceeded"})
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
		archived, err := repository.Archive(r.Context(), id)
		if err != nil {
			recordRequestError(r, err)
			writeError(w, http.StatusConflict, "runner archival failed", err)
			return
		}
		if !archived {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mu.Lock()
	runner, ok := s.runners[id]
	if !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorRunnerNotFound})
		return
	}
	runner.IsArchived, runner.IsDeleted = true, true
	runner.DesiredState, runner.ObservedState = "DISABLED", "OFFLINE"
	runner.HeartbeatAt = ""
	s.runners[id] = runner
	for token, item := range s.enrollments {
		if item.RunnerID == id {
			delete(s.enrollments, token)
		}
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

type runnerEnrollmentInput struct {
	RunnerID             string `json:"runner_id"`
	RunnerName           string `json:"runner_name"`
	EmbeddedNATSEndpoint string `json:"embedded_nats_endpoint"`
	ControlPlaneURL      string `json:"control_plane_url"`
	PoolID               string `json:"pool_id"`
	Platform             string `json:"platform"`
	Architecture         string `json:"architecture"`
	Capacity             *int   `json:"capacity"`
	UI                   string `json:"ui"`
}

func parseRunnerEnrollment(w http.ResponseWriter, r *http.Request) (runnerEnrollmentInput, bool) {
	var input runnerEnrollmentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid runner enrollment request", err)
		return input, false
	}
	input.RunnerID = strings.TrimSpace(input.RunnerID)
	input.RunnerName = strings.TrimSpace(input.RunnerName)
	input.ControlPlaneURL = strings.TrimRight(strings.TrimSpace(input.ControlPlaneURL), "/")
	if input.RunnerID == "" && input.RunnerName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_name is required"})
		return input, false
	}
	if input.RunnerName != "" {
		if !runnerIDPattern.MatchString(input.RunnerName) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_name must contain only letters, digits, dot, underscore, or hyphen"})
			return input, false
		}
		rawID := make([]byte, 8)
		if _, err := rand.Read(rawID); err != nil {
			writeError(w, http.StatusInternalServerError, "enrollment failed while generating a runner id", err)
			return input, false
		}
		input.RunnerID = input.RunnerName + "-" + hex.EncodeToString(rawID)
	} else {
		input.RunnerName = input.RunnerID
	}
	input.PoolID = strings.TrimSpace(input.PoolID)
	if input.PoolID == "" {
		input.PoolID = "default"
	}
	input.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
	input.Architecture = strings.ToLower(strings.TrimSpace(input.Architecture))
	input.UI = strings.ToLower(strings.TrimSpace(input.UI))
	if input.UI == "" {
		input.UI = "gui"
	}
	if !validateRunnerEnrollmentInput(w, input) {
		return input, false
	}
	return input, true
}

func validateRunnerEnrollmentInput(w http.ResponseWriter, input runnerEnrollmentInput) bool {
	if !runnerIDPattern.MatchString(input.RunnerID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_id must contain only letters, digits, dot, underscore, or hyphen"})
		return false
	}
	if input.Platform != "linux" && input.Platform != "windows" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform must be linux or windows"})
		return false
	}
	if input.Architecture != "amd64" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "architecture must be amd64"})
		return false
	}
	if input.UI != "gui" && input.UI != "tui" && input.UI != "headless" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ui must be gui, tui, or headless"})
		return false
	}
	if input.Capacity != nil && *input.Capacity < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "capacity must be at least 1"})
		return false
	}
	return true
}

func (s *InfrastructureService) enroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	s.mu.RLock()
	limiter := s.enrollmentRateLimiter
	s.mu.RUnlock()
	if limiter != nil && !limiter.AllowSource("runner-artifact", remoteAddress(r), time.Now().UTC()) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "runner enrollment rate limit exceeded"})
		return
	}
	input, ok := parseRunnerEnrollment(w, r)
	if !ok {
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
	artifact, filename, err := s.buildRunnerArtifact(r, input.Platform, input.Architecture, input.RunnerID, token, input.ControlPlaneURL, input.EmbeddedNATSEndpoint, input.UI)
	if err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusServiceUnavailable, "runner binary is unavailable", err)
		return
	}
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		s.enrollDurable(w, r, input, token, expiry, runnerArtifact{data: artifact, filename: filename}, repository)
		return
	}
	s.enrollInMemory(w, input, token, expiry, artifact, filename)
}

func (s *InfrastructureService) enrollDurable(w http.ResponseWriter, r *http.Request, input runnerEnrollmentInput, token string, expiry time.Time, artifact runnerArtifact, repository store.RunnerRepository) {
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
	if err := repository.CreateEnrollment(r.Context(), store.RunnerRecord{ID: input.RunnerID, Name: input.RunnerName, PoolID: input.PoolID, Capacity: capacity, Platform: input.Platform, Architecture: input.Architecture, NATSEndpoint: input.EmbeddedNATSEndpoint, ControlPlaneURL: input.ControlPlaneURL}, store.RunnerEnrollmentRecord{ID: "enrollment-" + input.RunnerID + "-" + platform.HashToken(token), RunnerID: input.RunnerID, TokenHash: platform.HashToken(token), ExpiresAt: expiry, Requester: requester, Target: input.RunnerID, Artifact: map[string]any{"platform": input.Platform, "architecture": input.Architecture}}); err != nil {
		recordRequestError(r, err)
		writeError(w, http.StatusConflict, "enrollment could not be saved", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"artifact": base64.StdEncoding.EncodeToString(artifact.data), "expires_at": expiry.UTC().Format(time.RFC3339), "filename": artifact.filename, "runner_id": input.RunnerID, "runner_name": input.RunnerName})
}

func (s *InfrastructureService) enrollInMemory(w http.ResponseWriter, input runnerEnrollmentInput, token string, expiry time.Time, artifact []byte, filename string) {
	s.mu.RLock()
	pool, found := s.pools[input.PoolID]
	s.mu.RUnlock()
	if !found || !pool.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "selected runner pool does not exist or is disabled"})
		return
	}
	if s.saveInMemoryEnrollment(input, token, expiry, pool) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "runner is archived"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"artifact": base64.StdEncoding.EncodeToString(artifact), "expires_at": expiry.UTC().Format(time.RFC3339), "filename": filename, "runner_id": input.RunnerID, "runner_name": input.RunnerName})
}

func (s *InfrastructureService) saveInMemoryEnrollment(input runnerEnrollmentInput, token string, expiry time.Time, pool RunnerPoolRecord) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for existingToken, item := range s.enrollments {
		if item.RunnerID == input.RunnerID && !item.Used {
			delete(s.enrollments, existingToken)
		}
	}
	s.enrollments[token] = &enrollment{Token: token, RunnerID: input.RunnerID, Expires: expiry}
	item, ok := s.runners[input.RunnerID]
	if ok && (item.IsArchived || item.IsDeleted) {
		return true
	}
	if !ok {
		item = RunnerRecord{ID: input.RunnerID, Name: input.RunnerName, PoolID: input.PoolID, DesiredState: "ENABLED", ObservedState: "PENDING", Pool: pool.Name, Platform: input.Platform, Architecture: input.Architecture, NATSEndpoint: strings.TrimSpace(input.EmbeddedNATSEndpoint), ControlPlaneURL: input.ControlPlaneURL}
	}
	if strings.TrimSpace(input.EmbeddedNATSEndpoint) != "" {
		item.NATSEndpoint = strings.TrimSpace(input.EmbeddedNATSEndpoint)
	}
	if input.ControlPlaneURL != "" {
		item.ControlPlaneURL = input.ControlPlaneURL
	}
	if input.Capacity != nil || !ok {
		item.Capacity = store.DefaultRunnerCapacity
		if input.Capacity != nil {
			item.Capacity = *input.Capacity
		}
	}
	s.runners[input.RunnerID] = item
	return false
}

func (s *InfrastructureService) buildRunnerArtifact(r *http.Request, platformName, architecture, runnerID, token, controlPlaneURL, embeddedNATSEndpoint, ui string) ([]byte, string, error) {
	if filepath.IsAbs(platformName) || filepath.IsAbs(architecture) || filepath.IsAbs(ui) ||
		strings.ContainsAny(platformName, `/\`) || strings.ContainsAny(architecture, `/\`) || strings.ContainsAny(ui, `/\`) ||
		(platformName != "linux" && platformName != "windows") || architecture != "amd64" ||
		(ui != "gui" && ui != "tui" && ui != "headless") {
		return nil, "", errors.New("runner artifact target is invalid")
	}
	s.mu.RLock()
	directory, defaultControlPlaneURL, maxMessageBytes, controlPlanePublicKey, repository := s.runnerBinaryDir, s.runnerControlPlaneURL, s.runnerMaxMessageBytes, s.controlPlanePublicKey, s.runnerRepository
	approvedNATSEndpoint, allowInsecure := s.runnerNATSURL, s.allowInsecureTransport
	if controlPlaneURL == "" {
		controlPlaneURL = s.runners[runnerID].ControlPlaneURL
	}
	s.mu.RUnlock()
	if controlPlaneURL == "" && repository != nil {
		item, found, err := repository.Find(r.Context(), runnerID)
		if err != nil {
			return nil, "", err
		}
		if found {
			controlPlaneURL = item.ControlPlaneURL
		}
	}
	binaryName := "glyphflow-runner-" + platformName + "-" + architecture
	if ui != "gui" {
		binaryName += "-" + ui
	}
	filename := runnerID + "-" + binaryName
	if platformName == "windows" {
		binaryName += ".exe"
		filename += ".exe"
	}
	artifactPath := filepath.Join(directory, binaryName)
	relativePath, err := filepath.Rel(directory, artifactPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return nil, "", errors.New("runner artifact path is outside the configured runner binary directory")
	}
	raw, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, "", err
	}
	if controlPlaneURL == "" {
		controlPlaneURL = defaultControlPlaneURL
	}
	if controlPlaneURL == "" {
		controlPlaneURL = requestBaseURL(r)
	}
	if embeddedNATSEndpoint == "" {
		embeddedNATSEndpoint = approvedNATSEndpoint
	}
	if err := validateRunnerEndpoints(controlPlaneURL, embeddedNATSEndpoint, defaultControlPlaneURL, approvedNATSEndpoint, allowInsecure); err != nil {
		return nil, "", err
	}
	packed, err := worker.PackBootstrap(raw, worker.Bootstrap{Token: token, RunnerID: runnerID, ControlPlaneURL: controlPlaneURL, ControlPublicKey: controlPlanePublicKey, NATSURL: strings.TrimSpace(embeddedNATSEndpoint), MaxMessageBytes: maxMessageBytes, AllowInsecureTransport: allowInsecure})
	return packed, filename, err
}

func validateRunnerEndpoints(controlPlaneURL, natsEndpoint, approvedControlPlaneURL, approvedNATSEndpoint string, allowInsecure bool) error {
	control, err := url.Parse(strings.TrimRight(strings.TrimSpace(controlPlaneURL), "/"))
	if err != nil || control.Host == "" || control.User != nil || (control.Scheme != "https" && !(allowInsecure && control.Scheme == "http")) {
		return errors.New("runner control-plane endpoint must use the approved HTTPS URL")
	}
	nats, err := url.Parse(strings.TrimSpace(natsEndpoint))
	if err != nil || nats.Host == "" || nats.User != nil || (nats.Scheme != "tls" && !(allowInsecure && nats.Scheme == "nats")) {
		return errors.New("runner NATS endpoint must use the approved TLS URL")
	}
	if !allowInsecure && approvedControlPlaneURL != "" && strings.TrimRight(strings.TrimSpace(approvedControlPlaneURL), "/") != control.String() {
		return errors.New("runner control-plane endpoint is not approved")
	}
	if !allowInsecure && approvedNATSEndpoint != "" && strings.TrimSpace(approvedNATSEndpoint) != nats.String() {
		return errors.New("runner NATS endpoint is not approved")
	}
	return nil
}

func remoteAddress(r *http.Request) string {
	if r == nil || r.RemoteAddr == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
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
		s.resourceCreate(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	s.mu.RLock()
	repository := s.resourceRepository
	s.mu.RUnlock()
	if repository != nil {
		items, err := repository.List(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, errorResourceStorage, err)
			return
		}
		result := make([]ResourceRecord, 0, len(items))
		for _, item := range items {
			result = append(result, resourceRecordFromStore(item))
		}
		result = filterResources(result, r.URL.Query().Get("search"), r.URL.Query().Get("kind"))
		writePage(w, r, result)
		return
	}
	s.mu.RLock()
	items := make([]ResourceRecord, 0, len(s.resources))
	for _, item := range s.resources {
		items = append(items, item)
	}
	s.mu.RUnlock()
	items = filterResources(items, r.URL.Query().Get("search"), r.URL.Query().Get("kind"))
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writePage(w, r, items)
}

func (s *InfrastructureService) resourceCreate(w http.ResponseWriter, r *http.Request) {
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
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	if input.Kind != "exclusive" && input.Kind != "non-blocking" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource kind must be exclusive or non-blocking"})
		return
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
		if err := repository.Create(r.Context(), resourceIDPrefix+id, strings.TrimSpace(input.Name), strings.TrimSpace(input.Kind)); err != nil {
			writeError(w, http.StatusBadRequest, "resource creation failed", err)
			return
		}
		item, found, err := repository.Find(r.Context(), resourceIDPrefix+id)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, errorResourceStorage, err)
			return
		}
		if !found {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resource was created but could not be read back"})
			return
		}
		writeJSON(w, http.StatusCreated, resourceRecordFromStore(item))
		return
	}
	item := ResourceRecord{ID: resourceIDPrefix + id, Name: strings.TrimSpace(input.Name), Kind: strings.TrimSpace(input.Kind), Enabled: true}
	s.mu.Lock()
	s.resources[item.ID] = item
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, item)
}

func filterResources(items []ResourceRecord, search string, kindValues ...string) []ResourceRecord {
	search = strings.ToLower(strings.TrimSpace(search))
	kind := ""
	if len(kindValues) > 0 {
		kind = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(kindValues[0]), "_", "-"))
	}
	if search == "" && kind == "" {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		itemKind := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.Kind), "_", "-"))
		if (kind == "" || itemKind == kind) && (strings.Contains(strings.ToLower(item.ID), search) || strings.Contains(strings.ToLower(item.Name), search)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
func (s *InfrastructureService) resourcePath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[4] == "lease" {
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			s.mu.RLock()
			repository := s.resourceRepository
			s.mu.RUnlock()
			s.resourceLeasePath(w, r, parts[3], repository)
			return
		}
	}
	if len(parts) != 4 {
		writeJSON(w, 404, map[string]string{"error": errorResourceNotFound})
		return
	}
	id := parts[3]
	s.mu.RLock()
	repository := s.resourceRepository
	s.mu.RUnlock()
	if repository != nil {
		s.resourceRepositoryPath(w, r, id, repository)
		return
	}
	s.resourceInMemoryPath(w, r, id)
}

func (s *InfrastructureService) resourceLeasePath(w http.ResponseWriter, r *http.Request, id string, repository store.ResourceRepository) {
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
			item, err := repository.Acquire(r.Context(), id, input.Holder, time.Duration(input.TTLSeconds)*time.Second, time.Now().UTC())
			if err != nil {
				writeError(w, http.StatusConflict, "resource lease could not be acquired", err)
				return
			}
			writeJSON(w, http.StatusCreated, resourceRecordFromStore(item))
			return
		}
		item, err := s.AcquireLease(id, input.Holder, time.Duration(input.TTLSeconds)*time.Second)
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
			if err := repository.Release(r.Context(), id, input.Holder, input.FencingToken); err != nil {
				writeError(w, http.StatusConflict, "resource lease could not be released", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := s.ReleaseLease(id, input.Holder, input.FencingToken); err != nil {
			writeError(w, http.StatusConflict, "resource lease could not be released", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *InfrastructureService) resourceRepositoryPath(w http.ResponseWriter, r *http.Request, id string, repository store.ResourceRepository) {
	item, found, err := repository.Find(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, errorResourceStorage, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorResourceNotFound})
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
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
	}
}

func (s *InfrastructureService) resourceInMemoryPath(w http.ResponseWriter, r *http.Request, id string) {
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
		writeJSON(w, 404, map[string]string{"error": errorResourceNotFound})
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, item)
	} else if r.Method == http.MethodDelete {
		writeJSON(w, 204, nil)
	} else {
		writeJSON(w, 405, map[string]string{"error": errorMethodNotAllowed})
	}
}

var (
	errEnrollmentNotFound = errors.New("enrollment not found")
	errEnrollmentExpired  = errors.New("enrollment expired")
	errEnrollmentUsed     = errors.New("enrollment already used")
	errResourceNotFound   = errors.New(errorResourceNotFound)
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
