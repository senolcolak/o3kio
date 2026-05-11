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
)

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

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", "config/o3k.yaml", "Path to configuration file")
	migrationsPath := fs.String("migrations", "migrations", "Path to migrations directory")
	_ = fs.Parse(args)

	// Load configuration (file not found = zero-config mode, returns empty Config).
	cfg, err := common.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
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

		fmt.Println("═══════════════════════════════════════════")
		fmt.Println("  O3K — OpenStack in a single binary")
		fmt.Println("═══════════════════════════════════════════")
		fmt.Printf("  Data:     %s\n", bootstrapResult.DataDir)
		fmt.Printf("  Database: SQLite (embedded)\n")
		fmt.Printf("  API:      http://localhost:35357/v3\n")
		fmt.Printf("  User:     admin\n")
		fmt.Printf("  Password: %s\n", bootstrapResult.AdminPassword)
		fmt.Println("───────────────────────────────────────────")
		fmt.Printf("  Join agents: o3k agent --server http://<this-ip>:6443 --token %s\n", bootstrapResult.AgentToken)
		fmt.Println("═══════════════════════════════════════════")
	}

	// In zero-config mode, always start the tunnel hub on default port with the bootstrap token.
	if bootstrapResult != nil {
		if cfg.Tunnel.Port == 0 {
			cfg.Tunnel.Port = 6443
		}
		if cfg.Tunnel.TokenSecret == "" {
			cfg.Tunnel.TokenSecret = bootstrapResult.AgentToken
		}
	}

	// Validate JWT secret now that bootstrap may have set it.
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

	// Seed defaults when running in zero-config (bootstrap) mode.
	if bootstrapResult != nil {
		if err := server.SeedDefaults(ctx, database.DB, bootstrapResult.AdminPassword); err != nil {
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

	// Initialize metadata service
	metadataService := metadata.NewService("localhost:8775")
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
		createPlacementServer(cfg, placementService, authService),
		createMetadataServer(cfg, metadataService),
	}

	// Channel for shutdown signaling (from OS signals or server failures)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start all servers
	for _, srv := range servers {
		srv := srv // capture loop variable
		go func() {
			log.Printf("Starting server on %s", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Server on %s failed: %v — initiating shutdown", srv.Addr, err)
				quit <- syscall.SIGTERM
			}
		}()
	}

	log.Println("O3K started successfully")
	log.Println("  - Keystone (Identity):    http://localhost:35357/v3")
	log.Println("  - Nova (Compute):         http://localhost:8774/v2.1")
	log.Println("  - Neutron (Network):      http://localhost:9696/v2.0")
	log.Println("  - Cinder (Block Storage): http://localhost:8776/v3")
	log.Println("  - Glance (Image):         http://localhost:9292/v2")
	log.Println("  - Placement:              http://localhost:8778")
	log.Println("  - Metadata Service:       http://localhost:8775")

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
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("identity"))
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
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("compute"))
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
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("network"))
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
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddlewareWithConfig(cfg.Server.CORSAllowedOrigins))
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("block-storage"))
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
	svc.RegisterRoutes(authGroup)

	return &http.Server{
		Addr:         common.BindAddress(cfg, cfg.Glance.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}
}

func createPlacementServer(cfg *common.Config, svc *placement.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.MetricsMiddleware())
	r.Use(middleware.ErrorHandlingMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.AuthMiddleware(authService))
	r.Use(middleware.EnforceAccessRules("placement"))
	r.NoRoute(middleware.NotFoundHandler())
	r.HandleMethodNotAllowed = true
	r.NoMethod(middleware.MethodNotAllowedHandler())

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:         common.BindAddress(cfg, 8778),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func createMetadataServer(cfg *common.Config, svc *metadata.Service) *http.Server {
	r := gin.New()
	middleware.RegisterHealthRoutes(r)
	middleware.RegisterMetricsRoute(r)
	r.Use(middleware.RequestIDMiddleware())
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
		Addr:         common.BindAddress(cfg, 8775),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}
