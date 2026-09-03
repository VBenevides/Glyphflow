package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
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
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

const (
	healthSessionCleanup   = "session-cleanup"
	healthHeartbeat        = "heartbeat"
	healthDispatcher       = "dispatcher"
	healthStartClaim       = "start-claim"
	healthSecretDelivery   = "secret-delivery"
	healthScheduler        = "scheduler"
	retentionCleanupFailed = "retention.cleanup_failed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type controlPlaneDatabase struct {
	db            any
	pingDatabase  func(context.Context) error
	closeDatabase func()
}

type controlPlaneAuth struct {
	authService               *api.AuthService
	oidcService               *api.OIDCService
	roles                     *api.RoleAdminService
	roleRepository            store.RoleRepository
	sessionRepository         store.SessionRepository
	ssoRepository             store.OIDCProviderRepository
	encryptedSecretRepository store.EncryptedSecretRepository
}

type controlPlaneServices struct {
	operations                *api.OperationsService
	runs                      *api.RunService
	infrastructure            *api.InfrastructureService
	audit                     *api.AuditQueryService
	globalVariables           *api.GlobalVariableService
	scheduleRepository        *store.ScheduleStore
	retentionRepository       *store.RetentionStore
	storagePressure           func(context.Context) (platform.StoragePressure, error)
	runRepository             *store.RunStore
	runnerRepository          *store.RunnerStore
	projectionService         *controlplane.ProjectionService
	metrics                   *platform.Metrics
	logger                    *platform.Logger
	health                    *controlplane.Health
	deadLetterRepository      *store.DeadLetterStore
	sessionRepository         store.SessionRepository
	encryptedSecretRepository store.EncryptedSecretRepository
}

type controlPlaneRuntime struct {
	application  api.Server
	signingKey   protocol.SigningKey
	pingDatabase func(context.Context) error
	auth         controlPlaneAuth
	services     controlPlaneServices
}

func run() error {
	cfg, err := config.FromEnv(config.ControlPlane)
	if err != nil {
		return err
	}
	warnOnWildcardCORS(cfg.CORSOrigins)
	ctx, stop := notifyContext()
	defer stop()

	database, err := openControlPlaneDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer database.closeDatabase()
	if cfg.DatabaseMode == "sqlite" {
		if err := waitForDatabase(ctx, database.pingDatabase, time.Second); err != nil {
			return err
		}
	}
	if cfg.NATSMode == "embedded" {
		server, err := startEmbeddedNATS(cfg.DataDir)
		if err != nil {
			return err
		}
		defer server.Shutdown()
		cfg.NATSURL = server.ClientURL()
	}

	runtime, err := initializeControlPlaneRuntime(ctx, &cfg, database)
	if err != nil {
		return err
	}
	jetstream, err := connectControlPlaneJetStream(ctx, cfg)
	if err != nil {
		return err
	}
	defer jetstream.Close()
	deadLetterSignals := configureControlPlaneJetStream(&runtime, jetstream)
	startControlPlaneWorkers(ctx, cfg, &runtime, jetstream)
	configureControlPlaneSystemMetrics(ctx, &runtime, jetstream, deadLetterSignals)
	return serveControlPlane(ctx, runtime.application)
}

func warnOnWildcardCORS(origins []string) {
	for _, origin := range origins {
		if origin != "*" {
			continue
		}
		fmt.Fprintln(os.Stderr, "WARNING: CORS set to \"*\", this is NOT RECOMMENDED for production environments")
		break
	}
}

func openControlPlaneDatabase(ctx context.Context, cfg config.Config) (controlPlaneDatabase, error) {
	if cfg.DatabaseMode == "sqlite" {
		sqliteDB, err := store.OpenSQLite(cfg.DatabaseURL)
		if err != nil {
			return controlPlaneDatabase{}, err
		}
		closeDatabase := func() { _ = sqliteDB.Close() }
		if err := store.ApplySQLiteMigrations(ctx, sqliteDB, "migrations"); err != nil {
			closeDatabase()
			return controlPlaneDatabase{}, err
		}
		return controlPlaneDatabase{db: sqliteDB, pingDatabase: sqliteDB.PingContext, closeDatabase: closeDatabase}, nil
	}

	postgresDB, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return controlPlaneDatabase{}, err
	}
	closeDatabase := func() { closeControlPlaneDB(postgresDB) }
	if err := waitForDatabase(ctx, postgresDB.Ping, time.Second); err != nil {
		closeDatabase()
		return controlPlaneDatabase{}, err
	}
	if err := store.ApplyMigrations(ctx, postgresDB, "migrations"); err != nil {
		closeDatabase()
		return controlPlaneDatabase{}, err
	}
	return controlPlaneDatabase{db: postgresDB, pingDatabase: postgresDB.Ping, closeDatabase: closeDatabase}, nil
}

func initializeControlPlaneRuntime(ctx context.Context, cfg *config.Config, database controlPlaneDatabase) (controlPlaneRuntime, error) {
	configStore, signingKey, err := initializeControlPlaneConfig(ctx, cfg, database.db)
	if err != nil {
		return controlPlaneRuntime{}, err
	}
	auth, err := initializeControlPlaneAuth(*cfg, database.db, configStore)
	if err != nil {
		return controlPlaneRuntime{}, err
	}
	services, err := initializeControlPlaneServices(ctx, *cfg, database.db, auth, signingKey)
	if err != nil {
		return controlPlaneRuntime{}, err
	}
	application, err := buildControlPlaneApplication(*cfg, database, auth, services)
	if err != nil {
		return controlPlaneRuntime{}, err
	}
	return controlPlaneRuntime{application: application, signingKey: signingKey, pingDatabase: database.pingDatabase, auth: auth, services: services}, nil
}

func initializeControlPlaneConfig(ctx context.Context, cfg *config.Config, db any) (*store.ConfigStore, protocol.SigningKey, error) {
	configStore := store.NewConfigStore(db)
	if err := seedControlPlaneConfig(ctx, cfg, configStore); err != nil {
		return nil, protocol.SigningKey{}, err
	}
	signingKeyPath := ""
	if cfg.Environment == "development" && cfg.ControlPlaneSigningPrivateKey == "" {
		signingKeyPath = filepath.Join(cfg.DataDir, "control-plane-signing.key")
	}
	signingKey, err := loadControlPlaneSigningKey(cfg.ControlPlaneSigningPrivateKey, signingKeyPath)
	if err != nil {
		return nil, protocol.SigningKey{}, err
	}
	if err := loadStoredControlPlaneConfig(ctx, cfg, configStore); err != nil {
		return nil, protocol.SigningKey{}, err
	}
	return configStore, signingKey, nil
}

func seedControlPlaneConfig(ctx context.Context, cfg *config.Config, configStore *store.ConfigStore) error {
	for name, value := range map[string]any{
		"WEB_ORIGIN":                   cfg.WebOrigin,
		"MAX_MESSAGE_BYTES":            cfg.MaxMessageBytes,
		"GLYPHFLOW_BOOTSTRAP_EMAIL":    cfg.BootstrapUsername,
		"GLYPHFLOW_SYSTEM_ADMINS":      cfg.SystemAdminEmails,
		"ENABLE_PASSWORD_LOGIN":        cfg.PasswordLoginEnabled,
		"ENABLE_PASSWORD_REGISTRATION": cfg.PasswordRegistrationEnabled,
		"REQUIRE_USER_APPROVAL":        cfg.RequireUserApproval,
		"DEFAULT_ROLE_ID":              cfg.DefaultRoleID,
		"LOCKDOWN_SCHEDULER":           false,
	} {
		if err := configStore.SetIfAbsent(ctx, name, value); err != nil {
			return err
		}
	}
	return nil
}

func loadStoredControlPlaneConfig(ctx context.Context, cfg *config.Config, configStore *store.ConfigStore) error {
	if len(cfg.SystemAdminEmails) == 0 {
		var storedSystemAdminEmails []string
		found, err := configStore.Get(ctx, "GLYPHFLOW_SYSTEM_ADMINS", &storedSystemAdminEmails)
		if err != nil {
			return err
		}
		if found {
			cfg.SystemAdminEmails = storedSystemAdminEmails
		}
	}
	var storedPasswordLogin, storedPasswordRegistration, storedUserApproval, storedLockdownScheduler bool
	var storedDefaultRoleID string
	found, err := configStore.Get(ctx, "ENABLE_PASSWORD_LOGIN", &storedPasswordLogin)
	if err != nil {
		return err
	}
	if found {
		cfg.PasswordLoginEnabled = storedPasswordLogin
	}
	found, err = configStore.Get(ctx, "ENABLE_PASSWORD_REGISTRATION", &storedPasswordRegistration)
	if err != nil {
		return err
	}
	if found {
		cfg.PasswordRegistrationEnabled = storedPasswordRegistration
	}
	found, err = configStore.Get(ctx, "REQUIRE_USER_APPROVAL", &storedUserApproval)
	if err != nil {
		return err
	}
	if found {
		cfg.RequireUserApproval = storedUserApproval
	}
	found, err = configStore.Get(ctx, "DEFAULT_ROLE_ID", &storedDefaultRoleID)
	if err != nil {
		return err
	}
	if found && strings.TrimSpace(storedDefaultRoleID) != "" {
		cfg.DefaultRoleID = storedDefaultRoleID
	}
	found, err = configStore.Get(ctx, "LOCKDOWN_SCHEDULER", &storedLockdownScheduler)
	if err != nil {
		return err
	}
	if found {
		cfg.LockdownScheduler = storedLockdownScheduler
	}
	return nil
}

func initializeControlPlaneAuth(cfg config.Config, db any, configStore *store.ConfigStore) (controlPlaneAuth, error) {
	authService, err := api.NewAuthService(cfg.AccessTokenSecret, cfg.PasswordLoginEnabled, cfg.PasswordRegistrationEnabled, []byte(cfg.PasswordPepper))
	if err != nil {
		return controlPlaneAuth{}, err
	}
	authService.SetUserRepository(store.NewUserRepository(db))
	roleRepository := store.NewRoleRepository(db)
	authService.SetRoleRepository(roleRepository)
	authService.SetConfigStore(configStore)
	authService.SetUserApprovalRequired(cfg.RequireUserApproval)
	authService.SetLockdownScheduler(cfg.LockdownScheduler)
	sessionRepository := store.NewSessionRepository(db)
	authService.SetSessionRepository(sessionRepository)
	ssoRepository := store.NewOIDCProviderRepository(db)
	encryptedSecretRepository := store.NewEncryptedSecretRepository(db)
	authService.SetSSORepository(ssoRepository)
	if err := seedControlPlaneAuthRoles(authService); err != nil {
		return controlPlaneAuth{}, err
	}
	if err := authService.SetDefaultRoleID(cfg.DefaultRoleID); err != nil {
		return controlPlaneAuth{}, err
	}
	oidcService := api.NewOIDCService()
	oidcService.SetAllowHTTPCallbacks(cfg.AllowInsecureTransport && (cfg.Environment == "local" || cfg.Environment == "development"))
	oidcService.SetDefaultCallback(strings.TrimRight(cfg.WebOrigin, "/") + "/api/v1/auth/oidc/callback")
	oidcService.SetRepository(ssoRepository)
	oidcService.SetSecretRepository(encryptedSecretRepository, cfg.InstallationEncryptionKey)
	oidcService.SetStateRepository(store.NewOIDCAuthorizationStateRepository(db), cfg.InstallationEncryptionKey)
	roles := api.NewRoleAdminService()
	roles.SetRepository(roleRepository)
	if err := seedControlPlaneRoleAdmin(roles); err != nil {
		return controlPlaneAuth{}, err
	}
	return controlPlaneAuth{authService: authService, oidcService: oidcService, roles: roles, roleRepository: roleRepository, sessionRepository: sessionRepository, ssoRepository: ssoRepository, encryptedSecretRepository: encryptedSecretRepository}, nil
}

func seedControlPlaneAuthRoles(authService *api.AuthService) error {
	if err := authService.AddRole("admin", platform.PermissionCatalog...); err != nil {
		return err
	}
	if err := authService.AddRole("user", platform.UserPermissionCatalog...); err != nil {
		return err
	}
	return authService.AddRole("operator", platform.OperatorPermissionCatalog...)
}

func seedControlPlaneRoleAdmin(roles *api.RoleAdminService) error {
	if err := roles.Seed("admin", platform.PermissionCatalog); err != nil {
		return err
	}
	if err := roles.Seed("user", platform.UserPermissionCatalog); err != nil {
		return err
	}
	return roles.Seed("operator", platform.OperatorPermissionCatalog)
}

func initializeControlPlaneServices(ctx context.Context, cfg config.Config, db any, auth controlPlaneAuth, signingKey protocol.SigningKey) (controlPlaneServices, error) {
	operations := api.NewOperationsService()
	taskRepository := store.NewTaskRepository(db)
	operations.SetTaskRepository(taskRepository)
	operations.SetSecretRepository(auth.encryptedSecretRepository)
	scheduleRepository := store.NewScheduleRepository(db)
	retentionRepository := store.NewRetentionRepository(db)
	var storagePressure func(context.Context) (platform.StoragePressure, error)
	if cfg.DatabaseMode == "sqlite" {
		storagePressure = store.NewSQLiteStoragePressureProvider(db.(*sql.DB), cfg.DatabaseStorageCapacityBytes)
	} else {
		storagePressure = store.NewPostgreSQLStoragePressureProvider(db.(*pgxpool.Pool), cfg.DatabaseStorageCapacityBytes)
	}
	operations.SetScheduleRepository(scheduleRepository)
	globalVariables := api.NewGlobalVariableService()
	globalVariables.SetRepository(store.NewGlobalVariableRepository(db))
	runs := api.NewRunService()
	runRepository := store.NewRunRepository(db)
	runRepository.SetStoragePressureProvider(storagePressure)
	scheduleRepository.SetStoragePressureProvider(storagePressure)
	runs.SetRepository(runRepository)
	runnerRepository := store.NewRunnerRepository(db)
	if err := runnerRepository.EnsurePool(ctx, "default", "default"); err != nil {
		return controlPlaneServices{}, err
	}
	infrastructure := api.NewInfrastructureService()
	infrastructure.SetRunnerRepository(runnerRepository)
	resourceRepository := store.NewResourceRepository(db)
	infrastructure.SetResourceRepository(resourceRepository)
	operations.SetResourceRepository(resourceRepository)
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
	projectionService := controlplane.NewProjectionService(scheduleRepository, logger)
	operations.SetScheduleProjection(projectionService)
	audit.SetAppendFailureHandler(func(event api.AuditEvent, err error) {
		metrics.AuditAppendErrors.Add(1)
		_ = logger.Event("audit.append_failed", map[string]string{"id": event.ID, "actor": event.Actor, "error": err.Error(), "count": strconv.FormatUint(metrics.AuditAppendErrors.Load(), 10)})
	})
	health := controlplane.NewHealth(healthSessionCleanup, healthHeartbeat, healthDispatcher, healthStartClaim, healthSecretDelivery, healthScheduler)
	deadLetterRepository := store.NewDeadLetterRepository(db, cfg.InstallationEncryptionKey)
	return controlPlaneServices{operations: operations, runs: runs, infrastructure: infrastructure, audit: audit, globalVariables: globalVariables, scheduleRepository: scheduleRepository, retentionRepository: retentionRepository, storagePressure: storagePressure, runRepository: runRepository, runnerRepository: runnerRepository, projectionService: projectionService, metrics: metrics, logger: logger, health: health, deadLetterRepository: deadLetterRepository, sessionRepository: auth.sessionRepository, encryptedSecretRepository: auth.encryptedSecretRepository}, nil
}

func buildControlPlaneApplication(cfg config.Config, database controlPlaneDatabase, auth controlPlaneAuth, services controlPlaneServices) (api.Server, error) {
	application := api.Server{AuthService: auth.authService, AuthAdmin: &api.AuthAdminService{Auth: auth.authService, OIDC: auth.oidcService, Sessions: auth.authService.SessionManager()}, Sessions: auth.authService.SessionManager(), OIDC: auth.oidcService, Roles: auth.roles, Auth: auth.authService.Authenticator(), Permissions: auth.authService.Permissions, Metrics: services.metrics, Logger: services.logger, CSRFOrigin: cfg.WebOrigin, CSRFOrigins: cfg.CSRFOrigins, CORSOrigins: cfg.CORSOrigins, Operations: services.operations, Runs: services.runs, Infrastructure: services.infrastructure, AuditQuery: services.audit, ExitCodes: store.NewExitCodeRepository(database.db), GlobalVariables: services.globalVariables, Secrets: api.NewSecretAdminService(auth.encryptedSecretRepository, cfg.InstallationEncryptionKey), DeadLetters: api.NewDeadLetterService(services.deadLetterRepository, nil), Ready: func(ctx context.Context) error {
		if err := database.pingDatabase(ctx); err != nil {
			return err
		}
		return nil
	}, ScheduleProjection: services.projectionService, RequireDurableRepositories: true}
	if err := application.ValidateDurableRepositories(); err != nil {
		return api.Server{}, err
	}
	if err := auth.authService.SetSystemAdminEmails(cfg.SystemAdminEmails); err != nil {
		return api.Server{}, err
	}
	if cfg.BootstrapUsername != "" && cfg.BootstrapPassword != "" {
		if _, err := auth.authService.EnsureBootstrap(cfg.BootstrapUsername, cfg.BootstrapPassword, "", ""); err != nil {
			return api.Server{}, err
		}
	}
	return application, nil
}

func connectControlPlaneJetStream(ctx context.Context, cfg config.Config) (*queue.JetStream, error) {
	if strings.HasPrefix(cfg.NATSURL, "tls://") {
		return queue.ConnectJetStreamTLSWithContext(ctx, cfg.NATSURL, queue.TLSConfig{CertificateFile: cfg.NATSCertFile, KeyFile: cfg.NATSKeyFile, CAFile: cfg.NATSCAFile})
	}
	return queue.ConnectJetStreamPlainWithContext(ctx, cfg.NATSURL)
}

func configureControlPlaneJetStream(runtime *controlPlaneRuntime, jetstream *queue.JetStream) func(context.Context) (platform.OperationalSignals, error) {
	deadLetterRepository := runtime.services.deadLetterRepository
	jetstream.SetDeadLetterSink(func(ctx context.Context, record queue.DeadLetter) error {
		return deadLetterRepository.Persist(ctx, store.DeadLetterRecord{
			RunnerID: record.RunnerID, Stream: record.Stream, Consumer: record.Consumer, Subject: record.Subject,
			MessageID: record.MessageID, Payload: record.Payload, Error: record.Error, Attempts: record.Attempts,
			FirstFailedAt: record.FirstFailedAt, LastFailedAt: record.LastFailedAt, CorrelationID: record.CorrelationID,
		})
	})
	runtime.application.DeadLetters.SetPublisher(jetstream)
	deadLetterSignals := func(ctx context.Context) (platform.OperationalSignals, error) {
		stats, err := deadLetterRepository.Stats(ctx)
		if err != nil {
			return platform.OperationalSignals{}, err
		}
		return platform.OperationalSignals{DeadLetters: platform.DeadLetterSignals{Open: stats.Open, OldestAgeSeconds: stats.OldestAgeSeconds}}, nil
	}
	runtime.services.infrastructure.SetRunnerCapacityPublisher(jetstream, runtime.signingKey)
	return deadLetterSignals
}

func startControlPlaneWorkers(ctx context.Context, cfg config.Config, runtime *controlPlaneRuntime, jetstream *queue.JetStream) {
	go runSessionCleanup(ctx, runtime.services.sessionRepository, runtime.services.health)
	go runRetentionCleanup(ctx, cfg, runtime.services)
	go runControlPlaneHeartbeat(ctx, runtime.services.health, jetstream, runtime.services.runnerRepository)
	go runControlPlaneRetryLoop(ctx, runtime.services.health, "dispatcher", "run dispatcher", func(ctx context.Context) error {
		return controlplane.RunDispatcher(ctx, jetstream, runtime.services.runRepository, runtime.services.runnerRepository, runtime.signingKey, 500*time.Millisecond)
	})
	go runControlPlaneRetryLoop(ctx, runtime.services.health, "start-claim", "start claim server", func(ctx context.Context) error {
		return controlplane.RunStartClaimServer(ctx, jetstream, runtime.services.runRepository, runtime.services.runnerRepository, runtime.signingKey)
	})
	go runControlPlaneRetryLoop(ctx, runtime.services.health, "secret-delivery", "secret delivery server", func(ctx context.Context) error {
		return controlplane.RunSecretDeliveryServer(ctx, jetstream, runtime.services.runRepository, runtime.services.encryptedSecretRepository, runtime.services.runnerRepository, runtime.signingKey, cfg.InstallationEncryptionKey)
	})
	go runControlPlaneRetryLoop(ctx, runtime.services.health, "scheduler", "schedule runner", func(ctx context.Context) error {
		return controlplane.RunScheduler(ctx, runtime.services.scheduleRepository, 500*time.Millisecond)
	})
	go runtime.services.projectionService.Run(ctx, 30*time.Minute)
}

func runSessionCleanup(ctx context.Context, repository store.SessionRepository, health *controlplane.Health) {
	const sessionRetention = 14 * 24 * time.Hour
	cleanup := func() {
		if err := repository.DeleteOlderThan(ctx, time.Now().UTC().Add(-sessionRetention)); err != nil && ctx.Err() == nil {
			health.MarkFailed(healthSessionCleanup, err)
			fmt.Fprintln(os.Stderr, "session cleanup:", err)
			return
		}
		health.MarkHealthy(healthSessionCleanup)
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
}

func runRetentionCleanup(ctx context.Context, cfg config.Config, services controlPlaneServices) {
	policy := store.RetentionPolicy{LogMonthsKeep: cfg.LogMonthsKeep, AuditMonthsKeep: cfg.AuditMonthsKeep, RunnerMetricsMonthsKeep: cfg.RunnerMetricsMonthsKeep}
	cleanup := func() {
		purgeControlPlaneRetention(ctx, policy, services.retentionRepository, services.storagePressure, services.logger)
	}
	cleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanup()
		case <-ctx.Done():
			return
		}
	}
}

func purgeControlPlaneRetention(ctx context.Context, policy store.RetentionPolicy, retentionRepository *store.RetentionStore, storagePressure func(context.Context) (platform.StoragePressure, error), logger *platform.Logger) {
	if _, err := retentionRepository.Purge(ctx, time.Now().UTC(), policy, 100); err != nil && ctx.Err() == nil {
		logger.Event(retentionCleanupFailed, map[string]string{"error": err.Error()})
		return
	}
	pressure, err := storagePressure(ctx)
	if err != nil {
		logger.Event(retentionCleanupFailed, map[string]string{"error": err.Error()})
		return
	}
	if pressure.State != platform.StorageCritical && pressure.State != platform.StorageEmergency {
		return
	}
	if _, err := retentionRepository.PurgeCriticalRuns(ctx, time.Now().UTC(), func() (float64, error) {
		current, err := storagePressure(ctx)
		if err != nil {
			return 0, err
		}
		if current.State == platform.StorageUnavailable {
			return 0, fmt.Errorf("database storage capacity unavailable")
		}
		return current.FreePercent, nil
	}, 100); err != nil && ctx.Err() == nil {
		logger.Event(retentionCleanupFailed, map[string]string{"error": err.Error()})
	}
}

func runControlPlaneHeartbeat(ctx context.Context, health *controlplane.Health, jetstream *queue.JetStream, runnerRepository *store.RunnerStore) {
	for ctx.Err() == nil {
		health.MarkHealthy(healthHeartbeat)
		if err := controlplane.RunRunnerHeartbeatMonitor(ctx, jetstream, runnerRepository, 30*time.Second, 10*time.Second); err != nil && ctx.Err() == nil {
			health.MarkFailed(healthHeartbeat, err)
			fmt.Fprintln(os.Stderr, "runner heartbeat monitor:", err)
			time.Sleep(time.Second)
		}
	}
}

func runControlPlaneRetryLoop(ctx context.Context, health *controlplane.Health, name, description string, run func(context.Context) error) {
	for ctx.Err() == nil {
		health.MarkHealthy(name)
		if err := run(ctx); err != nil && ctx.Err() == nil {
			health.MarkFailed(name, err)
			fmt.Fprintln(os.Stderr, description+":", err)
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

func configureControlPlaneSystemMetrics(ctx context.Context, runtime *controlPlaneRuntime, jetstream *queue.JetStream, deadLetterSignals func(context.Context) (platform.OperationalSignals, error)) {
	runtime.application.Ready = func(ctx context.Context) error {
		if err := runtime.pingDatabase(ctx); err != nil {
			return err
		}
		if jetstream == nil {
			return fmt.Errorf("NATS is not connected")
		}
		return runtime.services.health.Ready()
	}
	systemMetrics := api.NewSystemMetricsService(runtime.services.metrics, runtime.application.Ready, runtime.services.logger)
	systemMetrics.Storage = runtime.services.storagePressure
	systemMetrics.Signals = deadLetterSignals
	runtime.application.SystemMetrics = systemMetrics
	go runSystemMetricsEvaluation(ctx, systemMetrics, runtime.services.logger)
}

func runSystemMetricsEvaluation(ctx context.Context, systemMetrics *api.SystemMetricsService, logger *platform.Logger) {
	evaluate := func() {
		if err := systemMetrics.Evaluate(ctx); err != nil && ctx.Err() == nil {
			_ = logger.Event("system.alert_evaluation_failed", map[string]string{"error": err.Error()})
		}
	}
	evaluate()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			evaluate()
		case <-ctx.Done():
			return
		}
	}
}

func serveControlPlane(ctx context.Context, application api.Server) error {
	server := &http.Server{
		Addr:              ":8080",
		Handler:           application.Handler(),
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

var notifyContext = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

var closeControlPlaneDB = func(db *pgxpool.Pool) { db.Close() }

func startEmbeddedNATS(dataDir string) (*natsserver.Server, error) {
	server, err := natsserver.NewServer(&natsserver.Options{
		Host:                   "127.0.0.1",
		Port:                   -1,
		JetStream:              true,
		StoreDir:               filepath.Join(dataDir, "nats"),
		NoLog:                  true,
		NoSigs:                 true,
		DisableJetStreamBanner: true,
	})
	if err != nil {
		return nil, err
	}
	server.Start()
	if !server.ReadyForConnections(10 * time.Second) {
		server.Shutdown()
		return nil, errors.New("embedded NATS did not become ready")
	}
	return server, nil
}

func waitForDatabase(ctx context.Context, ping func(context.Context) error, retryInterval time.Duration) error {
	for {
		if err := ping(ctx); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		} else {
			fmt.Fprintln(os.Stderr, "database connection:", err)
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		}
	}
}
