package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

// DatabaseConfig holds all database connections
type DatabaseConfig struct {
	PostgresDB *gorm.DB
	MongoDB    *mongo.Client
	MongoDBName string
}

// Global database config instance
var GlobalDBConfig *DatabaseConfig

// InitDatabases initializes both PostgreSQL (Supabase) and MongoDB connections
func InitDatabases() (*DatabaseConfig, error) {
	config := &DatabaseConfig{}

	// Initialize PostgreSQL (already handled by InitDB in config.go)
	// This is the existing GORM connection
	if DB != nil {
		config.PostgresDB = DB
		log.Println("✓ PostgreSQL (Supabase) connection ready")
	} else {
		log.Println("⚠ PostgreSQL connection not initialized")
	}

	// Initialize MongoDB if configured
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI != "" {
		mongoClient, err := initMongoDBConnection(mongoURI)
		if err != nil {
			log.Printf("⚠ MongoDB connection failed: %v", err)
			// Don't fail - MongoDB is optional
		} else {
			config.MongoDB = mongoClient
			config.MongoDBName = "cpls_stock"
			log.Println("✓ MongoDB Atlas connection ready")
		}
	} else {
		log.Println("⚠ MONGODB_URI not configured - MongoDB features disabled")
	}

	GlobalDBConfig = config
	return config, nil
}

// initMongoDBConnection creates a MongoDB client connection
func initMongoDBConnection(mongoURI string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure MongoDB client options
	clientOptions := options.Client().
		ApplyURI(mongoURI).
		SetServerAPIOptions(options.ServerAPI(options.ServerAPIVersion1)).
		SetMaxPoolSize(10).
		SetMinPoolSize(2).
		SetMaxConnIdleTime(30 * time.Second).
		SetConnectTimeout(30 * time.Second).
		SetRetryWrites(true).
		SetRetryReads(true)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection with ping
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return client, nil
}

// GetMongoDatabase returns the MongoDB database instance
func (dc *DatabaseConfig) GetMongoDatabase() *mongo.Database {
	if dc.MongoDB == nil {
		return nil
	}
	return dc.MongoDB.Database(dc.MongoDBName)
}

// GetMongoCollection returns a MongoDB collection
func (dc *DatabaseConfig) GetMongoCollection(collectionName string) *mongo.Collection {
	db := dc.GetMongoDatabase()
	if db == nil {
		return nil
	}
	return db.Collection(collectionName)
}

// IsMongoDBAvailable checks if MongoDB is connected
func (dc *DatabaseConfig) IsMongoDBAvailable() bool {
	if dc.MongoDB == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := dc.MongoDB.Ping(ctx, nil)
	return err == nil
}

// CloseConnections closes all database connections
func (dc *DatabaseConfig) CloseConnections() error {
	// Close MongoDB connection
	if dc.MongoDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := dc.MongoDB.Disconnect(ctx); err != nil {
			log.Printf("Error closing MongoDB connection: %v", err)
		} else {
			log.Println("MongoDB connection closed")
		}
	}

	// PostgreSQL connection is closed in main.go
	return nil
}
