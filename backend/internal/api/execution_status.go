package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const executionStatusPath = "/api/v1/admin/execution-status"

type ExitCode struct {
	Code     int    `json:"code"`
	Meaning  string `json:"meaning"`
	IsSystem bool   `json:"isSystem"`
}

func (s Server) executionStatusRoutes(mux routeRegistrar) {
	handler := s.require("auth.settings.manage", http.HandlerFunc(s.executionStatus))
	mux.Handle(executionStatusPath, handler)
	mux.Handle(executionStatusPath+"/", handler)
}

func (s Server) executionStatus(w http.ResponseWriter, r *http.Request) {
	if s.ExitCodes == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "execution status unavailable"})
		return
	}
	if r.URL.Path == executionStatusPath {
		s.executionStatusCollection(w, r)
		return
	}
	rawCode := strings.TrimPrefix(r.URL.Path, executionStatusPath+"/")
	code, err := strconv.Atoi(rawCode)
	if err != nil || strings.Contains(rawCode, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "exit code not found"})
		return
	}
	s.executionStatusItem(w, r, code)
}

func (s Server) executionStatusCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.ExitCodes.List(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "execution status unavailable", err)
			return
		}
		writeJSON(w, http.StatusOK, exitCodeRecords(items))
	case http.MethodPost:
		var input struct {
			Code    *int   `json:"code"`
			Meaning string `json:"meaning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Code == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and meaning are required"})
			return
		}
		item, err := s.ExitCodes.Create(r.Context(), *input.Code, input.Meaning)
		if err != nil {
			writeExitCodeError(w, "exit code creation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, exitCodeRecords([]store.ExitCodeRecord{item})[0])
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s Server) executionStatusItem(w http.ResponseWriter, r *http.Request, code int) {
	switch r.Method {
	case http.MethodPut:
		var input struct {
			Code    *int   `json:"code"`
			Meaning string `json:"meaning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "meaning is required"})
			return
		}
		newCode := code
		if input.Code != nil {
			newCode = *input.Code
		}
		item, err := s.ExitCodes.Update(r.Context(), code, newCode, input.Meaning)
		if err != nil {
			writeExitCodeError(w, "exit code update failed", err)
			return
		}
		writeJSON(w, http.StatusOK, exitCodeRecords([]store.ExitCodeRecord{item})[0])
	case http.MethodDelete:
		if err := s.ExitCodes.Delete(r.Context(), code); err != nil {
			writeExitCodeError(w, "exit code deletion failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func exitCodeRecords(items []store.ExitCodeRecord) []ExitCode {
	result := make([]ExitCode, 0, len(items))
	for _, item := range items {
		result = append(result, ExitCode{Code: item.Code, Meaning: item.Meaning, IsSystem: item.IsSystem})
	}
	return result
}

func writeExitCodeError(w http.ResponseWriter, operation string, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrExitCodeNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, store.ErrExitCodeSystem) || errors.Is(err, store.ErrExitCodeExists) || errors.Is(err, store.ErrExitCodeInUse) {
		status = http.StatusConflict
	}
	writeError(w, status, operation, err)
}
