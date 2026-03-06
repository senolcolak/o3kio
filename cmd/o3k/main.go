package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/cobaltcore-dev/o3k/internal/cinder"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/glance"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/cobaltcore-dev/o3k/internal/middleware"
	"github.com/cobaltcore-dev/o3k/internal/neutron"
	"github.com/cobaltcore-dev/o3k/internal/nova"
)

var (
	configPath     = flag.String("config", "config/o3k.yaml", "Path to configuration file")
	migrationsPath = flag.String("migrations", "migrations", "Path to migrations directory")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := common.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set up logging
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Connect to database
	ctx := context.Background()
	if err := database.Connect(ctx, cfg.Database.URL, cfg.Database.MaxConnections); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	log.Println("Database connection established")

	// Run migrations
	// NOTE: Skipping migrations since tables are already created
	// absPath, err := filepath.Abs(*migrationsPath)
	// if err != nil {
	// 	log.Fatalf("Failed to get absolute migrations path: %v", err)
	// }
	// if err := database.RunMigrations(cfg.Database.URL, absPath); err != nil {
	// 	log.Fatalf("Failed to run migrations: %v", err)
	// }

	log.Println("Database migrations skipped (tables already exist)")

	// Initialize services
	authService := keystone.NewAuthService(cfg.Keystone.JWTSecret, cfg.Keystone.TokenTTL)
	keystoneService := keystone.NewService(authService)
	novaService := nova.NewService(cfg.Nova.LibvirtURI)

	// Initialize hypervisor (non-fatal if libvirt not available)
	if err := novaService.InitHypervisor(); err != nil {
		log.Printf("WARNING: Failed to initialize hypervisor: %v", err)
		log.Println("Nova will run in stub mode (no actual VM creation)")
	} else {
		log.Println("Hypervisor initialized successfully")
	}

	neutronService := neutron.NewService()
	cinderService := cinder.NewService(cfg.Cinder.CephPool, cfg.Cinder.CephConf)
	glanceService := glance.NewService(cfg.Glance.CephPool, cfg.Glance.CephConf)

	// Create HTTP servers for each service
	servers := []*http.Server{
		createKeystoneServer(cfg, keystoneService),
		createNovaServer(cfg, novaService, authService),
		createNeutronServer(cfg, neutronService, authService),
		createCinderServer(cfg, cinderService, authService),
		createGlanceServer(cfg, glanceService, authService),
	}

	// Start all servers
	for _, srv := range servers {
		srv := srv // capture loop variable
		go func() {
			log.Printf("Starting server on %s", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server failed: %v", err)
			}
		}()
	}

	log.Println("O3K started successfully")
	log.Println("  - Keystone (Identity):    http://localhost:5000/v3")
	log.Println("  - Nova (Compute):         http://localhost:8774/v2.1")
	log.Println("  - Neutron (Network):      http://localhost:9696/v2.0")
	log.Println("  - Cinder (Block Storage): http://localhost:8776/v3")
	log.Println("  - Glance (Image):         http://localhost:9292/v2")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

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

func createKeystoneServer(cfg *common.Config, svc *keystone.Service) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())

	// Root version discovery
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"versions": gin.H{
				"values": []gin.H{
					{
						"id":     "v3.14",
						"status": "stable",
						"links": []gin.H{
							{"rel": "self", "href": fmt.Sprintf("http://localhost:%d/v3", cfg.Keystone.Port)},
						},
					},
				},
			},
		})
	})

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Keystone.Port),
		Handler: r,
	}
}

func createNovaServer(cfg *common.Config, svc *nova.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware(authService))

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Nova.Port),
		Handler: r,
	}
}

func createNeutronServer(cfg *common.Config, svc *neutron.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware(authService))

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Neutron.Port),
		Handler: r,
	}
}

func createCinderServer(cfg *common.Config, svc *cinder.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware(authService))

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Cinder.Port),
		Handler: r,
	}
}

func createGlanceServer(cfg *common.Config, svc *glance.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware(authService))

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Glance.Port),
		Handler: r,
	}
}
