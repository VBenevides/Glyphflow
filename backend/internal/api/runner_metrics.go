package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type RunnerMetricRecord struct {
	SampledAt        string  `json:"sampledAt"`
	CPUPercent       float64 `json:"cpuPercent"`
	MemoryPercent    float64 `json:"memoryPercent"`
	MemoryUsedBytes  int64   `json:"memoryUsedBytes"`
	MemoryTotalBytes int64   `json:"memoryTotalBytes"`
}

type RunnerMetricHistory struct {
	Items []RunnerMetricRecord `json:"items"`
	From  string               `json:"from"`
	To    string               `json:"to"`
}

func runnerMetricRecordFromStore(item store.RunnerMetricsRecord) RunnerMetricRecord {
	return RunnerMetricRecord{SampledAt: item.SampledAt.UTC().Format(time.RFC3339), CPUPercent: item.CPUPercent, MemoryPercent: item.MemoryPercent, MemoryUsedBytes: item.MemoryUsedBytes, MemoryTotalBytes: item.MemoryTotalBytes}
}

func runnerMetricHistoryRange(r *http.Request) (time.Time, time.Time, int, error) {
	to := time.Now().UTC()
	from := to.Add(-24 * time.Hour)
	for name, target := range map[string]*time.Time{"from": &from, "to": &to} {
		value := strings.TrimSpace(r.URL.Query().Get(name))
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, time.Time{}, 0, err
		}
		*target = parsed.UTC()
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, 0, strconv.ErrSyntax
	}
	limit := 2000
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 2000 {
			return time.Time{}, time.Time{}, 0, strconv.ErrRange
		}
		limit = parsed
	}
	return from, to, limit, nil
}

func (s *InfrastructureService) runnerMetricsPath(w http.ResponseWriter, r *http.Request, runnerID string) {
	from, to, limit, err := runnerMetricHistoryRange(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runner metrics range"})
		return
	}
	s.mu.RLock()
	repository := s.runnerRepository
	_, inMemoryRunner := s.runners[runnerID]
	s.mu.RUnlock()
	if repository != nil {
		if _, found, findErr := repository.Find(r.Context(), runnerID); findErr != nil {
			writeError(w, http.StatusServiceUnavailable, "runner storage unavailable", findErr)
			return
		} else if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
			return
		}
		metricsRepository, ok := repository.(store.RunnerMetricsRepository)
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner metrics storage unavailable"})
			return
		}
		items, listErr := metricsRepository.ListRunnerMetrics(r.Context(), runnerID, from, to, limit)
		if listErr != nil {
			writeError(w, http.StatusServiceUnavailable, "runner metrics storage unavailable", listErr)
			return
		}
		result := make([]RunnerMetricRecord, 0, len(items))
		for _, item := range items {
			result = append(result, runnerMetricRecordFromStore(item))
		}
		writeJSON(w, http.StatusOK, RunnerMetricHistory{Items: result, From: from.Format(time.RFC3339), To: to.Format(time.RFC3339)})
		return
	}
	if !inMemoryRunner {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
		return
	}
	s.mu.RLock()
	items := append([]store.RunnerMetricsRecord(nil), s.runnerMetrics[runnerID]...)
	s.mu.RUnlock()
	result := make([]RunnerMetricRecord, 0, len(items))
	for _, item := range items {
		if item.SampledAt.Before(from) || item.SampledAt.After(to) {
			continue
		}
		result = append(result, runnerMetricRecordFromStore(item))
		if len(result) == limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, RunnerMetricHistory{Items: result, From: from.Format(time.RFC3339), To: to.Format(time.RFC3339)})
}

func (s *InfrastructureService) runnerWithCurrentMetrics(runner RunnerRecord) RunnerRecord {
	items := s.runnerMetrics[runner.ID]
	if len(items) > 0 {
		latest := items[0]
		for _, item := range items[1:] {
			if item.SampledAt.After(latest.SampledAt) {
				latest = item
			}
		}
		runner.CurrentMetrics = &RunnerMetricRecord{SampledAt: latest.SampledAt.UTC().Format(time.RFC3339), CPUPercent: latest.CPUPercent, MemoryPercent: latest.MemoryPercent, MemoryUsedBytes: latest.MemoryUsedBytes, MemoryTotalBytes: latest.MemoryTotalBytes}
	}
	return runner
}
