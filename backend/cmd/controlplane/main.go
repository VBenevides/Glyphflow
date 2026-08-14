package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VBenevides/Glyphflow/backend/internal/api"
	"github.com/VBenevides/Glyphflow/backend/internal/config"
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
	persistence := api.NewPersistence(configStore)
	if err := persistence.InitializeEnvironment(map[string]any{
		"DATABASE_URL":                 cfg.DatabaseURL,
		"NATS_URL":                     cfg.NATSURL,
		"WEB_ORIGIN":                   cfg.WebOrigin,
		"MAX_MESSAGE_BYTES":            cfg.MaxMessageBytes,
		"GLYPHFLOW_BOOTSTRAP_EMAIL":    cfg.BootstrapUsername,
		"GLYPHFLOW_SYSTEM_ADMINS":      cfg.SystemAdminEmails,
		"ENABLE_PASSWORD_LOGIN":        cfg.PasswordLoginEnabled,
		"ENABLE_PASSWORD_REGISTRATION": cfg.PasswordRegistrationEnabled,
		"DEFAULT_ROLE_ID":              cfg.DefaultRoleID,
	}); err != nil {
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
	if err := authService.AddRole("admin", platform.PermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.AddRole("user", platform.UserPermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.SetDefaultRoleID(cfg.DefaultRoleID); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	oidcService := api.NewOIDCService()
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
	application := api.Server{AuthService: authService, AuthAdmin: &api.AuthAdminService{Auth: authService, OIDC: oidcService, Sessions: authService.SessionManager()}, Sessions: authService.SessionManager(), OIDC: oidcService, Roles: roles, Auth: authService.Authenticator(), Permissions: authService.Permissions, CSRFOrigin: cfg.WebOrigin, Operations: api.NewOperationsService(), Runs: api.NewRunService(), Infrastructure: api.NewInfrastructureService(), AuditQuery: api.NewAuditQueryService(), Persistence: persistence, Ready: func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return err
		}
		return nil
	}}
	if err := persistence.Restore(application); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	if err := persistence.Save(application); err != nil {
		db.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
	defer func() { jetstream.Close(); db.Close() }()
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
		if err := persistence.Save(application); err != nil {
			fmt.Fprintln(os.Stderr, "persist application state during shutdown:", err)
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Println("Glyphflow control plane")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
