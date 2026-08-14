package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type RunnerRecord struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DesiredState  string `json:"desiredState"`
	ObservedState string `json:"observedState"`
	Pool          string `json:"pool"`
	Capacity      int    `json:"capacity"`
	ActiveCount   int    `json:"activeCount"`
	HeartbeatAt   string `json:"heartbeatAt,omitempty"`
	Platform      string `json:"platform,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
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
type InfrastructureService struct {
	mu                 sync.RWMutex
	runners            map[string]RunnerRecord
	resources          map[string]ResourceRecord
	enrollments        map[string]*enrollment
	next               int
	runnerRepository   store.RunnerRepository
	resourceRepository store.ResourceRepository
}

func NewInfrastructureService() *InfrastructureService {
	return &InfrastructureService{runners: map[string]RunnerRecord{}, resources: map[string]ResourceRecord{}, enrollments: map[string]*enrollment{}}
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

func runnerRecordFromStore(runner store.RunnerRecord) RunnerRecord {
	heartbeat := ""
	if runner.HeartbeatAt != nil {
		heartbeat = runner.HeartbeatAt.UTC().Format(time.RFC3339)
	}
	return RunnerRecord{ID: runner.ID, Name: runner.Name, DesiredState: runner.DesiredState, ObservedState: runner.ObservedState, Pool: runner.Pool, Capacity: runner.Capacity, ActiveCount: runner.ActiveCount, HeartbeatAt: heartbeat, Platform: runner.Platform, Architecture: runner.Architecture}
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
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner storage unavailable"})
			return
		}
		result := make([]RunnerRecord, 0, len(items))
		for _, item := range items {
			result = append(result, runnerRecordFromStore(item))
		}
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
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writePage(w, r, items)
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
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner storage unavailable"})
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
				writeJSON(w, http.StatusConflict, map[string]string{"error": "runner state update failed"})
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

func (s *InfrastructureService) deleteRunner(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		deleted, err := repository.Delete(r.Context(), id)
		if err != nil {
			recordRequestError(r, err)
			writeJSON(w, http.StatusConflict, map[string]string{"error": "runner cannot be deleted"})
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
		Platform     string `json:"platform"`
		Architecture string `json:"architecture"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.RunnerID) == "" {
		writeJSON(w, 400, map[string]string{"error": "runner_id is required"})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		recordRequestError(r, err)
		writeJSON(w, 500, map[string]string{"error": "enrollment failed"})
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiry := time.Now().Add(15 * time.Minute)
	s.mu.RLock()
	repository := s.runnerRepository
	s.mu.RUnlock()
	if repository != nil {
		requester := "system"
		if claims, ok := r.Context().Value(requestClaimsContextKey{}).(Claims); ok && claims.UserID != "" {
			requester = claims.UserID
		}
		if err := repository.CreateEnrollment(r.Context(), store.RunnerRecord{ID: input.RunnerID, Name: input.RunnerID, Pool: "default", Capacity: 1, Platform: input.Platform, Architecture: input.Architecture}, store.RunnerEnrollmentRecord{ID: "enrollment-" + input.RunnerID + "-" + platform.HashToken(token), RunnerID: input.RunnerID, TokenHash: platform.HashToken(token), ExpiresAt: expiry, Requester: requester, Target: input.RunnerID, Artifact: map[string]any{"platform": input.Platform, "architecture": input.Architecture}}); err != nil {
			recordRequestError(r, err)
			writeJSON(w, http.StatusConflict, map[string]string{"error": "enrollment failed"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]string{"artifact": token, "expires_at": expiry.UTC().Format(time.RFC3339), "filename": input.RunnerID + "-enrollment.bin"})
		return
	}
	s.mu.Lock()
	s.enrollments[token] = &enrollment{Token: token, RunnerID: input.RunnerID, Expires: expiry}
	if _, ok := s.runners[input.RunnerID]; !ok {
		s.runners[input.RunnerID] = RunnerRecord{ID: input.RunnerID, Name: input.RunnerID, DesiredState: "ENABLED", ObservedState: "PENDING", Pool: "default", Capacity: 1, Platform: input.Platform, Architecture: input.Architecture}
	}
	s.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, map[string]string{"artifact": token, "expires_at": expiry.UTC().Format(time.RFC3339), "filename": input.RunnerID + "-enrollment.bin"})
}
func (s *InfrastructureService) resourceCollection(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resource storage unavailable"})
			return
		}
		result := make([]ResourceRecord, 0, len(items))
		for _, item := range items {
			result = append(result, resourceRecordFromStore(item))
		}
		writePage(w, r, result)
		return
	}
	s.mu.RLock()
	items := make([]ResourceRecord, 0, len(s.resources))
	for _, item := range s.resources {
		items = append(items, item)
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writePage(w, r, items)
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
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
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
				writeJSON(w, status, map[string]string{"error": err.Error()})
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
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if err := s.ReleaseLease(parts[3], input.Holder, input.FencingToken); err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
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
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "resource storage unavailable"})
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
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
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
	runner.ObservedState = "ONLINE"
	runner.HeartbeatAt = now.UTC().Format(time.RFC3339)
	s.runners[item.RunnerID] = runner
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
