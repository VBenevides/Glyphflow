package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/api"
	"github.com/VBenevides/Glyphflow/backend/internal/config"
	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newSQLiteControlPlaneDatabase(t *testing.T) (controlPlaneDatabase, config.Config) {
	t.Helper()
	t.Chdir("/home/wdtg/Projects/Glyphflow/backend")
	cfg := config.Config{
		DatabaseMode:                 "sqlite",
		DatabaseURL:                  t.TempDir() + "/controlplane.sqlite",
		NATSMode:                     "embedded",
		NATSURL:                      "nats://127.0.0.1:4222",
		AccessTokenSecret:            "01234567890123456789012345678901",
		PasswordPepper:               "0123456789012345",
		WebOrigin:                    "http://localhost:3000",
		CSRFOrigins:                  []string{"http://localhost:3000"},
		Environment:                  "development",
		AllowInsecureTransport:       true,
		DataDir:                      t.TempDir(),
		MaxMessageBytes:              1024,
		DefaultRoleID:                "system-user",
		LogMonthsKeep:                3,
		AuditMonthsKeep:              12,
		RunnerMetricsMonthsKeep:      3,
		InstallationEncryptionKey:    []byte("01234567890123456789012345678901"),
		DatabaseStorageCapacityBytes: 0,
	}
	database, err := openControlPlaneDatabase(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.closeDatabase)
	return database, cfg
}

func newSQLiteControlPlaneRuntime(t *testing.T) (controlPlaneDatabase, config.Config, controlPlaneRuntime) {
	t.Helper()
	database, cfg := newSQLiteControlPlaneDatabase(t)
	runtime, err := initializeControlPlaneRuntime(context.Background(), &cfg, database)
	if err != nil {
		t.Fatal(err)
	}
	return database, cfg, runtime
}

func TestWarnOnWildcardCORS(t *testing.T) {
	warnOnWildcardCORS([]string{"https://example.test", "*"})
	warnOnWildcardCORS([]string{"https://example.test"})
}

func TestConnectControlPlaneJetStreamRejectsUnsupportedURLs(t *testing.T) {
	if _, err := connectControlPlaneJetStream(context.Background(), config.Config{NATSURL: "http://nats.example"}); err == nil {
		t.Fatal("plain JetStream connection accepted a non-NATS URL")
	}
	if _, err := connectControlPlaneJetStream(context.Background(), config.Config{NATSURL: "tls://nats.example"}); err == nil {
		t.Fatal("TLS JetStream connection accepted missing client certificates")
	}
}

func TestOpenControlPlaneDatabaseRejectsInvalidSQLiteInputs(t *testing.T) {
	if _, err := openControlPlaneDatabase(context.Background(), config.Config{DatabaseMode: "sqlite", DatabaseURL: " "}); err == nil {
		t.Fatal("empty SQLite path was accepted")
	}
	_, cfg := newSQLiteControlPlaneDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := openControlPlaneDatabase(ctx, cfg); err == nil {
		t.Fatal("canceled SQLite migration was accepted")
	}
	if _, err := openControlPlaneDatabase(context.Background(), config.Config{DatabaseMode: "postgresql", DatabaseURL: "not-a-database-url"}); err == nil {
		t.Fatal("invalid PostgreSQL URL was accepted")
	}
}

func TestRunStopsAfterSQLiteStartupRuntimeFailure(t *testing.T) {
	setControlPlaneStartupEnv(t)
	t.Chdir("/home/wdtg/Projects/Glyphflow/backend")
	t.Setenv("GLYPHFLOW_DATABASE", "sqlite")
	t.Setenv("DATABASE_URL", t.TempDir()+"/controlplane.sqlite")
	t.Setenv("GLYPHFLOW_NATS", "remote")
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	t.Setenv("DEFAULT_ROLE_ID", "missing-role")
	if err := run(); err == nil {
		t.Fatal("startup accepted a missing default role")
	}
}

func TestRunStopsAfterJetStreamConnectionFailure(t *testing.T) {
	setControlPlaneStartupEnv(t)
	t.Chdir("/home/wdtg/Projects/Glyphflow/backend")
	t.Setenv("GLYPHFLOW_DATABASE", "sqlite")
	t.Setenv("DATABASE_URL", t.TempDir()+"/controlplane.sqlite")
	t.Setenv("GLYPHFLOW_NATS", "remote")
	t.Setenv("NATS_URL", "nats://127.0.0.1:not-a-port")
	if err := run(); err == nil {
		t.Fatal("startup accepted an invalid NATS port")
	}
}

func TestInitializeControlPlaneConfigUsesExplicitKeyAndRejectsInvalidKey(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	generated, err := protocol.GenerateSigningKey("test", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ControlPlaneSigningPrivateKey = base64.RawStdEncoding.EncodeToString(generated.Private)
	_, signingKey, err := initializeControlPlaneConfig(context.Background(), &cfg, database.db)
	if err != nil {
		t.Fatal(err)
	}
	if len(signingKey.Private) != len(generated.Private) {
		t.Fatalf("explicit signing key length = %d, want %d", len(signingKey.Private), len(generated.Private))
	}
	cfg.ControlPlaneSigningPrivateKey = "invalid"
	if _, _, err := initializeControlPlaneConfig(context.Background(), &cfg, database.db); err == nil {
		t.Fatal("invalid signing key was accepted")
	}
	database, cfg = newSQLiteControlPlaneDatabase(t)
	if err := store.NewConfigStore(database.db).Set(context.Background(), "ENABLE_PASSWORD_LOGIN", map[string]string{"invalid": "value"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := initializeControlPlaneConfig(context.Background(), &cfg, database.db); err == nil {
		t.Fatal("invalid stored configuration was accepted during initialization")
	}
}

func TestLoadStoredControlPlaneConfigReadsStoredValues(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	configStore := store.NewConfigStore(database.db)
	if err := loadStoredControlPlaneConfig(context.Background(), &cfg, configStore); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"GLYPHFLOW_SYSTEM_ADMINS":      []string{"admin@example.com"},
		"ENABLE_PASSWORD_LOGIN":        true,
		"ENABLE_PASSWORD_REGISTRATION": true,
		"REQUIRE_USER_APPROVAL":        true,
		"DEFAULT_ROLE_ID":              "stored-role",
		"LOCKDOWN_SCHEDULER":           true,
	}
	for name, value := range values {
		if err := configStore.Set(context.Background(), name, value); err != nil {
			t.Fatal(err)
		}
	}
	cfg.SystemAdminEmails = nil
	cfg.DefaultRoleID = "original-role"
	if err := loadStoredControlPlaneConfig(context.Background(), &cfg, configStore); err != nil {
		t.Fatal(err)
	}
	if len(cfg.SystemAdminEmails) != 1 || cfg.SystemAdminEmails[0] != "admin@example.com" || !cfg.PasswordLoginEnabled || !cfg.PasswordRegistrationEnabled || !cfg.RequireUserApproval || cfg.DefaultRoleID != "stored-role" || !cfg.LockdownScheduler {
		t.Fatalf("stored control-plane configuration was not loaded: %+v", cfg)
	}
}

func TestBuildControlPlaneApplicationValidationAndBootstrap(t *testing.T) {
	database, cfg, runtime := newSQLiteControlPlaneRuntime(t)
	if _, err := buildControlPlaneApplication(cfg, database, runtime.auth, controlPlaneServices{}); err == nil {
		t.Fatal("application with missing durable repositories was accepted")
	}
	cfg.SystemAdminEmails = []string{"not-an-email"}
	if _, err := buildControlPlaneApplication(cfg, database, runtime.auth, runtime.services); err == nil {
		t.Fatal("invalid system administrator email was accepted")
	}
	cfg.SystemAdminEmails = []string{"admin@example.com"}
	cfg.BootstrapUsername = "bootstrap@example.com"
	cfg.BootstrapPassword = "bootstrap-password"
	application, err := buildControlPlaneApplication(cfg, database, runtime.auth, runtime.services)
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapUsername = "invalid"
	if _, err := buildControlPlaneApplication(cfg, database, runtime.auth, runtime.services); err == nil {
		t.Fatal("invalid bootstrap email was accepted")
	}
	database.closeDatabase()
	if err := application.Ready(context.Background()); err == nil {
		t.Fatal("readiness accepted a closed database")
	}
}

func TestInitializeControlPlaneRuntimeReturnsInitializationErrors(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	cfg.ControlPlaneSigningPrivateKey = "invalid"
	if _, err := initializeControlPlaneRuntime(context.Background(), &cfg, database); err == nil {
		t.Fatal("invalid signing key was accepted during runtime initialization")
	}

	database, cfg = newSQLiteControlPlaneDatabase(t)
	cfg.AccessTokenSecret = "short"
	if _, err := initializeControlPlaneRuntime(context.Background(), &cfg, database); err == nil {
		t.Fatal("invalid access token secret was accepted during runtime initialization")
	}

	database, cfg, runtime := newSQLiteControlPlaneRuntime(t)
	cfg.SystemAdminEmails = []string{"not-an-email"}
	if _, err := initializeControlPlaneRuntime(context.Background(), &cfg, database); err == nil {
		t.Fatal("invalid system administrator email was accepted during runtime initialization")
	}
	if _, err := initializeControlPlaneServices(context.Background(), cfg, database.db, runtime.auth, runtime.signingKey); err != nil {
		t.Fatal("service initialization unexpectedly failed: ", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := initializeControlPlaneServices(ctx, cfg, database.db, runtime.auth, runtime.signingKey); err == nil {
		t.Fatal("canceled service initialization was accepted")
	}

	pool, err := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/glyphflow")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cfg.DatabaseMode = "postgresql"
	if _, err := initializeControlPlaneServices(context.Background(), cfg, pool, runtime.auth, runtime.signingKey); err == nil {
		t.Fatal("unavailable PostgreSQL service initialization was accepted")
	}
}

func TestInitializeControlPlaneRuntimeReturnsConfigurationStorageError(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	database.closeDatabase()
	if _, err := initializeControlPlaneRuntime(context.Background(), &cfg, database); err == nil {
		t.Fatal("runtime initialization accepted a closed configuration database")
	}
}

func TestConfigurationAndRoleSeedingPropagateStorageErrors(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	database.closeDatabase()
	configStore := store.NewConfigStore(database.db)
	if err := seedControlPlaneConfig(context.Background(), &cfg, configStore); err == nil {
		t.Fatal("configuration seeding accepted a closed database")
	}
	if _, err := initializeControlPlaneAuth(cfg, database.db, configStore); err == nil {
		t.Fatal("auth initialization accepted a closed database")
	}
	cfg.SystemAdminEmails = nil
	if err := loadStoredControlPlaneConfig(context.Background(), &cfg, configStore); err == nil {
		t.Fatal("configuration loading accepted a closed database")
	}
	auth, err := api.NewAuthService(cfg.AccessTokenSecret, true, true, []byte(cfg.PasswordPepper))
	if err != nil {
		t.Fatal(err)
	}
	auth.SetRoleRepository(store.NewRoleRepository(database.db))
	if err := seedControlPlaneAuthRoles(auth); err == nil {
		t.Fatal("auth role seeding accepted a closed database")
	}
	roles := api.NewRoleAdminService()
	roles.SetRepository(store.NewRoleRepository(database.db))
	if err := seedControlPlaneRoleAdmin(roles); err == nil {
		t.Fatal("role-admin seeding accepted a closed database")
	}
}

func TestStoredConfigurationDecodeErrorsAreReturned(t *testing.T) {
	tests := []struct {
		name   string
		target string
		prior  []string
	}{
		{name: "system admins", target: "GLYPHFLOW_SYSTEM_ADMINS"},
		{name: "password login", target: "ENABLE_PASSWORD_LOGIN"},
		{name: "password registration", target: "ENABLE_PASSWORD_REGISTRATION", prior: []string{"ENABLE_PASSWORD_LOGIN"}},
		{name: "user approval", target: "REQUIRE_USER_APPROVAL", prior: []string{"ENABLE_PASSWORD_LOGIN", "ENABLE_PASSWORD_REGISTRATION"}},
		{name: "default role", target: "DEFAULT_ROLE_ID", prior: []string{"ENABLE_PASSWORD_LOGIN", "ENABLE_PASSWORD_REGISTRATION", "REQUIRE_USER_APPROVAL"}},
		{name: "lockdown", target: "LOCKDOWN_SCHEDULER", prior: []string{"ENABLE_PASSWORD_LOGIN", "ENABLE_PASSWORD_REGISTRATION", "REQUIRE_USER_APPROVAL", "DEFAULT_ROLE_ID"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, cfg := newSQLiteControlPlaneDatabase(t)
			configStore := store.NewConfigStore(database.db)
			cfg.SystemAdminEmails = []string{"configured@example.com"}
			for _, name := range test.prior {
				value := any(true)
				if name == "DEFAULT_ROLE_ID" {
					value = "stored-role"
				}
				if err := configStore.Set(context.Background(), name, value); err != nil {
					t.Fatal(err)
				}
			}
			if test.target == "GLYPHFLOW_SYSTEM_ADMINS" {
				cfg.SystemAdminEmails = nil
			}
			if err := configStore.Set(context.Background(), test.target, map[string]string{"invalid": "value"}); err != nil {
				t.Fatal(err)
			}
			if err := loadStoredControlPlaneConfig(context.Background(), &cfg, configStore); err == nil {
				t.Fatal("invalid stored configuration was accepted")
			}
		})
	}
}

type roleRepositoryFailOnEnsure struct {
	store.RoleRepository
	failAt int
	calls  int
}

func (r *roleRepositoryFailOnEnsure) Ensure(ctx context.Context, id, name, description string, system bool, permissions []string) error {
	r.calls++
	if r.calls == r.failAt {
		return errors.New("role storage failed")
	}
	return r.RoleRepository.Ensure(ctx, id, name, description, system, permissions)
}

func TestRoleSeedingReturnsLaterRepositoryErrors(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	base := store.NewRoleRepository(database.db)
	failing := &roleRepositoryFailOnEnsure{RoleRepository: base, failAt: 2}
	auth, err := api.NewAuthService(cfg.AccessTokenSecret, true, true, []byte(cfg.PasswordPepper))
	if err != nil {
		t.Fatal(err)
	}
	auth.SetRoleRepository(failing)
	if err := seedControlPlaneAuthRoles(auth); err == nil {
		t.Fatal("auth role seeding accepted a later repository failure")
	}
	failing.calls = 0
	roles := api.NewRoleAdminService()
	roles.SetRepository(failing)
	if err := seedControlPlaneRoleAdmin(roles); err == nil {
		t.Fatal("role-admin seeding accepted a later repository failure")
	}
}

func TestRunReturnsConfigurationError(t *testing.T) {
	setControlPlaneStartupEnv(t)
	t.Setenv("ACCESS_TOKEN_SECRET", "short")
	if err := run(); err == nil {
		t.Fatal("invalid control-plane configuration was accepted")
	}
}

func TestSessionCleanupMarksHealthAfterCanceledCleanup(t *testing.T) {
	database, _ := newSQLiteControlPlaneDatabase(t)
	health := controlplane.NewHealth(healthSessionCleanup)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runSessionCleanup(ctx, store.NewSessionRepository(database.db), health)
	if err := health.Ready(); err != nil {
		t.Fatalf("session cleanup health = %v", err)
	}
}

func TestSessionCleanupRecordsRepositoryFailure(t *testing.T) {
	database, _ := newSQLiteControlPlaneDatabase(t)
	database.closeDatabase()
	health := controlplane.NewHealth(healthSessionCleanup)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	runSessionCleanup(ctx, store.NewSessionRepository(database.db), health)
	if err := health.Ready(); err == nil {
		t.Fatal("session cleanup repository failure was not recorded")
	}
}

func TestRetentionCleanupHandlesCanceledStorageCheck(t *testing.T) {
	database, cfg := newSQLiteControlPlaneDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := &platform.Logger{Out: io.Discard}
	runRetentionCleanup(ctx, cfg, controlPlaneServices{
		retentionRepository: store.NewRetentionRepository(database.db),
		storagePressure: func(context.Context) (platform.StoragePressure, error) {
			return platform.StoragePressure{}, errors.New("storage check failed")
		},
		logger: logger,
	})

	criticalContext, criticalCancel := context.WithCancel(context.Background())
	criticalCancel()
	purgeControlPlaneRetention(criticalContext, store.RetentionPolicy{}, store.NewRetentionRepository(database.db), func(context.Context) (platform.StoragePressure, error) {
		return platform.StoragePressure{State: platform.StorageCritical}, nil
	}, logger)
	purgeControlPlaneRetention(context.Background(), store.RetentionPolicy{}, nil, func(context.Context) (platform.StoragePressure, error) {
		return platform.StoragePressure{}, nil
	}, logger)
	pressureErrorContext, pressureErrorCancel := context.WithCancel(context.Background())
	pressureErrorCancel()
	pressureCalls := 0
	purgeControlPlaneRetention(pressureErrorContext, store.RetentionPolicy{}, store.NewRetentionRepository(database.db), func(context.Context) (platform.StoragePressure, error) {
		pressureCalls++
		if pressureCalls == 1 {
			return platform.StoragePressure{State: platform.StorageCritical}, nil
		}
		return platform.StoragePressure{}, errors.New("capacity unavailable")
	}, logger)
	unavailableContext, unavailableCancel := context.WithCancel(context.Background())
	unavailableCancel()
	unavailableCalls := 0
	purgeControlPlaneRetention(unavailableContext, store.RetentionPolicy{}, store.NewRetentionRepository(database.db), func(context.Context) (platform.StoragePressure, error) {
		unavailableCalls++
		if unavailableCalls == 1 {
			return platform.StoragePressure{State: platform.StorageCritical}, nil
		}
		return platform.StoragePressure{State: platform.StorageUnavailable}, nil
	}, logger)
}

func TestControlPlaneHeartbeatStopsAfterConfigurationFailure(t *testing.T) {
	database, _ := newSQLiteControlPlaneDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	runControlPlaneHeartbeat(ctx, controlplane.NewHealth(healthHeartbeat), nil, store.NewRunnerRepository(database.db))
}

func TestControlPlaneRetryLoopRecordsFailureAndStops(t *testing.T) {
	health := controlplane.NewHealth("retry")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	runControlPlaneRetryLoop(ctx, health, "retry", "retry test", func(context.Context) error {
		return errors.New("retry failed")
	})
	if err := health.Ready(); err == nil {
		t.Fatal("retry failure was not recorded")
	}
}

func TestControlPlaneRetryLoopStopsAfterSuccessfulRunCancellation(t *testing.T) {
	health := controlplane.NewHealth("retry")
	ctx, cancel := context.WithCancel(context.Background())
	runControlPlaneRetryLoop(ctx, health, "retry", "retry test", func(context.Context) error {
		cancel()
		return nil
	})
	if err := health.Ready(); err != nil {
		t.Fatalf("successful retry health = %v", err)
	}
}

func TestConfigureControlPlaneSystemMetricsReadiness(t *testing.T) {
	logger := &platform.Logger{Out: io.Discard}
	runtime := controlPlaneRuntime{
		pingDatabase: func(context.Context) error { return nil },
		services: controlPlaneServices{
			metrics: new(platform.Metrics),
			logger:  logger,
			health:  controlplane.NewHealth(),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	configureControlPlaneSystemMetrics(ctx, &runtime, &queue.JetStream{}, func(context.Context) (platform.OperationalSignals, error) {
		return platform.OperationalSignals{}, nil
	})
	if runtime.application.SystemMetrics == nil {
		t.Fatal("system metrics service was not configured")
	}
	if err := runtime.application.Ready(context.Background()); err != nil {
		t.Fatalf("ready callback with empty health = %v", err)
	}
	if err := runtime.application.Ready(context.Background()); err != nil {
		t.Fatalf("database-backed readiness = %v", err)
	}
	failedRuntime := controlPlaneRuntime{
		pingDatabase: func(context.Context) error { return errors.New("database unavailable") },
		services: controlPlaneServices{
			metrics: new(platform.Metrics),
			logger:  logger,
			health:  controlplane.NewHealth(),
		},
	}
	configureControlPlaneSystemMetrics(ctx, &failedRuntime, &queue.JetStream{}, nil)
	if err := failedRuntime.application.Ready(context.Background()); err == nil {
		t.Fatal("database failure was not returned by readiness")
	}
	missingNATSRuntime := controlPlaneRuntime{
		pingDatabase: func(context.Context) error { return nil },
		services: controlPlaneServices{
			metrics: new(platform.Metrics),
			logger:  logger,
			health:  controlplane.NewHealth(),
		},
	}
	configureControlPlaneSystemMetrics(ctx, &missingNATSRuntime, nil, nil)
	if err := missingNATSRuntime.application.Ready(context.Background()); err == nil {
		t.Fatal("missing JetStream was accepted by readiness")
	}
}

func TestConfigureControlPlaneJetStreamInstallsCallbacks(t *testing.T) {
	_, _, runtime := newSQLiteControlPlaneRuntime(t)
	deadLetterSignals := configureControlPlaneJetStream(&runtime, &queue.JetStream{})
	if deadLetterSignals == nil {
		t.Fatal("dead-letter signal callback was not returned")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := deadLetterSignals(ctx); err == nil {
		t.Fatal("canceled dead-letter stats query was accepted")
	}
	if signals, err := deadLetterSignals(context.Background()); err != nil || signals.DeadLetters.Open != 0 {
		t.Fatalf("dead-letter stats = %+v, %v", signals, err)
	}
}

func TestStartControlPlaneWorkersStopsWithoutNetworkListeners(t *testing.T) {
	_, cfg, runtime := newSQLiteControlPlaneRuntime(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	startControlPlaneWorkers(ctx, cfg, &runtime, nil)
	time.Sleep(30 * time.Millisecond)
}

func TestRunSystemMetricsEvaluationLogsEvaluationFailure(t *testing.T) {
	logger := &platform.Logger{Out: io.Discard}
	systemMetrics := api.NewSystemMetricsService(new(platform.Metrics), nil, logger)
	systemMetrics.Signals = func(context.Context) (platform.OperationalSignals, error) {
		return platform.OperationalSignals{}, errors.New("signals unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	runSystemMetricsEvaluation(ctx, systemMetrics, logger)
}
