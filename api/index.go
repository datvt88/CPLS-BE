package main

import (
	"net/http"

	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"

	"github.com/gin-gonic/gin"
)

var router *gin.Engine

func init() {
	// Load configuration
	_, err := config.LoadConfig()
	if err != nil {
		panic("Failed to load configuration")
	}

	// Initialize database
	db, err := config.InitDB()
	if err != nil {
		panic("Failed to connect to database")
	}

	// Run migrations
	models.MigrateStockModels(db)
	models.MigrateTradingModels(db)
	models.MigrateUserModels(db)
	models.MigrateSubscriptionModels(db)

	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Initialize router
	router = gin.New()
	router.Use(gin.Recovery())

	// CORS middleware
	router.Use(corsMiddleware())

	// Setup routes
	routes.SetupRoutes(router, db)
}

// Handler is the Vercel serverless function handler
func Handler(w http.ResponseWriter, r *http.Request) {
	router.ServeHTTP(w, r)
}

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
