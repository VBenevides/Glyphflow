package controlplane

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const ProjectionHorizon = 7 * 24 * time.Hour

type ProjectionResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProjectionOccurrence struct {
	ID                string    `json:"id"`
	ScheduleID        string    `json:"scheduleId"`
	ScheduleName      string    `json:"scheduleName"`
	ScheduleVersionID string    `json:"scheduleVersionId"`
	TaskID            string    `json:"taskId"`
	TaskName          string    `json:"taskName"`
	TaskVersionID     string    `json:"taskVersionId"`
	Timezone          string    `json:"timezone"`
	LaneID            string    `json:"laneId"`
	LaneLabel         string    `json:"laneLabel"`
	StartAt           time.Time `json:"startAt"`
	EndAt             time.Time `json:"endAt"`
}

type ProjectionSegment struct {
	ID                 string               `json:"id"`
	ScheduleID         string               `json:"scheduleId"`
	ScheduleName       string               `json:"scheduleName"`
	ScheduleVersionID  string               `json:"scheduleVersionId"`
	TaskID             string               `json:"taskId"`
	TaskName           string               `json:"taskName"`
	TaskVersionID      string               `json:"taskVersionId"`
	Timezone           string               `json:"timezone"`
	LaneID             string               `json:"laneId"`
	LaneLabel          string               `json:"laneLabel"`
	StartAt            time.Time            `json:"startAt"`
	EndAt              time.Time            `json:"endAt"`
	OccurrenceCount    int                  `json:"occurrenceCount"`
	Conflicted         bool                 `json:"conflicted"`
	ExclusiveResources []ProjectionResource `json:"exclusiveResources"`
}

type ProjectionConflict struct {
	ID           string                 `json:"id"`
	ResourceID   string                 `json:"resourceId"`
	ResourceName string                 `json:"resourceName"`
	StartAt      time.Time              `json:"startAt"`
	EndAt        time.Time              `json:"endAt"`
	Occurrences  []ProjectionOccurrence `json:"occurrences"`
}

type ProjectionReport struct {
	Available      bool                 `json:"available"`
	CalculatedAt   time.Time            `json:"calculatedAt"`
	WindowStart    time.Time            `json:"windowStart"`
	WindowEnd      time.Time            `json:"windowEnd"`
	DurationSource string               `json:"durationSource"`
	Segments       []ProjectionSegment  `json:"segments"`
	Conflicts      []ProjectionConflict `json:"conflicts"`
}

type projectionWindow struct {
	occurrence ProjectionOccurrence
	resources  []ProjectionResource
}

type projectionEvent struct {
	at         time.Time
	start      bool
	occurrence ProjectionOccurrence
}

func BuildScheduleProjection(inputs []store.ScheduleProjectionInput, now time.Time) (ProjectionReport, error) {
	start := now.UTC()
	if start.IsZero() {
		return ProjectionReport{}, errors.New("projection time is required")
	}
	end := start.Add(ProjectionHorizon)
	windows := make([]projectionWindow, 0)
	byResource := make(map[string][]ProjectionOccurrence)
	resourceNames := make(map[string]string)
	for _, input := range inputs {
		if input.ScheduleID == "" || input.TaskID == "" || input.TaskVersionID == "" || input.Expression == "" {
			return ProjectionReport{}, errors.New("projection input is incomplete")
		}
		if input.DurationSeconds <= 0 {
			return ProjectionReport{}, fmt.Errorf("schedule %q has invalid task duration", input.ScheduleID)
		}
		laneID, laneLabel := projectionLane(input)
		cursor := start
		for cursor.Before(end) {
			next, err := NextFire(input.Expression, input.Timezone, cursor)
			if err != nil {
				return ProjectionReport{}, fmt.Errorf("schedule %q: %w", input.ScheduleID, err)
			}
			if !next.After(cursor) {
				return ProjectionReport{}, fmt.Errorf("schedule %q produced a non-increasing occurrence", input.ScheduleID)
			}
			if !next.Before(end) {
				break
			}
			duration := time.Duration(input.DurationSeconds) * time.Second
			finish := next.Add(duration)
			if !finish.After(next) {
				return ProjectionReport{}, fmt.Errorf("schedule %q has an invalid task duration", input.ScheduleID)
			}
			occurrence := ProjectionOccurrence{
				ID:                input.ScheduleID + "@" + next.UTC().Format(time.RFC3339Nano),
				ScheduleID:        input.ScheduleID,
				ScheduleName:      fallbackName(input.ScheduleName, input.ScheduleID),
				ScheduleVersionID: input.ScheduleVersionID,
				TaskID:            input.TaskID,
				TaskName:          fallbackName(input.TaskName, input.TaskID),
				TaskVersionID:     input.TaskVersionID,
				Timezone:          input.Timezone,
				LaneID:            laneID,
				LaneLabel:         laneLabel,
				StartAt:           next.UTC(),
				EndAt:             finish.UTC(),
			}
			resources := exclusiveResources(input.Resources)
			windows = append(windows, projectionWindow{occurrence: occurrence, resources: resources})
			for _, resource := range resources {
				byResource[resource.ID] = append(byResource[resource.ID], occurrence)
				resourceNames[resource.ID] = resource.Name
			}
			cursor = next
		}
	}
	conflicts := projectionConflicts(byResource, resourceNames)
	conflicted := make(map[string]bool)
	for _, conflict := range conflicts {
		for _, occurrence := range conflict.Occurrences {
			conflicted[occurrence.ID] = true
		}
	}
	segments := projectionSegments(windows, conflicted)
	return ProjectionReport{
		Available:      true,
		CalculatedAt:   start,
		WindowStart:    start,
		WindowEnd:      end,
		DurationSource: "task_duration",
		Segments:       segments,
		Conflicts:      conflicts,
	}, nil
}

func projectionLane(input store.ScheduleProjectionInput) (string, string) {
	if input.PinnedRunnerID != "" {
		name := fallbackName(input.PinnedRunnerName, input.PinnedRunnerID)
		return "runner:" + input.PinnedRunnerID, "Runner: " + name
	}
	name := fallbackName(input.RunnerPoolName, input.RunnerPoolID)
	return "pool:" + input.RunnerPoolID, "Any runner in " + name
}

func fallbackName(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}

func exclusiveResources(resources []store.ScheduleProjectionResource) []ProjectionResource {
	result := make([]ProjectionResource, 0, len(resources))
	seen := make(map[string]bool, len(resources))
	for _, resource := range resources {
		kind := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(resource.Kind), "_", "-"))
		if kind != "exclusive" || resource.ID == "" || seen[resource.ID] {
			continue
		}
		seen[resource.ID] = true
		result = append(result, ProjectionResource{ID: resource.ID, Name: fallbackName(resource.Name, resource.ID)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func projectionSegments(windows []projectionWindow, conflicted map[string]bool) []ProjectionSegment {
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].occurrence.LaneLabel != windows[j].occurrence.LaneLabel {
			return windows[i].occurrence.LaneLabel < windows[j].occurrence.LaneLabel
		}
		if windows[i].occurrence.ScheduleID != windows[j].occurrence.ScheduleID {
			return windows[i].occurrence.ScheduleID < windows[j].occurrence.ScheduleID
		}
		return windows[i].occurrence.StartAt.Before(windows[j].occurrence.StartAt)
	})
	segments := make([]ProjectionSegment, 0, len(windows))
	for _, window := range windows {
		o := window.occurrence
		resources := append([]ProjectionResource(nil), window.resources...)
		segments = append(segments, ProjectionSegment{
			ID:                 o.ID,
			ScheduleID:         o.ScheduleID,
			ScheduleName:       o.ScheduleName,
			ScheduleVersionID:  o.ScheduleVersionID,
			TaskID:             o.TaskID,
			TaskName:           o.TaskName,
			TaskVersionID:      o.TaskVersionID,
			Timezone:           o.Timezone,
			LaneID:             o.LaneID,
			LaneLabel:          o.LaneLabel,
			StartAt:            o.StartAt,
			EndAt:              o.EndAt,
			OccurrenceCount:    1,
			Conflicted:         conflicted[o.ID],
			ExclusiveResources: resources,
		})
	}
	return segments
}

func projectionConflicts(byResource map[string][]ProjectionOccurrence, names map[string]string) []ProjectionConflict {
	result := make([]ProjectionConflict, 0)
	for resourceID, occurrences := range byResource {
		events := make([]projectionEvent, 0, len(occurrences)*2)
		for _, occurrence := range occurrences {
			events = append(events, projectionEvent{at: occurrence.StartAt, start: true, occurrence: occurrence}, projectionEvent{at: occurrence.EndAt, start: false, occurrence: occurrence})
		}
		sort.Slice(events, func(i, j int) bool {
			if events[i].at != events[j].at {
				return events[i].at.Before(events[j].at)
			}
			if events[i].start != events[j].start {
				return !events[i].start
			}
			return events[i].occurrence.ID < events[j].occurrence.ID
		})
		active := make(map[string]ProjectionOccurrence)
		var current *ProjectionConflict
		for i := 0; i < len(events); {
			j := i + 1
			for j < len(events) && events[j].at.Equal(events[i].at) {
				j++
			}
			for _, event := range events[i:j] {
				if !event.start {
					delete(active, event.occurrence.ID)
				}
			}
			for _, event := range events[i:j] {
				if event.start {
					active[event.occurrence.ID] = event.occurrence
				}
			}
			if j == len(events) || !events[j].at.After(events[i].at) || len(active) < 2 {
				if current != nil {
					result = append(result, *current)
					current = nil
				}
				i = j
				continue
			}
			spanEnd := events[j].at
			if current == nil {
				current = &ProjectionConflict{ID: resourceID + "@" + events[i].at.UTC().Format(time.RFC3339Nano), ResourceID: resourceID, ResourceName: fallbackName(names[resourceID], resourceID), StartAt: events[i].at.UTC()}
			}
			current.EndAt = spanEnd.UTC()
			seen := make(map[string]bool, len(current.Occurrences))
			for _, occurrence := range current.Occurrences {
				seen[occurrence.ID] = true
			}
			for _, occurrence := range active {
				if !seen[occurrence.ID] {
					current.Occurrences = append(current.Occurrences, occurrence)
				}
			}
			sort.Slice(current.Occurrences, func(a, b int) bool { return current.Occurrences[a].StartAt.Before(current.Occurrences[b].StartAt) })
			i = j
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].StartAt.Equal(result[j].StartAt) {
			return result[i].StartAt.Before(result[j].StartAt)
		}
		if result[i].ResourceName != result[j].ResourceName {
			return result[i].ResourceName < result[j].ResourceName
		}
		return result[i].ResourceID < result[j].ResourceID
	})
	return result
}
