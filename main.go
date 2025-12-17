package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go_backend_project/admin"
	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var globalDB *gorm.DB
var globalScheduler *scheduler.Scheduler
var globalAuthController *admin.AuthController
var authControllerMutex sync.RWMutex

// InitStatus tracks the initialization state of the application
type InitStatus struct {
	mu           sync.RWMutex
	isReady      bool
	dbConnected  bool
	message      string
	startTime    time.Time
	lastError    string
	retryCount   int
}

var initStatus = &InitStatus{
	startTime: time.Now(),
	message:   "Starting initialization...",
}

func (s *InitStatus) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isReady = ready
	if ready {
		s.message = "System is ready"
	}
}

func (s *InitStatus) SetDBConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dbConnected = connected
}

func (s *InitStatus) SetMessage(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = msg
}

func (s *InitStatus) SetError(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err
}

func (s *InitStatus) IncrementRetry() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryCount++
	return s.retryCount
}

func (s *InitStatus) GetStatus() (ready bool, dbConnected bool, msg string, elapsed time.Duration, lastErr string, retries int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isReady, s.dbConnected, s.message, time.Since(s.startTime), s.lastError, s.retryCount
}

func main() {
	log.Println("=== CPLS Backend Starting ===")

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("LoadConfig failed: %v", err)
	}

	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Load templates with error handling (don't fail if templates can't be loaded)
	router.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	})
	if err := loadTemplates(router); err != nil {
		log.Printf("Warning: Failed to load templates: %v", err)
	}

	// Health check - always available (responds immediately for Cloud Run health checks)
	router.GET("/health", func(c *gin.Context) {
		ready, dbConnected, msg, elapsed, lastErr, retries := initStatus.GetStatus()
		
		status := "initializing"
		if ready {
			status = "ready"
		} else if lastErr != "" {
			status = "error"
		}
		
		dbStatus := "disconnected"
		if dbConnected && globalDB != nil {
			dbStatus = "connected"
		}
		
		c.JSON(200, gin.H{
			"status":      status,
			"db_status":   dbStatus,
			"message":     msg,
			"uptime_sec":  int(elapsed.Seconds()),
			"retries":     retries,
		})
	})
	
	// Readiness check endpoint - returns 200 only when fully ready
	router.GET("/ready", func(c *gin.Context) {
		ready, _, _, _, _, _ := initStatus.GetStatus()
		if ready {
			c.JSON(200, gin.H{"status": "ready"})
		} else {
			c.JSON(503, gin.H{"status": "not_ready"})
		}
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "CPLS Backend API",
		})
	})

	// Set up the auth controller setter callback for routes package
	routes.AuthControllerSetter = func(ac *admin.AuthController) {
		authControllerMutex.Lock()
		globalAuthController = ac
		authControllerMutex.Unlock()
	}

	// Setup initial admin routes before server starts (so /admin/login is always available)
	setupInitialAdminRoutes(router)

	// Start server IMMEDIATELY to respond to Cloud Run health checks
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: router,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	log.Println("=== Server is ready to accept connections ===")

	// Initialize database and routes in background AFTER server is listening
	go initializeApp(router)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Stop scheduler if running
	if globalScheduler != nil {
		globalScheduler.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

// loadTemplates loads HTML templates with error recovery
func loadTemplates(router *gin.Engine) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("template loading panic: %v", r)
		}
	}()
	router.LoadHTMLGlob("admin/templates/*.html")
	return nil
}

// initializeApp performs database connection and route setup after server starts
func initializeApp(router *gin.Engine) {
	log.Println("Initializing application...")
	initStatus.SetMessage("Connecting to database...")

	// Try to connect to database with retry mechanism
	db := connectDBWithRetry(5, 2*time.Second)
	
	if db != nil {
		globalDB = db
		initStatus.SetDBConnected(true)
		initStatus.SetMessage("Running database migrations...")
		
		// Run migrations
		log.Println("Running migrations...")
		runMigrations(globalDB)

		initStatus.SetMessage("Setting up routes...")
		
		// Setup full routes
		routes.SetupRoutes(router, globalDB)

		// Start scheduler
		initStatus.SetMessage("Starting scheduler...")
		globalScheduler = scheduler.NewScheduler(globalDB)
		globalScheduler.Start()

		initStatus.SetReady(true)
		log.Println("=== Application fully initialized ===")
	} else {
		initStatus.SetMessage("Running in maintenance mode (database unavailable)")
		initStatus.SetError("Failed to connect to database after retries")
		
		// Setup maintenance routes only
		setupMaintenanceRoutes(router)
		log.Println("=== Application started in maintenance mode (no database) ===")
	}
}

// connectDBWithRetry attempts to connect to the database with exponential backoff
func connectDBWithRetry(maxRetries int, initialDelay time.Duration) *gorm.DB {
	delay := initialDelay
	
	for i := 0; i < maxRetries; i++ {
		retryNum := initStatus.IncrementRetry()
		log.Printf("Database connection attempt %d/%d...", retryNum, maxRetries)
		initStatus.SetMessage(fmt.Sprintf("Connecting to database (attempt %d/%d)...", retryNum, maxRetries))
		
		db, err := config.InitDB()
		if err == nil {
			log.Println("Database connected successfully!")
			return db
		}
		
		log.Printf("Database connection failed (attempt %d): %v", retryNum, err)
		initStatus.SetError(err.Error())
		
		if i < maxRetries-1 {
			log.Printf("Retrying in %v...", delay)
			initStatus.SetMessage(fmt.Sprintf("Connection failed, retrying in %v...", delay))
			time.Sleep(delay)
			delay = delay * 2 // Exponential backoff
			if delay > 30*time.Second {
				delay = 30 * time.Second // Cap at 30 seconds
			}
		}
	}
	
	log.Printf("Failed to connect to database after %d attempts", maxRetries)
	return nil
}

func runMigrations(db *gorm.DB) {
	if err := models.MigrateStockModels(db); err != nil {
		log.Printf("MigrateStockModels: %v", err)
	}
	if err := models.MigrateTradingModels(db); err != nil {
		log.Printf("MigrateTradingModels: %v", err)
	}
	if err := models.MigrateUserModels(db); err != nil {
		log.Printf("MigrateUserModels: %v", err)
	}
	if err := models.MigrateSubscriptionModels(db); err != nil {
		log.Printf("MigrateSubscriptionModels: %v", err)
	}
	if err := models.MigrateAdminModels(db); err != nil {
		log.Printf("MigrateAdminModels: %v", err)
	}
	if err := models.SeedDefaultAdminUser(db); err != nil {
		log.Printf("SeedDefaultAdminUser: %v", err)
	}
	log.Println("Migrations completed")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func setupMaintenanceRoutes(router *gin.Engine) {
	// Use NoRoute handler for unknown paths instead of wildcard to avoid conflicts
	router.NoRoute(func(c *gin.Context) {
		// Check if it's an admin route
		if len(c.Request.URL.Path) >= 6 && c.Request.URL.Path[:6] == "/admin" {
			c.Redirect(http.StatusFound, "/admin/login")
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
	})

	// Note: admin login routes are registered in setupInitialAdminRoutes
	// Only register the redirect route here
	admin := router.Group("/admin")
	{
		admin.GET("", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
	}
}

// setupInitialAdminRoutes registers admin login routes before server starts
// This ensures /admin/login is always available, even during DB initialization
func setupInitialAdminRoutes(router *gin.Engine) {
	adminGroup := router.Group("/admin")
	{
		// Status endpoint for AJAX polling during initialization
		adminGroup.GET("/status", func(c *gin.Context) {
			ready, dbConnected, msg, elapsed, lastErr, retries := initStatus.GetStatus()
			c.JSON(200, gin.H{
				"ready":        ready,
				"db_connected": dbConnected,
				"message":      msg,
				"uptime_sec":   int(elapsed.Seconds()),
				"last_error":   lastErr,
				"retries":      retries,
			})
		})
		
		adminGroup.GET("/login", func(c *gin.Context) {
			// If auth controller is set (DB connected and routes initialized), use it
			authControllerMutex.RLock()
			ac := globalAuthController
			authControllerMutex.RUnlock()
			if ac != nil {
				ac.LoginPage(c)
				return
			}
			// DB is not yet connected, show initializing message with status info
			ready, _, msg, elapsed, lastErr, retries := initStatus.GetStatus()
			
			errorMsg := fmt.Sprintf("System is initializing... (%s)", msg)
			if lastErr != "" && retries > 0 {
				errorMsg = fmt.Sprintf("Connecting to database (attempt %d)... Last error: %s", retries, lastErr)
			}
			if elapsed > 60*time.Second && !ready {
				errorMsg = fmt.Sprintf("System initialization is taking longer than expected. Please wait... (%s)", msg)
			}
			
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error":        errorMsg,
				"initializing": true,
			})
		})
		adminGroup.POST("/login", func(c *gin.Context) {
			// If auth controller is set (DB connected and routes initialized), use it
			authControllerMutex.RLock()
			ac := globalAuthController
			authControllerMutex.RUnlock()
			if ac != nil {
				ac.Login(c)
				return
			}
			// DB is not yet connected, cannot login
			_, _, msg, _, _, _ := initStatus.GetStatus()
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error":        fmt.Sprintf("System is initializing. Please wait... (%s)", msg),
				"initializing": true,
			})
		})
	}
}
