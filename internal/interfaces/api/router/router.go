package router

import (
	"fmt"
	"net/http"
	"reminder/internal/interfaces/api/handler"
	"reminder/internal/pkg/logger"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config holds the dependencies for the router.
type Config struct {
	LineHandler *handler.LineHandler
	Logger      logger.Logger
}

// NewRouter creates and configures a new Echo router.
func NewRouter(cfg *Config) *echo.Echo {
	e := echo.New()

	// Middleware
	e.Use(middleware.RequestID())
	// Use custom logger that integrates with our logger interface
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:       true,
		LogStatus:    true,
		LogMethod:    true,
		LogHost:      true,
		LogLatency:   true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			cfg.Logger.Info(fmt.Sprintf("REQUEST: method=%s, uri=%s, status=%d, latency=%s, req_id=%s",
				v.Method, v.URI, v.Status, v.Latency, v.RequestID,
			))
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-Line-Signature"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Routes
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	// LINE Webhook Endpoint
	// Note: LINE Platform requires POST for webhook
	e.POST("/callback", cfg.LineHandler.HandleWebhook)

	cfg.Logger.Info("Router initialized with routes.")
	return e
}
