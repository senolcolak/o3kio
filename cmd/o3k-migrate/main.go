package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/middleware"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Parse common flags
	configPath := flag.String("config", "config/o3k.yaml", "Path to configuration file")
	migrationsPath := flag.String("migrations", "migrations", "Path to migrations directory")
	_ = flag.CommandLine.Parse(os.Args[2:])

	// Load configuration
	cfg, err := common.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging
	middleware.InitLogger(&cfg.Logging)

	// Validate migrations directory
	if err := database.ValidateMigrations(*migrationsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Migration validation failed: %v\n", err)
		os.Exit(1)
	}

	// Execute command
	switch command {
	case "status":
		handleStatus(cfg.Database.URL, *migrationsPath)
	case "up":
		handleUp(cfg.Database.URL, *migrationsPath)
	case "down":
		handleDown(cfg.Database.URL, *migrationsPath)
	case "reset":
		handleReset(cfg.Database.URL, *migrationsPath)
	case "goto":
		handleGoto(cfg.Database.URL, *migrationsPath)
	case "force":
		handleForce(cfg.Database.URL, *migrationsPath)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleStatus(dbURL, migrationsPath string) {
	info, err := database.GetMigrationStatus(dbURL, migrationsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get migration status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Database Migration Status\n")
	fmt.Printf("=========================\n")
	fmt.Printf("Current Version: %d\n", info.Version)
	if info.Dirty {
		fmt.Printf("Status: DIRTY (manual intervention required)\n")
		fmt.Printf("\nTo fix dirty state, use:\n")
		fmt.Printf("  o3k-migrate force <version>\n")
		os.Exit(1)
	} else {
		fmt.Printf("Status: Clean\n")
	}
}

func handleUp(dbURL, migrationsPath string) {
	if err := database.MigrateUp(dbURL, migrationsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Migrations applied successfully")
}

func handleDown(dbURL, migrationsPath string) {
	fmt.Print("⚠️  Are you sure you want to roll back one migration? (yes/no): ")
	var response string
	_, _ = fmt.Scanln(&response)
	if response != "yes" {
		fmt.Println("Rollback cancelled")
		return
	}

	if err := database.MigrateDown(dbURL, migrationsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Migration rolled back successfully")
}

func handleReset(dbURL, migrationsPath string) {
	fmt.Print("⚠️  WARNING: This will DROP ALL TABLES and re-run all migrations.\n")
	fmt.Print("Are you absolutely sure? Type 'RESET' to confirm: ")
	var response string
	_, _ = fmt.Scanln(&response)
	if response != "RESET" {
		fmt.Println("Reset cancelled")
		return
	}

	if err := database.MigrateReset(dbURL, migrationsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Reset failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Database reset complete")
}

func handleGoto(dbURL, migrationsPath string) {
	if len(flag.Args()) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: o3k-migrate goto <version>\n")
		os.Exit(1)
	}

	targetVersion, err := strconv.ParseUint(flag.Arg(0), 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid version number: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("⚠️  Migrating to version %d. Continue? (yes/no): ", targetVersion)
	var response string
	_, _ = fmt.Scanln(&response)
	if response != "yes" {
		fmt.Println("Migration cancelled")
		return
	}

	if err := database.MigrateToVersion(dbURL, migrationsPath, uint(targetVersion)); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Migrated to version %d\n", targetVersion)
}

func handleForce(dbURL, migrationsPath string) {
	if len(flag.Args()) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: o3k-migrate force <version>\n")
		os.Exit(1)
	}

	version, err := strconv.Atoi(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid version number: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("⚠️  WARNING: Forcing version to %d without running migrations.\n", version)
	fmt.Print("This should only be used to fix dirty state. Continue? (yes/no): ")
	var response string
	_, _ = fmt.Scanln(&response)
	if response != "yes" {
		fmt.Println("Force cancelled")
		return
	}

	if err := database.ForceMigrationVersion(dbURL, migrationsPath, version); err != nil {
		fmt.Fprintf(os.Stderr, "Force failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Version forced to %d\n", version)
}

func printUsage() {
	fmt.Println("O3K Database Migration Tool")
	fmt.Println("===========================")
	fmt.Println()
	fmt.Println("Usage: o3k-migrate <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  status                Show current migration version and status")
	fmt.Println("  up                    Apply all pending migrations")
	fmt.Println("  down                  Roll back one migration")
	fmt.Println("  goto <version>        Migrate to specific version")
	fmt.Println("  reset                 Drop all tables and re-run migrations (DANGEROUS)")
	fmt.Println("  force <version>       Force migration version (for fixing dirty state)")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -config <path>        Path to configuration file (default: config/o3k.yaml)")
	fmt.Println("  -migrations <path>    Path to migrations directory (default: migrations)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  o3k-migrate status")
	fmt.Println("  o3k-migrate up")
	fmt.Println("  o3k-migrate down")
	fmt.Println("  o3k-migrate goto 5")
	fmt.Println("  o3k-migrate force 7")
	fmt.Println()
}
