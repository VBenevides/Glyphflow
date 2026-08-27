package api

import (
	"net/http"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
)

func (s Server) scheduleProjection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	report := controlplane.ProjectionReport{}
	if s.ScheduleProjection != nil {
		report = s.ScheduleProjection.Snapshot()
	}
	writeJSON(w, http.StatusOK, report)
}
