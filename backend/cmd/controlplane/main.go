package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/VBenevides/Glyphflow/backend"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VBenevides/Glyphflow/backend/internal/api"
	"github.com/VBenevides/Glyphflow/backend/internal/config"
	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv(config.ControlPlane)
	if err != nil {
		return err
	}
	for _, origin := range cfg.CORSOrigins {
		if origin != "*" {
			continue
		}
		fmt.Fprintln(os.Stderr, `WARNING: CORS set to "*", this is NOT RECOMMENDED for production environments`)
		break
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return err
	}
	if err := store.ApplyMigrations(ctx, db, "migrations"); err != nil {
		return err
	}
	configStore := store.NewConfigStore(db)
	for name, value := range map[string]any{
		"WEB_ORIGIN":                   cfg.WebOrigin,
		"MAX_MESSAGE_BYTES":            cfg.MaxMessageBytes,
		"GLYPHFLOW_BOOTSTRAP_EMAIL":    cfg.BootstrapUsername,
		"GLYPHFLOW_SYSTEM_ADMINS":      cfg.SystemAdminEmails,
		"ENABLE_PASSWORD_LOGIN":        cfg.PasswordLoginEnabled,
		"ENABLE_PASSWORD_REGISTRATION": cfg.PasswordRegistrationEnabled,
		"DEFAULT_ROLE_ID":              cfg.DefaultRoleID,
		"LOCKDOWN_SCHEDULER":           false,
	} {
		if err := configStore.SetIfAbsent(ctx, name, value); err != nil {
			return err
		}
	}
	signingKeyPath := ""
	if cfg.Environment == "development" && cfg.ControlPlaneSigningPrivateKey == "" {
		signingKeyPath = filepath.Join(cfg.DataDir, "control-plane-signing.key")
	}
	signingKey, err := loadControlPlaneSigningKey(cfg.ControlPlaneSigningPrivateKey, signingKeyPath)
	if err != nil {
		return err
	}
	if len(cfg.SystemAdminEmails) == 0 {
		var storedSystemAdminEmails []string
		if found, err := configStore.Get(ctx, "GLYPHFLOW_SYSTEM_ADMINS", &storedSystemAdminEmails); err != nil {
			return err
		} else if found {
			cfg.SystemAdminEmails = storedSystemAdminEmails
		}
	}
	var storedPasswordLogin, storedPasswordRegistration, storedLockdownScheduler bool
	var storedDefaultRoleID string
	if found, err := configStore.Get(ctx, "ENABLE_PASSWORD_LOGIN", &storedPasswordLogin); err != nil {
		return err
	} else if found {
		cfg.PasswordLoginEnabled = storedPasswordLogin
	}
	if found, err := configStore.Get(ctx, "ENABLE_PASSWORD_REGISTRATION", &storedPasswordRegistration); err != nil {
		return err
	} else if found {
		cfg.PasswordRegistrationEnabled = storedPasswordRegistration
	}
	if found, err := configStore.Get(ctx, "DEFAULT_ROLE_ID", &storedDefaultRoleID); err != nil {
		return err
	} else if found && strings.TrimSpace(storedDefaultRoleID) != "" {
		cfg.DefaultRoleID = storedDefaultRoleID
	}
	if found, err := configStore.Get(ctx, "LOCKDOWN_SCHEDULER", &storedLockdownScheduler); err != nil {
		return err
	} else if found {
		cfg.LockdownScheduler = storedLockdownScheduler
	}
	authService, err := api.NewAuthService(cfg.AccessTokenSecret, cfg.PasswordLoginEnabled, cfg.PasswordRegistrationEnabled, []byte(cfg.PasswordPepper))
	if err != nil {
		return err
	}
	authService.SetUserRepository(store.NewUserRepository(db))
	roleRepository := store.NewRoleRepository(db)
	authService.SetRoleRepository(roleRepository)
	authService.SetConfigStore(configStore)
	authService.SetLockdownScheduler(cfg.LockdownScheduler)
	sessionRepository := store.NewSessionRepository(db)
	authService.SetSessionRepository(sessionRepository)
	authService.SetSSORepository(store.NewOIDCProviderRepository(db))
	if err := authService.AddRole("admin", platform.PermissionCatalog...); err != nil {
		return err
	}
	if err := authService.AddRole("user", platform.UserPermissionCatalog...); err != nil {
		return err
	}
	if err := authService.AddRole("operator", platform.OperatorPermissionCatalog...); err != nil {
		return err
	}
	if err := authService.SetDefaultRoleID(cfg.DefaultRoleID); err != nil {
		return err
	}
	oidcService := api.NewOIDCService()
	oidcService.SetDefaultCallback(strings.TrimRight(cfg.WebOrigin, "/") + "/api/v1/auth/oidc/callback")
	oidcService.SetRepository(store.NewOIDCProviderRepository(db))
	oidcService.SetStateRepository(store.NewOIDCAuthorizationStateRepository(db), []byte(cfg.AccessTokenSecret))
	roles := api.NewRoleAdminService()
	roles.SetRepository(roleRepository)
	if err := roles.Seed("admin", platform.PermissionCatalog); err != nil {
		return err
	}
	if err := roles.Seed("user", platform.UserPermissionCatalog); err != nil {
		return err
	}
	if err := roles.Seed("operator", platform.OperatorPermissionCatalog); err != nil {
		return err
	}
	operations := api.NewOperationsService()
	operations.SetTaskRepository(store.NewTaskRepository(db))
	scheduleRepository := store.NewScheduleRepository(db)
	operations.SetScheduleRepository(scheduleRepository)
	globalVariables := api.NewGlobalVariableService()
	globalVariables.SetRepository(store.NewGlobalVariableRepository(db))
	runs := api.NewRunService()
	runRepository := store.NewRunRepository(db)
	runs.SetRepository(runRepository)
	runnerRepository := store.NewRunnerRepository(db)
	if err := runnerRepository.EnsurePool(ctx, "default", "default"); err != nil {
		return err
	}
	infrastructure := api.NewInfrastructureService()
	infrastructure.SetRunnerRepository(runnerRepository)
	infrastructure.SetResourceRepository(store.NewResourceRepository(db))
	infrastructure.SetRunnerBinaryDirectory(os.Getenv("RUNNER_BINARIES_DIR"))
	runnerNATSURL := strings.TrimSpace(os.Getenv("RUNNER_NATS_URL"))
	if runnerNATSURL == "" {
		runnerNATSURL = cfg.NATSURL
	}
	runnerControlPlaneURL := strings.TrimSpace(os.Getenv("RUNNER_CONTROL_PLANE_URL"))
	if runnerControlPlaneURL == "" {
		runnerControlPlaneURL = cfg.WebOrigin
	}
	infrastructure.SetRunnerArtifactConfig(runnerNATSURL, cfg.MaxMessageBytes)
	infrastructure.SetRunnerControlPlaneURL(runnerControlPlaneURL)
	infrastructure.SetRunnerEndpointPolicy(cfg.AllowInsecureTransport)
	infrastructure.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(signingKey.Public.PublicKey))
	audit := api.NewAuditQueryService()
	audit.SetRepository(store.NewAuditRepository(db))
	metrics := new(platform.Metrics)
	logger := &platform.Logger{Out: os.Stderr}
	audit.SetAppendFailureHandler(func(event api.AuditEvent, err error) {
		metrics.AuditAppendErrors.Add(1)
		_ = logger.Event("audit.append_failed", map[string]string{"id": event.ID, "actor": event.Actor, "error": err.Error(), "count": strconv.FormatUint(metrics.AuditAppendErrors.Load(), 10)})
	})
	health := controlplane.NewHealth("session-cleanup", "heartbeat", "dispatcher", "start-claim", "scheduler")
	application := api.Server{AuthService: authService, AuthAdmin: &api.AuthAdminService{Auth: authService, OIDC: oidcService, Sessions: authService.SessionManager()}, Sessions: authService.SessionManager(), OIDC: oidcService, Roles: roles, Auth: authService.Authenticator(), Permissions: authService.Permissions, CSRFOrigin: cfg.WebOrigin, CSRFOrigins: cfg.CSRFOrigins, CORSOrigins: cfg.CORSOrigins, Operations: operations, Runs: runs, Infrastructure: infrastructure, AuditQuery: audit, ExitCodes: store.NewExitCodeRepository(db), GlobalVariables: globalVariables, Ready: func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return err
		}
		return nil
	}, RequireDurableRepositories: true}
	if err := application.ValidateDurableRepositories(); err != nil {
		return err
	}
	if err := authService.SetSystemAdminEmails(cfg.SystemAdminEmails); err != nil {
		return err
	}
	if cfg.BootstrapUsername != "" && cfg.BootstrapPassword != "" {
		if _, err := authService.EnsureBootstrap(cfg.BootstrapUsername, cfg.BootstrapPassword, "", ""); err != nil {
			return err
		}
	}
	var jetstream *queue.JetStream
	if strings.HasPrefix(cfg.NATSURL, "tls://") {
		jetstream, err = queue.ConnectJetStreamTLS(cfg.NATSURL, queue.TLSConfig{CertificateFile: cfg.NATSCertFile, KeyFile: cfg.NATSKeyFile, CAFile: cfg.NATSCAFile})
	} else {
		jetstream, err = queue.ConnectJetStreamPlain(cfg.NATSURL)
	}
	if err != nil {
		return err
	}
	infrastructure.SetRunnerCapacityPublisher(jetstream, signingKey)
	defer jetstream.Close()
	go func() {
		const sessionRetention = 14 * 24 * time.Hour
		cleanup := func() {
			if err := sessionRepository.DeleteOlderThan(ctx, time.Now().UTC().Add(-sessionRetention)); err != nil && ctx.Err() == nil {
				health.MarkFailed("session-cleanup", err)
				fmt.Fprintln(os.Stderr, "session cleanup:", err)
				return
			}
			health.MarkHealthy("session-cleanup")
		}
		cleanup()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cleanup()
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		for ctx.Err() == nil {
			health.MarkHealthy("heartbeat")
			if err := controlplane.RunRunnerHeartbeatMonitor(ctx, jetstream, runnerRepository, 30*time.Second, 10*time.Second); err != nil && ctx.Err() == nil {
				health.MarkFailed("heartbeat", err)
				fmt.Fprintln(os.Stderr, "runner heartbeat monitor:", err)
				time.Sleep(time.Second)
			}
		}
	}()
	go func() {
		for ctx.Err() == nil {
			health.MarkHealthy("dispatcher")
			if err := controlplane.RunDispatcher(ctx, jetstream, runRepository, runnerRepository, signingKey, 500*time.Millisecond); err != nil && ctx.Err() == nil {
				health.MarkFailed("dispatcher", err)
				fmt.Fprintln(os.Stderr, "run dispatcher:", err)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() {
		for ctx.Err() == nil {
			health.MarkHealthy("start-claim")
			if err := controlplane.RunStartClaimServer(ctx, jetstream, runRepository, runnerRepository, signingKey); err != nil && ctx.Err() == nil {
				health.MarkFailed("start-claim", err)
				fmt.Fprintln(os.Stderr, "start claim server:", err)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() {
		for ctx.Err() == nil {
			health.MarkHealthy("scheduler")
			if err := controlplane.RunScheduler(ctx, scheduleRepository, 500*time.Millisecond); err != nil && ctx.Err() == nil {
				health.MarkFailed("scheduler", err)
				fmt.Fprintln(os.Stderr, "schedule runner:", err)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	server := &http.Server{
		Addr: ":8080",
		Handler: func() http.Handler {
			application.Ready = func(ctx context.Context) error {
				if err := db.Ping(ctx); err != nil {
					return err
				}
				if jetstream == nil {
					return fmt.Errorf("NATS is not connected")
				}
				return health.Ready()
			}
			return application.Handler()
		}(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("Glyphflow control plane v%s\n", backend.Version)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
