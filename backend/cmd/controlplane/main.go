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
	authService, err := api.NewAuthService(cfg.AccessTokenSecret, cfg.PasswordLoginEnabled, cfg.PasswordRegistrationEnabled, []byte(cfg.PasswordPepper))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.AddRole("admin", platform.PermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.AddRole("user", platform.UserPermissionCatalog...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := authService.SetSystemAdminEmails(cfg.SystemAdminEmails); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	authService.SetDefaultRole("user")
	if cfg.BootstrapUsername != "" && cfg.BootstrapPassword != "" {
		if _, err := authService.EnsureBootstrap(cfg.BootstrapUsername, cfg.BootstrapPassword, "", ""); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	oidcService := api.NewOIDCService()
	roles := api.NewRoleAdminService()
	if err := roles.Seed("admin", platform.PermissionCatalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := roles.Seed("user", platform.UserPermissionCatalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
		Handler: api.Server{AuthService: authService, AuthAdmin: &api.AuthAdminService{Auth: authService, OIDC: oidcService, Sessions: authService.SessionManager()}, OIDC: oidcService, Roles: roles, Auth: authService.Authenticator(), Permissions: authService.Permissions, CSRFOrigin: cfg.WebOrigin, Ready: func(ctx context.Context) error {
			if err := db.Ping(ctx); err != nil {
				return err
			}
			if jetstream == nil {
				return fmt.Errorf("NATS is not connected")
			}
			return nil
		}}.Handler(),
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
	fmt.Println("Glyphflow control plane")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
