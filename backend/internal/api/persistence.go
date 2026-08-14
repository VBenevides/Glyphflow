package api

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

const (
	stateAuth           = "state.auth"
	stateSessions       = "state.sessions"
	stateRefresh        = "state.refresh"
	stateRoles          = "state.roles"
	stateOIDC           = "state.oidc"
	stateAudit          = "state.audit"
	stateOperations     = "state.operations"
	stateInfrastructure = "state.infrastructure"
	stateRuns           = "state.runs"
)

type Persistence struct {
	config *store.ConfigStore
	mu     sync.Mutex
}

func NewPersistence(config *store.ConfigStore) *Persistence {
	return &Persistence{config: config}
}

type sessionState struct {
	Sessions map[string]accessTokenPayload `json:"sessions"`
}

type roleState struct {
	Roles       map[string]RoleDefinition  `json:"roles"`
	Assignments map[string]map[string]bool `json:"assignments"`
}

type oidcState struct {
	Providers map[string]OIDCProvider             `json:"providers"`
	States    platform.AuthorizationStoreSnapshot `json:"states"`
}

type operationsState struct {
	Tasks          map[string]TaskRecord     `json:"tasks"`
	Schedules      map[string]ScheduleRecord `json:"schedules"`
	NextTaskID     int                       `json:"nextTaskId"`
	NextScheduleID int                       `json:"nextScheduleId"`
}

type infrastructureState struct {
	Runners     map[string]RunnerRecord   `json:"runners"`
	Resources   map[string]ResourceRecord `json:"resources"`
	Enrollments map[string]enrollment     `json:"enrollments"`
	Next        int                       `json:"next"`
}

type runsState struct {
	Runs map[string]RunRecord             `json:"runs"`
	Logs map[string]map[string][]LogChunk `json:"logs"`
	Next int                              `json:"next"`
}

func (p *Persistence) Restore(s Server) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.Sessions != nil {
		var state sessionState
		if err := p.load(stateSessions, &state); err != nil {
			return err
		}
		if state.Sessions != nil {
			s.Sessions.mu.Lock()
			s.Sessions.sessions = state.Sessions
			s.Sessions.mu.Unlock()
		}
	}
	if s.AuthService != nil && s.AuthService.refresh != nil {
		var state struct {
			Sessions map[string]platform.RefreshSessionSnapshot `json:"sessions"`
			Disabled map[string]bool                            `json:"disabled"`
		}
		if err := p.load(stateRefresh, &state); err != nil {
			return err
		}
		if state.Sessions != nil {
			s.AuthService.refresh.Restore(state.Sessions, state.Disabled)
		}
	}
	if s.OIDC != nil {
		var state oidcState
		if err := p.load(stateOIDC, &state); err != nil {
			return err
		}
		if state.Providers != nil {
			s.OIDC.mu.Lock()
			s.OIDC.providers = state.Providers
			s.OIDC.mu.Unlock()
			s.OIDC.states.Restore(state.States)
		}
	}
	if s.AuditQuery != nil {
		var events []AuditEvent
		if err := p.load(stateAudit, &events); err != nil {
			return err
		}
		if events != nil {
			s.AuditQuery.mu.Lock()
			s.AuditQuery.events = events
			s.AuditQuery.mu.Unlock()
		}
	}
	if s.Operations != nil {
		var state operationsState
		if err := p.load(stateOperations, &state); err != nil {
			return err
		}
		if state.Tasks != nil {
			s.Operations.mu.Lock()
			s.Operations.tasks, s.Operations.schedules = state.Tasks, state.Schedules
			s.Operations.nextTaskID, s.Operations.nextScheduleID = state.NextTaskID, state.NextScheduleID
			s.Operations.mu.Unlock()
		}
	}
	if s.Infrastructure != nil {
		var state infrastructureState
		if err := p.load(stateInfrastructure, &state); err != nil {
			return err
		}
		if state.Runners != nil {
			enrollments := make(map[string]*enrollment, len(state.Enrollments))
			for key, value := range state.Enrollments {
				item := value
				enrollments[key] = &item
			}
			s.Infrastructure.mu.Lock()
			s.Infrastructure.runners, s.Infrastructure.resources, s.Infrastructure.enrollments, s.Infrastructure.next = state.Runners, state.Resources, enrollments, state.Next
			s.Infrastructure.mu.Unlock()
		}
	}
	if s.Runs != nil {
		var state runsState
		if err := p.load(stateRuns, &state); err != nil {
			return err
		}
		if state.Runs != nil {
			s.Runs.mu.Lock()
			s.Runs.runs, s.Runs.logs, s.Runs.next = state.Runs, state.Logs, state.Next
			s.Runs.mu.Unlock()
		}
	}
	return nil
}

func (p *Persistence) Save(s Server) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.AuthService != nil {
		s.AuthService.mu.RLock()
		passwordEnabled := s.AuthService.passwordEnabled
		registrationEnabled := s.AuthService.registrationEnabled
		defaultRole := s.AuthService.defaultRole
		systemAdminEmails := s.AuthService.systemAdminEmails
		s.AuthService.mu.RUnlock()
		if err := p.config.Set(context.Background(), "ENABLE_PASSWORD_LOGIN", passwordEnabled); err != nil {
			return err
		}
		if err := p.config.Set(context.Background(), "ENABLE_PASSWORD_REGISTRATION", registrationEnabled); err != nil {
			return err
		}
		if err := p.config.Set(context.Background(), "DEFAULT_ROLE_ID", defaultRole); err != nil {
			return err
		}
		systemAdmins := make([]string, 0, len(systemAdminEmails))
		for email := range systemAdminEmails {
			systemAdmins = append(systemAdmins, email)
		}
		sort.Strings(systemAdmins)
		if err := p.config.Set(context.Background(), "GLYPHFLOW_SYSTEM_ADMINS", systemAdmins); err != nil {
			return err
		}
	}
	if s.Sessions != nil {
		s.Sessions.mu.RLock()
		state := sessionState{Sessions: s.Sessions.sessions}
		s.Sessions.mu.RUnlock()
		if err := p.save(stateSessions, state); err != nil {
			return err
		}
	}
	if s.AuthService != nil && s.AuthService.refresh != nil {
		sessions, disabled := s.AuthService.refresh.Snapshot()
		if err := p.save(stateRefresh, struct {
			Sessions map[string]platform.RefreshSessionSnapshot `json:"sessions"`
			Disabled map[string]bool                            `json:"disabled"`
		}{sessions, disabled}); err != nil {
			return err
		}
	}
	if s.OIDC != nil {
		s.OIDC.mu.RLock()
		state := oidcState{Providers: s.OIDC.providers}
		s.OIDC.mu.RUnlock()
		state.States = s.OIDC.states.Snapshot()
		if err := p.save(stateOIDC, state); err != nil {
			return err
		}
	}
	if s.AuditQuery != nil {
		s.AuditQuery.mu.RLock()
		events := append([]AuditEvent(nil), s.AuditQuery.events...)
		s.AuditQuery.mu.RUnlock()
		if err := p.save(stateAudit, events); err != nil {
			return err
		}
	}
	if s.Operations != nil {
		s.Operations.mu.RLock()
		state := operationsState{Tasks: s.Operations.tasks, Schedules: s.Operations.schedules, NextTaskID: s.Operations.nextTaskID, NextScheduleID: s.Operations.nextScheduleID}
		s.Operations.mu.RUnlock()
		if err := p.save(stateOperations, state); err != nil {
			return err
		}
	}
	if s.Infrastructure != nil {
		s.Infrastructure.mu.RLock()
		enrollments := make(map[string]enrollment, len(s.Infrastructure.enrollments))
		for key, value := range s.Infrastructure.enrollments {
			enrollments[key] = *value
		}
		state := infrastructureState{Runners: s.Infrastructure.runners, Resources: s.Infrastructure.resources, Enrollments: enrollments, Next: s.Infrastructure.next}
		s.Infrastructure.mu.RUnlock()
		if err := p.save(stateInfrastructure, state); err != nil {
			return err
		}
	}
	if s.Runs != nil {
		s.Runs.mu.RLock()
		state := runsState{Runs: s.Runs.runs, Logs: s.Runs.logs, Next: s.Runs.next}
		s.Runs.mu.RUnlock()
		if err := p.save(stateRuns, state); err != nil {
			return err
		}
	}
	return nil
}

func (p *Persistence) Wrap(next http.Handler, s Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if err := p.Save(s); err != nil {
			log.Printf("persist application state: %v", err)
		}
	})
}

func (p *Persistence) load(name string, target any) error {
	if strings.HasPrefix(name, "state.") {
		return nil
	}
	_, err := p.config.Get(context.Background(), name, target)
	return err
}

func (p *Persistence) save(name string, value any) error {
	if strings.HasPrefix(name, "state.") {
		return nil
	}
	return p.config.Set(context.Background(), name, value)
}

func (p *Persistence) InitializeEnvironment(values map[string]any) error {
	for name, value := range values {
		if err := p.config.SetIfAbsent(context.Background(), name, value); err != nil {
			return err
		}
	}
	return nil
}
