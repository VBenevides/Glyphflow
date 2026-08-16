package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
	cfg, err := config.FromEnv(config.ControlPlane)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := store.ApplyMigrations(ctx, db, "migrations"); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
	} {
		if err := configStore.SetIfAbsent(ctx, name, value); err != nil {
			db.Close()
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	signingKey, err := loadControlPlaneSigningKey(cfg.ControlPlaneSigningPrivateKey)
	if err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(cfg.SystemAdminEmails) == 0 {
		var storedSystemAdminEmails []string
		if found, err := configStore.Get(ctx, "GLYPHFLOW_SYSTEM_ADMINS", &storedSystemAdminEmails); err != nil {
			db.Close()
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if found {
			cfg.SystemAdminEmails = storedSystemAdminEmails
		}
	}
	var storedPasswordLogin, storedPasswordRegistration bool
	var storedDefaultRoleID string
	if found, err := configStore.Get(ctx, "ENABLE_PASSWORD_LOGIN", &storedPasswordLogin); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else if found {
		cfg.PasswordLoginEnabled = storedPasswordLogin
	}
	if found, err := configStore.Get(ctx, "ENABLE_PASSWORD_REGISTRATION", &storedPasswordRegistration); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else if found {
		cfg.PasswordRegistrationEnabled = storedPasswordRegistration
	}
	if found, err := configStore.Get(ctx, "DEFAULT_ROLE_ID", &storedDefaultRoleID); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else if found && strings.TrimSpace(storedDefaultRoleID) != "" {
		cfg.DefaultRoleID = storedDefaultRoleID
	}
	authService, err := api.NewAuthService(cfg.AccessTokenSecret, cfg.PasswordLoginEnabled, cfg.PasswordRegistrationEnabled, []byte(cfg.PasswordPepper))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	authService.SetUserRepository(store.NewUserRepository(db))
	roleRepository := store.NewRoleRepository(db)
	authService.SetRoleRepository(roleRepository)
	authService.SetConfigStore(configStore)
	authService.SetSessionRepository(store.NewSessionRepository(db))
	authService.SetSSORepository(store.NewOIDCProviderRepository(db))
	if err := authService.AddRole("admin", platform.PermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.AddRole("user", platform.UserPermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.AddRole("operator", platform.OperatorPermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.SetDefaultRoleID(cfg.DefaultRoleID); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	oidcService := api.NewOIDCService()
	oidcService.SetDefaultCallback(strings.TrimRight(cfg.WebOrigin, "/") + "/api/v1/auth/oidc/callback")
	oidcService.SetRepository(store.NewOIDCProviderRepository(db))
	oidcService.SetStateRepository(store.NewOIDCAuthorizationStateRepository(db), []byte(cfg.AccessTokenSecret))
	roles := api.NewRoleAdminService()
	roles.SetRepository(roleRepository)
	if err := roles.Seed("admin", platform.PermissionCatalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := roles.Seed("user", platform.UserPermissionCatalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := roles.Seed("operator", platform.OperatorPermissionCatalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	infrastructure := api.NewInfrastructureService()
	infrastructure.SetRunnerRepository(runnerRepository)
	infrastructure.SetResourceRepository(store.NewResourceRepository(db))
	infrastructure.SetRunnerBinaryDirectory(os.Getenv("RUNNER_BINARIES_DIR"))
	infrastructure.SetRunnerArtifactConfig(cfg.NATSURL, cfg.MaxMessageBytes)
	infrastructure.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(signingKey.Public.PublicKey))
	audit := api.NewAuditQueryService()
	audit.SetRepository(store.NewAuditRepository(db))
	metrics := new(platform.Metrics)
	logger := &platform.Logger{Out: os.Stderr}
	audit.SetAppendFailureHandler(func(event api.AuditEvent, err error) {
		metrics.AuditAppendErrors.Add(1)
		_ = logger.Event("audit.append_failed", map[string]string{"id": event.ID, "actor": event.Actor, "error": err.Error(), "count": strconv.FormatUint(metrics.AuditAppendErrors.Load(), 10)})
	})
	application := api.Server{AuthService: authService, AuthAdmin: &api.AuthAdminService{Auth: authService, OIDC: oidcService, Sessions: authService.SessionManager()}, Sessions: authService.SessionManager(), OIDC: oidcService, Roles: roles, Auth: authService.Authenticator(), Permissions: authService.Permissions, CSRFOrigin: cfg.WebOrigin, Operations: operations, Runs: runs, Infrastructure: infrastructure, AuditQuery: audit, ExitCodes: store.NewExitCodeRepository(db), GlobalVariables: globalVariables, Ready: func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return err
		}
		return nil
	}}
	if err := authService.SetSystemAdminEmails(cfg.SystemAdminEmails); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.BootstrapUsername != "" && cfg.BootstrapPassword != "" {
		if _, err := authService.EnsureBootstrap(cfg.BootstrapUsername, cfg.BootstrapPassword, "", ""); err != nil {
			db.Close()
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	var jetstream *queue.JetStream
	if strings.HasPrefix(cfg.NATSURL, "tls://") {
		jetstream, err = queue.ConnectJetStreamTLS(cfg.NATSURL, queue.TLSConfig{CertificateFile: cfg.NATSCertFile, KeyFile: cfg.NATSKeyFile, CAFile: cfg.NATSCAFile})
	} else {
		jetstream, err = queue.ConnectJetStreamPlain(cfg.NATSURL)
	}
	if err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	infrastructure.SetRunnerCapacityPublisher(jetstream, signingKey)
	defer func() { jetstream.Close(); db.Close() }()
	go func() {
		for ctx.Err() == nil {
			if err := controlplane.RunRunnerHeartbeatMonitor(ctx, jetstream, runnerRepository, 30*time.Second, 10*time.Second); err != nil && ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, "runner heartbeat monitor:", err)
				time.Sleep(time.Second)
			}
		}
	}()
	go func() {
		for ctx.Err() == nil {
			if err := controlplane.RunDispatcher(ctx, jetstream, runRepository, runnerRepository, signingKey, 500*time.Millisecond); err != nil && ctx.Err() == nil {
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
			if err := controlplane.RunScheduler(ctx, scheduleRepository, 500*time.Millisecond); err != nil && ctx.Err() == nil {
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
				return nil
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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
