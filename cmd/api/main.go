package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	// Application Layer
	appService "reminder/internal/application/service"
	// Domain Layer (Repositories used by services)

	// Infrastructure Layer
	"reminder/internal/infrastructure/database/sqlite"
	lineClient "reminder/internal/infrastructure/line"
	"reminder/internal/infrastructure/scheduler"

	// Interfaces Layer
	"reminder/internal/interfaces/api/handler"
	"reminder/internal/interfaces/api/router"

	// Packages
	appLogger "reminder/internal/pkg/logger"

	_ "github.com/joho/godotenv/autoload" // Automatically load .env file
)

func gracefulShutdown(apiServer *http.Server, schedulerService appService.SchedulerService, done chan bool) {
	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Listen for the interrupt signal.
	<-ctx.Done()

	log.Println("Shutting down gracefully, press Ctrl+C again to force")

	// Stop the scheduler first
	log.Println("Stopping scheduler...")
	schedulerService.Stop()
	log.Println("Scheduler stopped.")

	// Close database connection
	log.Println("Closing database connection...")
	if err := sqlite.CloseDB(); err != nil {
		log.Printf("Error closing database: %v", err)
	} else {
		log.Println("Database connection closed.")
	}

	// Shutdown HTTP server
	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown with error: %v", err)
	}

	log.Println("Server exiting")

	// Notify the main goroutine that the shutdown is complete
	done <- true
}

func main() {
	// --- Initialization ---
	appLog := appLogger.New()
	appLog.Info("Logger initialized.")

	// Load Environment Variables (using autoload)
	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = "8080" // Default port
		appLog.Warn("PORT environment variable not set, defaulting to 8080")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		appLog.Error("Invalid PORT environment variable", err)
		os.Exit(1)
	}
	// Other env vars (DB_URL, LINE secrets) are loaded by their respective modules

	// --- Infrastructure ---
	db := sqlite.NewDB()
	userRepo := sqlite.NewUserRepository(db)
	reminderRepo := sqlite.NewReminderRepository(db)
	appLog.Info("Database and repositories initialized.")

	line := lineClient.NewClient(appLog)
	cronScheduler := scheduler.NewScheduler(appLog)

	// --- Application Services ---
	// Initialize services (order matters for dependency injection workaround)
	schedulerSvc := appService.NewSchedulerService(cronScheduler, reminderRepo, appLog)
	// ReminderService needs SchedulerService, UserService needs ReminderRepo
	userSvc := appService.NewUserService(userRepo, reminderRepo, appLog)
	reminderSvc := appService.NewReminderService(reminderRepo, userRepo, schedulerSvc, line, appLog)
	appLog.Info("Application services initialized.")

	// --- Initialize Schedules ---
	appLog.Info("Initializing reminder schedules...")
	if err := schedulerSvc.InitializeSchedules(context.Background()); err != nil {
		// Log the error but continue starting the server
		appLog.Error("Failed to initialize schedules on startup", err)
	} else {
		appLog.Info("Reminder schedules initialized.")
	}

	// --- API Handlers ---
	lineHandler := handler.NewLineHandler(line, userSvc, reminderSvc, appLog)
	appLog.Info("API handlers initialized.")

	// --- Router ---
	routerCfg := &router.Config{
		LineHandler: lineHandler,
		Logger:      appLog,
	}
	echoRouter := router.NewRouter(routerCfg)

	// --- HTTP Server ---
	apiServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      echoRouter,
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// --- Start Server & Shutdown Handling ---
	done := make(chan bool, 1)
	go gracefulShutdown(apiServer, schedulerSvc, done) // Pass scheduler service for stopping

	appLog.Info(fmt.Sprintf("Server starting on port %d", port))
	err = apiServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		appLog.Error("HTTP server ListenAndServe error", err)
		panic(fmt.Sprintf("http server error: %s", err))
	}

	// Wait for graceful shutdown signal
	<-done
	appLog.Info("Graceful shutdown complete.")
}
