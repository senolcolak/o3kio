package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/cinder"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/compute"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/glance"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/cobaltcore-dev/o3k/internal/metadata"
	"github.com/cobaltcore-dev/o3k/internal/middleware"
	"github.com/cobaltcore-dev/o3k/internal/neutron"
	"github.com/cobaltcore-dev/o3k/internal/nova"
	"github.com/cobaltcore-dev/o3k/internal/placement"
	"github.com/cobaltcore-dev/o3k/internal/scheduler"
	"github.com/cobaltcore-dev/o3k/internal/server"
	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	migrations "github.com/cobaltcore-dev/o3k/migrations"
	"github.com/cobaltcore-dev/o3k/pkg/cache"
	"github.com/cobaltcore-dev/o3k/pkg/networking"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// initTracer initialises the global OpenTelemetry TracerProvider.
//
// If O3K_OTEL_ENDPOINT is set, traces are exported to that OTLP/gRPC collector
// (e.g. "localhost:4317"). Otherwise traces are written to stdout — no external
// infrastructure required for development or zero-config mode.
//
// The caller must call Shutdown on the returned provider before process exit.
func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("o3k"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTel resource: %w", err)
	}

	var exporter sdktrace.SpanExporter

	if endpoint := os.Getenv("O3K_OTEL_ENDPOINT"); endpoint != "" {
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create OTLP exporter: %w", err)
		}
		exporter = exp
		log.Printf("Tracing: OTLP exporter → %s", endpoint)
	} else {
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout trace exporter: %w", err)
		}
		exporter = exp
		log.Println("Tracing: stdout exporter (set O3K_OTEL_ENDPOINT for OTLP)")
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

// isSubcommand reports whether s is a recognised o3k subcommand.
func isSubcommand(s string) bool {
	switch s {
	case "server", "agent", "token", "migrate-datastore":
		return true
	}
	return false
}

func main() {
	if len(os.Args) >= 2 && isSubcommand(os.Args[1]) {
		switch os.Args[1] {
		case "server":
			runServer(os.Args[2:])
		case "agent":
			runAgent(os.Args[2:])
		case "token":
			runTokenCmd(os.Args[2:])
		case "migrate-datastore":
			runMigrateDatastore(os.Args[2:])
		}
		return
	}
	// Default: behave as "server" with full arg list so that
	// `o3k --config config/o3k.yaml` keeps working unchanged.
	runServer(os.Args[1:])
}

// defaultPorts maps each service to its standard OpenStack port.
// Used when the config has port=0 (not explicitly set).
var defaultPorts = struct {
	Keystone  int
	Nova      int
	Neutron   int
	Cinder    int
	Glance    int
	Placement int
	Metadata  int
	Tunnel    int
}{
	Keystone:  35357,
	Nova:      8774,
	Neutron:   9696,
	Cinder:    8776,
	Glance:    9292,
	Placement: 8778,
	Metadata:  8775,
	Tunnel:    6443,
}

// applyPortDefaults fills zero-value service ports.
// When basePort > 0 the ports are offset from it (K8s-style); otherwise the
// standard OpenStack port numbers are used.
func applyPortDefaults(cfg *common.Config, basePort int) {
	if basePort > 0 {
		// Offset layout: base+357, base+774, base+696, base+776, base+292, base+778, base+775
		if cfg.Keystone.Port == 0 {
			cfg.Keystone.Port = basePort + 357
		}
		if cfg.Nova.Port == 0 {
			cfg.Nova.Port = basePort + 774
		}
		if cfg.Neutron.Port == 0 {
			cfg.Neutron.Port = basePort + 696
		}
		if cfg.Cinder.Port == 0 {
			cfg.Cinder.Port = basePort + 776
		}
		if cfg.Glance.Port == 0 {
			cfg.Glance.Port = basePort + 292
		}
		return
	}
	if cfg.Keystone.Port == 0 {
		cfg.Keystone.Port = defaultPorts.Keystone
	}
	if cfg.Nova.Port == 0 {
		cfg.Nova.Port = defaultPorts.Nova
	}
	if cfg.Neutron.Port == 0 {
		cfg.Neutron.Port = defaultPorts.Neutron
	}
	if cfg.Cinder.Port == 0 {
		cfg.Cinder.Port = defaultPorts.Cinder
	}
	if cfg.Glance.Port == 0 {
		cfg.Glance.Port = defaultPorts.Glance
	}
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", "config/o3k.yaml", "Path to configuration file")
	migrationsPath := fs.String("migrations", "migrations", "Path to migrations directory")
	datastoreFlag := fs.String("datastore", "", "Database backend: sqlite (default, zero-config) or postgres")
	dbURLFlag := fs.String("db-url", "", "Database URL for postgres datastore (overrides DATABASE_URL env var)")
	portFlag := fs.Int("port", 0, "Base port for all services (0 = standard OpenStack ports: Keystone 35357, Nova 8774, …)")
	tlsCertFlag := fs.String("tls-cert-file", "", "Path to PEM-encoded TLS certificate (enables HTTPS for all HTTP services when paired with --tls-key-file)")
	tlsKeyFlag := fs.String("tls-key-file", "", "Path to PEM-encoded TLS private key (paired with --tls-cert-file)")
	_ = fs.Parse(args)

	// Load configuration (file not found = zero-config mode, returns empty Config).
	cfg, err := common.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// CLI flags override config-file values.
	// --db-url sets the postgres connection string.
	if *dbURLFlag != "" {
		cfg.Database.URL = *dbURLFlag
		cfg.Database.Datastore = *dbURLFlag
	}
	// --datastore sqlite|postgres overrides config when explicitly set.
	if *datastoreFlag != "" {
		switch *datastoreFlag {
		case "sqlite":
			// Force zero-config SQLite path; clear any postgres URL from config.
			cfg.Database.Datastore = ""
			cfg.Database.URL = ""
		case "postgres":
			// Require a connection URL from --db-url, O3K_DB_URL, or DATABASE_URL.
			if cfg.Database.Datastore == "" && cfg.Database.URL == "" {
				pgURL := os.Getenv("DATABASE_URL")
				if pgURL == "" {
					log.Fatalf("--datastore postgres requires --db-url, O3K_DB_URL, or DATABASE_URL env var")
				}
				cfg.Database.URL = pgURL
			}
		default:
			log.Fatalf("--datastore must be 'sqlite' or 'postgres', got %q", *datastoreFlag)
		}
	}

	// CLI TLS flags override config-file values for HTTP services.
	if *tlsCertFlag != "" {
		cfg.Server.TLSCertFile = *tlsCertFlag
	}
	if *tlsKeyFlag != "" {
		cfg.Server.TLSKeyFile = *tlsKeyFlag
	}
	if (cfg.Server.TLSCertFile == "") != (cfg.Server.TLSKeyFile == "") {
		log.Fatalf("FATAL: --tls-cert-file and --tls-key-file must both be set or both empty")
	}

	// Validate configuration before bootstrapping anything.
	if err := common.ValidateConfig(cfg); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Zero-config mode: if no database URL or datastore is configured, bootstrap
	// with embedded SQLite and auto-generate secrets.
	var bootstrapResult *server.BootstrapResult
	datastore := cfg.Database.Datastore
	if datastore == "" {
		datastore = cfg.Database.URL
	}
	if datastore == "" {
		cfg.ZeroConfig = true
		var bsErr error
		bootstrapResult, bsErr = server.Bootstrap()
		if bsErr != nil {
			log.Fatalf("Bootstrap failed: %v", bsErr)
		}
		datastore = "sqlite://" + bootstrapResult.DBPath

		// Use the bootstrap-generated JWT secret when config has none.
		if cfg.Keystone.JWTSecret == "" {
			cfg.Keystone.JWTSecret = bootstrapResult.JWTSecret
		}

		keystonePort := cfg.Keystone.Port
		if keystonePort == 0 {
			if *portFlag > 0 {
				keystonePort = *portFlag + 357
			} else {
				keystonePort = defaultPorts.Keystone
			}
		}
		fmt.Println("═══════════════════════════════════════════")
		fmt.Println("  O3K — OpenStack in a single binary")
		fmt.Println("═══════════════════════════════════════════")
		fmt.Printf("  Mode:     zero-config (SQLite, stub services)\n")
		fmt.Printf("  Data:     %s\n", bootstrapResult.DataDir)
		fmt.Printf("  Database: SQLite (embedded)\n")
		fmt.Printf("  API:      http://localhost:%d/v3\n", keystonePort)
		fmt.Printf("  User:     admin\n")
		fmt.Printf("  Password: %s\n", bootstrapResult.AdminPassword)
		fmt.Println("───────────────────────────────────────────")
		fmt.Printf("  Use --config to customize. Set O3K_JWT_SECRET for persistent auth.\n")
		fmt.Printf("  Join agents: o3k agent --server http://<this-ip>:6443 --token %s\n", bootstrapResult.AgentToken)
		fmt.Println("═══════════════════════════════════════════")
	}

	// Apply port defaults (after bootstrap so the banner can show the right port).
	applyPortDefaults(cfg, *portFlag)

	// In zero-config mode, always start the tunnel hub on default port with the bootstrap token.
	if bootstrapResult != nil {
		if cfg.Tunnel.Port == 0 {
			cfg.Tunnel.Port = defaultPorts.Tunnel
		}
		if cfg.Tunnel.TokenSecret == "" {
			cfg.Tunnel.TokenSecret = bootstrapResult.AgentToken
		}
	}

	// Validate JWT secret now that bootstrap may have set it.
	// In zero-config (SQLite) mode the secret was auto-generated above, so this
	// check only fires for postgres/explicit-config mode without a proper secret.
	if cfg.Keystone.JWTSecret == "" || cfg.Keystone.JWTSecret == "change-me-in-production" || len(cfg.Keystone.JWTSecret) < 32 {
		env := os.Getenv("O3K_ENV")
		if env != "development" && env != "test" {
			log.Fatalf("FATAL: JWT secret is not set or too short (min 32 chars). Set O3K_JWT_SECRET or use O3K_ENV=development for dev mode.")
		}
	}

	// Initialize structured logging
	middleware.InitLogger(&cfg.Logging)

	// Set up Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Connect to database with optimized pool settings
	ctx := context.Background()
	workerCtx, workerCancel := context.WithCancel(ctx)

	// Initialize OpenTelemetry tracing.
	// Stdout exporter is used by default; set O3K_OTEL_ENDPOINT for OTLP.
	tp, err := initTracer(ctx)
	if err != nil {
		log.Fatalf("Failed to initialise tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Tracer shutdown error: %v", err)
		}
	}()

	poolConfig := &database.PoolConfig{
		MaxConns:          int32(cfg.Database.MaxConnections),
		MinConns:          int32(cfg.Database.MinConnections),
		MaxConnLifetime:   cfg.Database.MaxConnLifetime,
		MaxConnIdleTime:   cfg.Database.MaxConnIdleTime,
		HealthCheckPeriod: cfg.Database.HealthCheckPeriod,
	}

	// Use defaults if not specified
	if poolConfig.MinConns == 0 {
		poolConfig.MinConns = 2
	}
	if poolConfig.MaxConnLifetime == 0 {
		poolConfig.MaxConnLifetime = 1 * time.Hour
	}
	if poolConfig.MaxConnIdleTime == 0 {
		poolConfig.MaxConnIdleTime = 15 * time.Minute
	}
	if poolConfig.HealthCheckPeriod == 0 {
		poolConfig.HealthCheckPeriod = 1 * time.Minute
	}

	// Determine datastore: explicit Datastore field takes priority over URL.
	// (Already set to the bootstrap SQLite path when running in zero-config mode.)
	if datastore == "" {
		datastore = cfg.Database.Datastore
		if datastore == "" {
			datastore = cfg.Database.URL
		}
	}

	if strings.HasPrefix(datastore, "sqlite://") || strings.HasPrefix(datastore, "sqlite:") {
		dbPath := strings.TrimPrefix(strings.TrimPrefix(datastore, "sqlite://"), "sqlite:")
		if dbPath == "" {
			dbPath = "/var/lib/o3k/db/state.db"
		}
		if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
			log.Fatalf("Failed to create database directory: %v", err)
		}
		if err := database.ConnectSQLite(ctx, dbPath); err != nil {
			log.Fatalf("Failed to connect to SQLite: %v", err)
		}
		log.Printf("Database: SQLite at %s", dbPath)
	} else {
		if err := database.Connect(ctx, datastore, poolConfig); err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		log.Printf("Database: PostgreSQL (pool: max=%d, min=%d)", poolConfig.MaxConns, poolConfig.MinConns)
	}
	defer database.Close()

	// Wrap the database connection with OpenTelemetry span instrumentation.
	// Every Exec/Query/QueryRow call will create a child span in the active trace.
	database.DB = database.NewTracingAdapter(database.DB)

	// Run migrations (PostgreSQL only — SQLite manages its own schema)
	if database.BackendType() == "postgres" {
		log.Println("Running database migrations...")
		if err := database.MigrateUp(datastore, *migrationsPath); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
	} else {
		log.Println("Running SQLite migrations...")
		if err := database.MigrateSQLiteFS(migrations.SQLiteFS); err != nil {
			log.Fatalf("Failed to run SQLite migrations: %v", err)
		}
	}

	// Seed defaults. In zero-config mode, the bootstrap-generated admin
	// password is used. Otherwise, O3K_ADMIN_PASSWORD must be set, or a
	// random password is generated and printed once to stderr.
	{
		var adminPassword string
		if bootstrapResult != nil {
			adminPassword = bootstrapResult.AdminPassword
		} else {
			adminPassword = os.Getenv("O3K_ADMIN_PASSWORD")
			if adminPassword == "" {
				generated, err := server.GenerateAdminPassword()
				if err != nil {
					log.Fatalf("FATAL: generate admin password: %v", err)
				}
				adminPassword = generated
				fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
				fmt.Fprintln(os.Stderr, "  O3K — generated initial admin password")
				fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
				fmt.Fprintf(os.Stderr, "  User:     admin\n")
				fmt.Fprintf(os.Stderr, "  Password: %s\n", adminPassword)
				fmt.Fprintln(os.Stderr, "  Set O3K_ADMIN_PASSWORD to use a fixed password.")
				fmt.Fprintln(os.Stderr, "  This is shown ONCE. Store it now.")
				fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
			}
		}
		if err := server.SeedDefaults(ctx, database.DB, adminPassword); err != nil {
			log.Printf("WARNING: seed defaults: %v", err)
		}
	}

	// Start TunnelHub gRPC server if configured
	var hub *tunnel.Hub
	if cfg.Tunnel.Port > 0 {
		tokenSecret := cfg.Tunnel.TokenSecret
		if cfg.Tunnel.TokenFile != "" {
			if data, err := os.ReadFile(cfg.Tunnel.TokenFile); err == nil {
				tokenSecret = strings.TrimSpace(string(data))
			}
		}
		hub = tunnel.NewHub(tokenSecret)

		// TLS is on by default; set tls_disabled=true in config to run plaintext (dev only).
		if !cfg.Tunnel.TLSDisabled {
			tlsCfg, tlsErr := tunnel.HubTLSConfig(cfg.Tunnel.TLSCertFile, cfg.Tunnel.TLSKeyFile)
			if tlsErr != nil {
				log.Fatalf("tunnel TLS setup failed: %v", tlsErr)
			}
			hub.SetTLSConfig(tlsCfg)
		}

		go func() {
			addr := fmt.Sprintf(":%d", cfg.Tunnel.Port)
			if err := hub.ListenAndServe(addr); err != nil {
				log.Printf("TunnelHub exited: %v", err)
			}
		}()
	}

	// Initialize cache
	var cacheInstance *cache.Cache
	if cfg.Cache.Enabled {
		cacheInstance, err = cache.NewCache(cache.Config{
			RedisURL:   cfg.Cache.RedisURL,
			Enabled:    cfg.Cache.Enabled,
			KeyPrefix:  cfg.Cache.KeyPrefix,
			DefaultTTL: cfg.Cache.DefaultTTL,
		})
		if err != nil {
			log.Fatalf("Failed to initialize cache: %v", err)
		}
		log.Printf("Redis cache enabled (prefix: %s, default TTL: %v)", cfg.Cache.KeyPrefix, cfg.Cache.DefaultTTL)
	} else {
		log.Println("Cache disabled")
	}

	// Initialize services
	authService := keystone.NewAuthService(cfg.Keystone.JWTSecret, cfg.Keystone.TokenTTL, cacheInstance)
	keystoneService := keystone.NewService(authService, cacheInstance)

	// Rate-limit token creation per IP. Default 600/min is brute-force-resistant
	// (still gates ~10 attempts/sec per attacker IP) while permitting parallel
	// contract/integration test suites and bursty CLI clients that re-auth per
	// command. Tunable via O3K_AUTH_RATE_LIMIT (requests per minute, 0 disables).
	authRate := 600
	if v := os.Getenv("O3K_AUTH_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			authRate = n
		}
	}
	if authRate > 0 {
		keystoneService.SetAuthRateLimiter(
			middleware.RateLimitMiddleware(middleware.NewRateLimiter(authRate, time.Minute)),
		)
	}

	// Load policy rules from DB (best-effort; table may not exist before migration 067)
	if err := keystoneService.LoadPoliciesFromDB(ctx); err != nil {
		log.Printf("WARNING: failed to load policies from DB (table may not exist yet): %v", err)
	}

	// Set default libvirt mode if not specified
	libvirtMode := cfg.Nova.LibvirtMode
	if libvirtMode == "" {
		libvirtMode = "stub"
	}
	novaService := nova.NewService(cfg.Nova.LibvirtURI, libvirtMode, cacheInstance)
	novaService.SetCephMonitors(cfg.Nova.CephMonitors)

	// Initialize hypervisor
	if err := novaService.InitHypervisor(); err != nil {
		log.Printf("WARNING: Failed to initialize hypervisor: %v", err)
		log.Printf("Nova will run in %s mode", libvirtMode)
	} else {
		log.Printf("Hypervisor initialized successfully in %s mode", libvirtMode)
	}

	// Set default networking mode if not specified
	networkingMode := cfg.Neutron.NetworkingMode
	if networkingMode == "" {
		networkingMode = "stub"
	}
	neutronService := neutron.NewService(networkingMode, cacheInstance)
	log.Printf("Neutron initialized in %s mode", networkingMode)

	// Wire up Nova-Neutron integration (so Nova can allocate ports)
	novaService.SetNeutronService(neutronService)

	// Wire async dispatcher when AsyncCompute is enabled and Hub is running
	if cfg.Nova.AsyncCompute && hub != nil {
		dispatcher := tunnel.NewDispatcher(hub)
		novaService.SetDispatcher(dispatcher)
		log.Printf("Nova async compute enabled — dispatching to agents via tunnel")
	}

	// Start task worker pool and reconciler when async compute is enabled
	if cfg.Nova.AsyncCompute && hub != nil {
		maxWorkers := cfg.Tasks.MaxWorkers
		if maxWorkers == 0 {
			maxWorkers = 10
		}
		reconcileInterval := cfg.Tasks.ReconcilerInterval
		if reconcileInterval == 0 {
			reconcileInterval = 30
		}

		hubAdapter := scheduler.NewHubAdapter(hub)
		for i := 0; i < maxWorkers; i++ {
			w := scheduler.NewWorker(database.DB, hubAdapter)
			go w.Run(workerCtx)
		}

		r := scheduler.NewReconciler(database.DB, reconcileInterval)
		go r.Run(workerCtx)

		log.Printf("Task scheduler started: %d workers, reconciler every %ds", maxWorkers, reconcileInterval)
	}

	// Initialize VXLAN if enabled
	var vxlanCoordinator *neutron.VXLANCoordinator
	var nodeRegistry *compute.NodeRegistry

	if cfg.Neutron.VXLANEnabled {
		// Create node registry
		nodeRegistry, err = compute.NewNodeRegistry(
			cfg.Compute.NodeID,
			cfg.Compute.TunnelIP,
			cfg.Compute.HeartbeatInterval,
		)
		if err != nil {
			log.Fatalf("Failed to create node registry: %v", err)
		}

		// Register node
		if err := nodeRegistry.RegisterNode(ctx); err != nil {
			log.Fatalf("Failed to register node: %v", err)
		}
		log.Printf("Node registered: %s (tunnel IP: %s)", nodeRegistry.GetHostname(), nodeRegistry.GetTunnelIP())

		// Start heartbeat
		go nodeRegistry.StartHeartbeat(ctx)

		// Create VXLAN manager
		vxlanManager := networking.NewVXLANManager(networkingMode, cfg.Compute.VXLANPort)

		// Create VXLAN coordinator
		vxlanCoordinator = neutron.NewVXLANCoordinator(
			vxlanManager,
			nodeRegistry,
			neutronService.GetNamespaceManager(),
			cfg.Neutron.CoordinationPollInterval,
			cfg.Neutron.VNIRangeStart,
			cfg.Neutron.VNIRangeEnd,
		)

		// Set coordinator in neutron service
		neutronService.SetVXLANCoordinator(vxlanCoordinator)

		// Start coordinator
		go vxlanCoordinator.Start(ctx)

		log.Printf("VXLAN overlay networking enabled (VNI range: %d-%d)", cfg.Neutron.VNIRangeStart, cfg.Neutron.VNIRangeEnd)
	}

	// Set default storage mode if not specified
	cinderStorageMode := cfg.Cinder.StorageMode
	if cinderStorageMode == "" {
		cinderStorageMode = "stub"
	}
	cinderService := cinder.NewService(cinderStorageMode, cfg.Cinder.CephPool, cfg.Cinder.CephConf)
	log.Printf("Cinder initialized in %s mode", cinderStorageMode)

	glanceStorageMode := cfg.Glance.StorageMode
	if glanceStorageMode == "" {
		glanceStorageMode = "stub"
	}
	glanceService := glance.NewService(
		glanceStorageMode,
		cfg.Glance.CephPool,
		cfg.Glance.CephConf,
		cfg.Glance.S3Bucket,
		cfg.Glance.S3Region,
		cfg.Glance.S3Endpoint,
		cacheInstance,
	)
	log.Printf("Glance initialized in %s mode", glanceStorageMode)

	// Metadata and Placement ports are not in Config yet; use defaults.
	metadataPort := defaultPorts.Metadata
	placementPort := defaultPorts.Placement
	if *portFlag > 0 {
		metadataPort = *portFlag + 775
		placementPort = *portFlag + 778
	}

	// Initialize metadata service
	metadataService := metadata.NewService(fmt.Sprintf("localhost:%d", metadataPort))
	log.Println("Metadata service initialized")

	// Initialize placement service
	placementService := placement.NewService()
	log.Println("Placement service initialized")

	// Create HTTP servers for each service
	servers := []*http.Server{
		createKeystoneServer(cfg, keystoneService, authService),
		createNovaServer(cfg, novaService, authService),
		createNeutronServer(cfg, neutronService, authService),
		createCinderServer(cfg, cinderService, authService),
		createGlanceServer(cfg, glanceService, authService),
		createPlacementServer(cfg, placementService, authService, placementPort),
		createMetadataServer(cfg, metadataService, metadataPort),
	}

	// Channel for shutdown signaling (from OS signals or server failures)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start all servers
	tlsEnabled := cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != ""
	if tlsEnabled {
		log.Printf("HTTPS enabled for all HTTP services (cert=%s)", cfg.Server.TLSCertFile)
	} else {
		log.Printf("WARNING: HTTP services running WITHOUT TLS. Set server.tls_cert_file/tls_key_file or front O3K with a TLS-terminating reverse proxy in production.")
	}
	for _, srv := range servers {
		srv := srv // capture loop variable
		go func() {
			scheme := "http"
			if tlsEnabled {
				scheme = "https"
			}
			log.Printf("Starting server on %s://%s", scheme, srv.Addr)
			var err error
			if tlsEnabled {
				err = srv.ListenAndServeTLS(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
			} else {
				err = srv.ListenAndServe()
			}
			if err != nil && err != http.ErrServerClosed {
				log.Printf("Server on %s failed: %v — initiating shutdown", srv.Addr, err)
				quit <- syscall.SIGTERM
			}
		}()
	}

	log.Println("O3K started successfully")
	log.Printf("  - Keystone (Identity):    http://localhost:%d/v3", cfg.Keystone.Port)
	log.Printf("  - Nova (Compute):         http://localhost:%d/v2.1", cfg.Nova.Port)
	log.Printf("  - Neutron (Network):      http://localhost:%d/v2.0", cfg.Neutron.Port)
	log.Printf("  - Cinder (Block Storage): http://localhost:%d/v3", cfg.Cinder.Port)
	log.Printf("  - Glance (Image):         http://localhost:%d/v2", cfg.Glance.Port)
	log.Printf("  - Placement:              http://localhost:%d", placementPort)
	log.Printf("  - Metadata Service:       http://localhost:%d", metadataPort)

	// Wait for shutdown signal
	<-quit

	log.Println("Shutting down servers...")

	// Stop task workers and reconciler
	workerCancel()

	// Stop service background goroutines before closing HTTP servers
	novaService.Shutdown()
	log.Println("Nova background goroutines stopped")
	cinderService.Shutdown()
	log.Println("Cinder background goroutines stopped")

	// Stop VXLAN services
	if vxlanCoordinator != nil {
		vxlanCoordinator.Stop()
		log.Println("VXLAN coordinator stopped")
	}
	if nodeRegistry != nil {
		nodeRegistry.StopHeartbeat()
		log.Println("Node heartbeat stopped")
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}

	log.Println("O3K stopped")
}

func runAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	serverAddr := fs.String("server", "", "o3k server address (host:port, required)")
	token := fs.String("token", "", "join token (from server's agent-token file)")
	tokenFile := fs.String("token-file", "", "path to file containing join token")
	nodeIDFile := fs.String("node-id-file", "", "path to persist node UUID (default: {data-dir}/agent/node-id)")
	mode := fs.String("mode", "stub", "compute mode: stub or real")
	tunnelTLS := fs.Bool("tunnel-tls", true, "use TLS when connecting to the hub (set false for plaintext dev mode)")
	_ = fs.Parse(args)

	if *serverAddr == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --server is required")
		fmt.Fprintln(os.Stderr, "Usage: o3k agent --server <host:port> --token <join-token>")
		os.Exit(1)
	}

	// Resolve token.
	joinToken := *token
	if joinToken == "" && *tokenFile != "" {
		data, err := os.ReadFile(*tokenFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: cannot read token file: %v\n", err)
			os.Exit(1)
		}
		joinToken = strings.TrimSpace(string(data))
	}
	if joinToken == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --token or --token-file is required")
		os.Exit(1)
	}

	// Resolve node ID (generated once, then persisted).
	dataDir := server.DataDir()
	agentDir := filepath.Join(dataDir, "agent")
	if err := os.MkdirAll(agentDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot create agent dir: %v\n", err)
		os.Exit(1)
	}
	nodeIDPath := *nodeIDFile
	if nodeIDPath == "" {
		nodeIDPath = filepath.Join(agentDir, "node-id")
	}
	nodeID := loadOrGenerateNodeID(nodeIDPath)

	// The hub validates via VerifyTokenHash(secret, nodeID, hash), so we must
	// send GenerateTokenHash(joinToken, nodeID) as the tokenHash.
	tokenHash := tunnel.GenerateTokenHash(joinToken, nodeID)

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  O3K Agent")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Server:  %s\n", *serverAddr)
	fmt.Printf("  Node ID: %s\n", nodeID)
	fmt.Printf("  Mode:    %s\n", *mode)
	fmt.Println("═══════════════════════════════════════════")

	client := tunnel.NewAgentClientWithExecutor(*serverAddr, nodeID, tokenHash, *mode)

	// Configure TLS for the agent when tunnel-tls is enabled (default true).
	// The agent uses InsecureSkipVerify because the server uses a self-signed cert.
	// For production, pass a CA cert file via --tunnel-ca instead (future work).
	if *tunnelTLS {
		tlsCfg, tlsErr := tunnel.AgentTLSConfig()
		if tlsErr != nil {
		fmt.Fprintf(os.Stderr, "ERROR: tunnel TLS config: %v\n", tlsErr)
			os.Exit(1)
		}
		client.SetTLSConfig(tlsCfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down agent...")
		cancel()
	}()

	if err := client.Connect(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "Agent connection failed: %v\n", err)
		os.Exit(1)
	}
}

func loadOrGenerateNodeID(path string) string {
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data))
	}
	id := uuid()
	_ = os.WriteFile(path, []byte(id+"\n"), 0600)
	return id
}

// uuid generates a new random UUID v4 string using crypto/rand.
func uuid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: hex-encode a timestamp + random bytes via crypto/rand is
		// already imported; if it fails we have bigger problems.
		panic(fmt.Sprintf("uuid: crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func runTokenCmd(args []string) {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	configPath := fs.String("config", "config/o3k.yaml", "path to config")
	nodeID := fs.String("node-id", "", "node ID to generate token for (required)")
	_ = fs.Parse(args)

	if *nodeID == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --node-id is required")
		os.Exit(1)
	}

	cfg, err := common.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to load config: %v\n", err)
		os.Exit(1)
	}

	secret := cfg.Tunnel.TokenSecret
	if cfg.Tunnel.TokenFile != "" {
		if data, err := os.ReadFile(cfg.Tunnel.TokenFile); err == nil {
			secret = strings.TrimSpace(string(data))
		}
	}
	if secret == "" {
		fmt.Fprintln(os.Stderr, "ERROR: tunnel.token_secret not set in config")
		os.Exit(1)
	}

	hash := tunnel.GenerateTokenHash(secret, *nodeID)
	fmt.Println(hash)
}

func createKeystoneServer(cfg *common.Config, svc *keystone.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("identity"))
	r.Use(middleware.PolicyMiddleware("identity"))
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	// Root version discovery
	r.GET("/", func(c *gin.Context) {
		baseURL := common.BaseURL(c, cfg.Keystone.Port) + "/v3"
		c.JSON(200, gin.H{
			"versions": gin.H{
				"values": []gin.H{
					{
						"id":     "v3.14",
						"status": "stable",
						"links": []gin.H{
							{"rel": "self", "href": baseURL},
						},
						"media-types": []gin.H{
							{"type": "application/vnd.openstack.identity-v3+json", "base": "application/json"},
						},
					},
				},
			},
		})
	})

	svc.RegisterRoutes(r.Group(""), middleware.RequireRole("admin"))

	return &http.Server{
		Addr:         common.BindAddress(cfg, cfg.Keystone.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func createNovaServer(cfg *common.Config, svc *nova.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("compute"))
	r.Use(middleware.PolicyMiddleware("compute"))
	r.Use(nova.MicroversionMiddleware())
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:         common.BindAddress(cfg, cfg.Nova.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func createNeutronServer(cfg *common.Config, svc *neutron.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("network"))
	r.Use(middleware.PolicyMiddleware("network"))
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:         common.BindAddress(cfg, cfg.Neutron.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func createCinderServer(cfg *common.Config, svc *cinder.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("block-storage"))
	r.Use(middleware.PolicyMiddleware("volume"))
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:         common.BindAddress(cfg, cfg.Cinder.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func createGlanceServer(cfg *common.Config, svc *glance.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	// Version discovery endpoints (no auth required per OpenStack spec)
	root := r.Group("")
	root.GET("/", svc.GetVersions)
	root.GET("/v2", svc.GetVersionV2)

	// All other routes require authentication and are under /v2
	authGroup := r.Group("/v2")
	authGroup.Use(middleware.AuthMiddleware(authService))
	authGroup.Use(middleware.EnforceAccessRules("image"))
	authGroup.Use(middleware.PolicyMiddleware("image"))
	svc.RegisterRoutes(authGroup)

	return &http.Server{
		Addr:         common.BindAddress(cfg, cfg.Glance.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}
}

func createPlacementServer(cfg *common.Config, svc *placement.Service, authService *keystone.AuthService, port int) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("placement"))
	r.Use(middleware.PolicyMiddleware("placement"))
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:         common.BindAddress(cfg, port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func createMetadataServer(cfg *common.Config, svc *metadata.Service, port int) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.TracingMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	// No auth middleware - metadata service uses instance IP identification
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:         common.BindAddress(cfg, port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}
