package sqlite

import (
	"fmt"
	"log"
	"os"
	"reminder/internal/domain/entity"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	dbInstance *gorm.DB
	once       sync.Once
)

// NewDB initializes the GORM database connection using SQLite.
// It ensures that the connection is established only once (singleton pattern).
func NewDB() *gorm.DB {
	once.Do(func() {
		dbURL := os.Getenv("BLUEPRINT_DB_URL")
		if dbURL == "" {
			dbURL = "test.db" // Default to test.db if not set
			log.Println("⚠️ WARN: BLUEPRINT_DB_URL environment variable not set, defaulting to 'test.db'")
		}

		// Configure GORM logger
		newLogger := gormlogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
			gormlogger.Config{
				SlowThreshold:             0,               // Log all SQL
				LogLevel:                  gormlogger.Info, // Log level Info
				IgnoreRecordNotFoundError: true,            // Ignore ErrRecordNotFound error for logger
				Colorful:                  true,            // Disable color
			},
		)

		db, err := gorm.Open(sqlite.Open(dbURL), &gorm.Config{
			Logger: newLogger,
		})
		if err != nil {
			log.Fatalf("🔴 ERROR: Failed to connect to database: %v", err)
		}

		log.Printf("Successfully connected to database: %s", dbURL)
		dbInstance = db

		// Auto-migrate the schema
		if err := AutoMigrate(dbInstance); err != nil {
			log.Fatalf("🔴 ERROR: Failed to auto-migrate database schema: %v", err)
		}
		log.Println("Database schema migration completed.")
	})
	return dbInstance
}

// AutoMigrate automatically migrates the database schema for the defined entities.
func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&entity.User{},
		&entity.Reminder{},
	)
	if err != nil {
		return fmt.Errorf("🔴 ERROR: schema migration failed: %w", err)
	}
	return nil
}

// CloseDB closes the database connection if it's open.
func CloseDB() error {
	if dbInstance != nil {
		sqlDB, err := dbInstance.DB()
		if err != nil {
			return fmt.Errorf("🔴 ERROR: failed to get underlying *sql.DB: %w", err)
		}
		log.Println("Closing database connection...")
		return sqlDB.Close()
	}
	return nil
}
