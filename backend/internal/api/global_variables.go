package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type GlobalVariableService struct {
	mu         sync.RWMutex
	repository store.GlobalVariableRepository
	items      map[string]store.GlobalVariableRecord
}

func NewGlobalVariableService() *GlobalVariableService {
	return &GlobalVariableService{items: map[string]store.GlobalVariableRecord{}}
}

func (s *GlobalVariableService) SetRepository(repository store.GlobalVariableRepository) {
	if repository != nil {
		s.mu.Lock()
		s.repository = repository
		s.mu.Unlock()
	}
}

func (s *GlobalVariableService) collection(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	repository := s.repository
	s.mu.RUnlock()
	if r.Method == http.MethodGet {
		if repository != nil {
			items, err := repository.List(r.Context())
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "global variable storage unavailable", err)
				return
			}
			writePage(w, r, items)
			return
		}
		s.mu.RLock()
		items := make([]store.GlobalVariableRecord, 0, len(s.items))
		for _, item := range s.items {
			items = append(items, item)
		}
		s.mu.RUnlock()
		writePage(w, r, items)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input globalVariableInput
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid global variable"})
		return
	}
	name := strings.TrimSpace(input.Name)
	if !platform.GlobalVariableName(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "variable name must use uppercase letters, numbers, and underscores"})
		return
	}
	id, err := randomID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "global variable creation failed", err)
		return
	}
	if repository != nil {
		item, err := repository.Create(r.Context(), "global-"+id, name, input.Value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "global variable creation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
		return
	}
	item := store.GlobalVariableRecord{ID: "global-" + id, Name: name, Value: input.Value, UpdatedAt: time.Now().UTC()}
	s.mu.Lock()
	for _, existing := range s.items {
		if strings.EqualFold(existing.Name, name) {
			s.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"error": "global variable name already exists"})
			return
		}
	}
	s.items[item.ID] = item
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, item)
}

func (s *GlobalVariableService) path(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(strings.Trim(r.URL.Path, "/"), "api/v1/global-variables/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "global variable not found"})
		return
	}
	s.mu.RLock()
	repository := s.repository
	s.mu.RUnlock()
	if r.Method == http.MethodGet {
		if repository != nil {
			item, found, err := repository.Find(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "global variable storage unavailable", err)
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "global variable not found"})
			} else {
				writeJSON(w, http.StatusOK, item)
			}
			return
		}
		s.mu.RLock()
		item, found := s.items[id]
		s.mu.RUnlock()
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "global variable not found"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if r.Method == http.MethodDelete {
		if repository != nil {
			if err := repository.Delete(r.Context(), id); err != nil {
				writeError(w, http.StatusConflict, "global variable cannot be deleted", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mu.Lock()
		if _, found := s.items[id]; !found {
			s.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "global variable not found"})
			return
		}
		delete(s.items, id)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input globalVariableInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || !platform.GlobalVariableName(strings.TrimSpace(input.Name)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid variable name is required"})
		return
	}
	if repository != nil {
		item, err := repository.Update(r.Context(), id, strings.TrimSpace(input.Name), input.Value)
		if err != nil {
			writeError(w, http.StatusConflict, "global variable cannot be updated", err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	s.mu.Lock()
	item, found := s.items[id]
	if found {
		item.Name, item.Value, item.UpdatedAt = strings.TrimSpace(input.Name), input.Value, time.Now().UTC()
		s.items[id] = item
	}
	s.mu.Unlock()
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "global variable not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type globalVariableInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
