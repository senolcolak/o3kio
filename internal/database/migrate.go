package database

import (
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog/log"
)

// MigrationInfo contains migration status information
type MigrationInfo struct {
	Version uint
	Dirty   bool
}

// CreateMigrator creates a new migrate instance
func CreateMigrator(dbURL, migrationsPath string) (*migrate.Migrate, error) {
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		dbURL,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}
	return m, nil
}

// GetMigrationStatus returns current migration version and dirty state
func GetMigrationStatus(dbURL, migrationsPath string) (*MigrationInfo, error) {
	m, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return nil, fmt.Errorf("failed to get migration version: %w", err)
	}

	return &MigrationInfo{
		Version: version,
		Dirty:   dirty,
	}, nil
}

// MigrateUp applies all pending migrations
func MigrateUp(dbURL, migrationsPath string) error {
	m, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	log.Info().Msg("Running database migrations...")

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			log.Info().Msg("Database is already up to date")
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	version, _, _ := m.Version()
	log.Info().Uint("version", version).Msg("Migrations applied successfully")
	return nil
}

// MigrateDown rolls back one migration
func MigrateDown(dbURL, migrationsPath string) error {
	m, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	currentVersion, _, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			return fmt.Errorf("no migrations to roll back")
		}
		return fmt.Errorf("failed to get current version: %w", err)
	}

	log.Warn().Uint("from_version", currentVersion).Msg("Rolling back one migration")

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	newVersion, _, _ := m.Version()
	log.Info().Uint("to_version", newVersion).Msg("Migration rolled back successfully")
	return nil
}

// MigrateToVersion migrates to a specific version
func MigrateToVersion(dbURL, migrationsPath string, targetVersion uint) error {
	m, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	currentVersion, _, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	log.Info().
		Uint("current_version", currentVersion).
		Uint("target_version", targetVersion).
		Msg("Migrating to specific version")

	if err := m.Migrate(targetVersion); err != nil {
		if err == migrate.ErrNoChange {
			log.Info().Msg("Already at target version")
			return nil
		}
		return fmt.Errorf("failed to migrate to version %d: %w", targetVersion, err)
	}

	log.Info().Uint("version", targetVersion).Msg("Migrated to target version")
	return nil
}

// MigrateReset drops all tables and re-runs all migrations
func MigrateReset(dbURL, migrationsPath string) error {
	m, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	log.Warn().Msg("Resetting database - this will DROP ALL TABLES")

	// Drop everything
	if err := m.Drop(); err != nil {
		return fmt.Errorf("failed to drop database: %w", err)
	}

	log.Info().Msg("Database dropped, re-running migrations")

	// Re-create migrator after drop
	m2, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m2.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	// Run all migrations
	if err := m2.Up(); err != nil {
		return fmt.Errorf("failed to run migrations after reset: %w", err)
	}

	version, _, _ := m2.Version()
	log.Info().Uint("version", version).Msg("Database reset complete")
	return nil
}

// ForceMigrationVersion sets the migration version without running migrations
// Use this to fix dirty state issues
func ForceMigrationVersion(dbURL, migrationsPath string, version int) error {
	m, err := CreateMigrator(dbURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			log.Error().Err(sourceErr).AnErr("db_err", dbErr).Msg("error closing migrator")
		}
	}()

	log.Warn().Int("version", version).Msg("Forcing migration version (use with caution)")

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force version: %w", err)
	}

	log.Info().Int("version", version).Msg("Migration version forced")
	return nil
}

// ValidateMigrations checks if migration files are valid
func ValidateMigrations(migrationsPath string) error {
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		return fmt.Errorf("migrations directory does not exist: %s", migrationsPath)
	}

	files, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no migration files found in %s", migrationsPath)
	}

	// Check for paired up/down files. Skip non-migration entries
	// (subdirectories like sqlite/, helper files like embed.go) — the
	// length guards prevent slice-out-of-bounds on names shorter than the
	// suffix we're comparing.
	upFiles := 0
	downFiles := 0
	for _, file := range files {
		name := file.Name()
		switch {
		case len(name) >= 8 && name[len(name)-8:] == "down.sql":
			downFiles++
		case len(name) >= 6 && name[len(name)-6:] == "up.sql":
			upFiles++
		}
	}

	if upFiles != downFiles {
		return fmt.Errorf("migration files mismatch: %d up files, %d down files", upFiles, downFiles)
	}

	log.Info().
		Int("migrations", upFiles).
		Str("path", migrationsPath).
		Msg("Migration files validated")

	return nil
}
