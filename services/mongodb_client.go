package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDB collection names
const (
	MongoDBName               = "cpls_stock"
	MongoStockListCollection  = "stock_list"
	MongoPriceDataCollection  = "price_data"
	MongoIndicatorsCollection = "indicators"
)

// MongoDBClient handles MongoDB Atlas connection and operations
type MongoDBClient struct {
	client      *mongo.Client
	database    *mongo.Database
	mu          sync.RWMutex
	isConnected bool
	uriSet      bool   // Whether MONGODB_URI is configured
	lastError   string // Last connection error message
}

// MongoStockList represents stock list document in MongoDB
type MongoStockList struct {
	ID        string          `bson:"_id"`
	UpdatedAt time.Time       `bson:"updated_at"`
	Count     int             `bson:"count"`
	Stocks    []VNDirectStock `bson:"stocks"`
}

// MongoPriceData represents price data document in MongoDB
type MongoPriceData struct {
	Code        string           `bson:"_id"`
	UpdatedAt   time.Time        `bson:"updated_at"`
	DataCount   int              `bson:"data_count"`
	Prices      []StockPriceData `bson:"prices"`
	Indicators  *StockIndicators `bson:"indicators,omitempty"`
}

// MongoIndicatorSummary represents indicators summary document in MongoDB
type MongoIndicatorSummary struct {
	ID        string                              `bson:"_id"`
	UpdatedAt time.Time                           `bson:"updated_at"`
	Count     int                                 `bson:"count"`
	Stocks    map[string]*ExtendedStockIndicators `bson:"stocks"`
}

// Global MongoDB client instance
var GlobalMongoClient *MongoDBClient

// InitMongoDBClient initializes the MongoDB client
func InitMongoDBClient() error {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("MONGODB_URI not set, MongoDB storage disabled")
		GlobalMongoClient = &MongoDBClient{
			uriSet:    false,
			lastError: "MONGODB_URI environment variable not set",
		}
		return nil
	}

	// Initialize with URI set flag
	GlobalMongoClient = &MongoDBClient{
		uriSet: true,
	}

	return GlobalMongoClient.Connect()
}

// Connect establishes connection to MongoDB Atlas
func (m *MongoDBClient) Connect() error {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		m.lastError = "MONGODB_URI environment variable not set"
		return fmt.Errorf("%s", m.lastError)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure client options with retry
	clientOptions := options.Client().
		ApplyURI(mongoURI).
		SetServerAPIOptions(options.ServerAPI(options.ServerAPIVersion1)).
		SetMaxPoolSize(10).
		SetMinPoolSize(2).
		SetMaxConnIdleTime(30 * time.Second).
		SetConnectTimeout(30 * time.Second).
		SetRetryWrites(true).
		SetRetryReads(true)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		m.lastError = fmt.Sprintf("Failed to connect: %v", err)
		log.Printf("Failed to connect to MongoDB Atlas: %v", err)
		return err
	}

	// Verify connection with ping
	if err := client.Ping(ctx, nil); err != nil {
		m.lastError = fmt.Sprintf("Failed to ping: %v", err)
		log.Printf("Failed to ping MongoDB Atlas: %v", err)
		// Disconnect on ping failure
		client.Disconnect(ctx)
		return err
	}

	m.mu.Lock()
	m.client = client
	m.database = client.Database(MongoDBName)
	m.isConnected = true
	m.lastError = ""
	m.mu.Unlock()

	// Create indexes
	m.createIndexes()

	log.Println("MongoDB Atlas connected successfully")
	return nil
}

// Reconnect attempts to reconnect to MongoDB Atlas
func (m *MongoDBClient) Reconnect() error {
	m.mu.Lock()
	if m.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		m.client.Disconnect(ctx)
		cancel()
	}
	m.isConnected = false
	m.mu.Unlock()

	return m.Connect()
}

// IsConfigured returns whether MongoDB is configured and connected
func (m *MongoDBClient) IsConfigured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isConnected
}

// IsURISet returns whether MONGODB_URI environment variable is set
func (m *MongoDBClient) IsURISet() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uriSet
}

// GetLastError returns the last connection error
func (m *MongoDBClient) GetLastError() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastError
}

// GetConnectionStatus returns detailed connection status
func (m *MongoDBClient) GetConnectionStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := map[string]interface{}{
		"uri_set":   m.uriSet,
		"connected": m.isConnected,
	}

	if m.lastError != "" {
		status["error"] = m.lastError
	}

	return status
}

// Close closes the MongoDB connection
func (m *MongoDBClient) Close() error {
	if m.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return m.client.Disconnect(ctx)
	}
	return nil
}

// createIndexes creates necessary indexes for collections
func (m *MongoDBClient) createIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Price data collection - index on updated_at for sorting
	priceCollection := m.database.Collection(MongoPriceDataCollection)
	priceCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "updated_at", Value: -1}},
	})

	log.Println("MongoDB indexes created")
}

// ==================== Stock List Operations ====================

// SaveStockList saves the stock list to MongoDB
func (m *MongoDBClient) SaveStockList(stocks []VNDirectStock) error {
	if !m.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	doc := MongoStockList{
		ID:        "stock_list",
		UpdatedAt: time.Now(),
		Count:     len(stocks),
		Stocks:    stocks,
	}

	collection := m.database.Collection(MongoStockListCollection)
	opts := options.Replace().SetUpsert(true)

	_, err := collection.ReplaceOne(ctx, bson.M{"_id": "stock_list"}, doc, opts)
	if err != nil {
		return fmt.Errorf("failed to save stock list to MongoDB: %w", err)
	}

	log.Printf("Saved %d stocks to MongoDB Atlas", len(stocks))
	return nil
}

// LoadStockList loads the stock list from MongoDB
func (m *MongoDBClient) LoadStockList() ([]VNDirectStock, error) {
	if !m.IsConfigured() {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := m.database.Collection(MongoStockListCollection)

	var doc MongoStockList
	err := collection.FindOne(ctx, bson.M{"_id": "stock_list"}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("stock list not found in MongoDB")
		}
		return nil, fmt.Errorf("failed to load stock list from MongoDB: %w", err)
	}

	log.Printf("Loaded %d stocks from MongoDB Atlas (updated: %s)", len(doc.Stocks), doc.UpdatedAt.Format(time.RFC3339))
	return doc.Stocks, nil
}

// GetStockListMetadata returns metadata about the stock list
func (m *MongoDBClient) GetStockListMetadata() (count int, updatedAt time.Time, err error) {
	if !m.IsConfigured() {
		return 0, time.Time{}, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := m.database.Collection(MongoStockListCollection)

	var doc struct {
		Count     int       `bson:"count"`
		UpdatedAt time.Time `bson:"updated_at"`
	}
	err = collection.FindOne(ctx, bson.M{"_id": "stock_list"}, options.FindOne().SetProjection(bson.M{"count": 1, "updated_at": 1})).Decode(&doc)
	if err != nil {
		return 0, time.Time{}, err
	}

	return doc.Count, doc.UpdatedAt, nil
}

// ==================== Price Data Operations ====================

// SavePriceData saves price data for a single stock to MongoDB
func (m *MongoDBClient) SavePriceData(code string, priceFile *StockPriceFile) error {
	if !m.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	doc := MongoPriceData{
		Code:       code,
		UpdatedAt:  time.Now(),
		DataCount:  len(priceFile.Prices),
		Prices:     priceFile.Prices,
		Indicators: priceFile.Indicators,
	}

	collection := m.database.Collection(MongoPriceDataCollection)
	opts := options.Replace().SetUpsert(true)

	_, err := collection.ReplaceOne(ctx, bson.M{"_id": code}, doc, opts)
	if err != nil {
		return fmt.Errorf("failed to save price data for %s to MongoDB: %w", code, err)
	}

	return nil
}

// SaveAllPriceData saves all price data to MongoDB (batch operation)
func (m *MongoDBClient) SaveAllPriceData(priceFiles map[string]*StockPriceFile) error {
	if !m.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	collection := m.database.Collection(MongoPriceDataCollection)

	// Prepare bulk operations
	var operations []mongo.WriteModel
	now := time.Now()

	for code, priceFile := range priceFiles {
		if priceFile == nil {
			continue
		}

		doc := MongoPriceData{
			Code:       code,
			UpdatedAt:  now,
			DataCount:  len(priceFile.Prices),
			Prices:     priceFile.Prices,
			Indicators: priceFile.Indicators,
		}

		operation := mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": code}).
			SetReplacement(doc).
			SetUpsert(true)

		operations = append(operations, operation)
	}

	if len(operations) == 0 {
		return nil
	}

	// Execute bulk write in batches of 100
	batchSize := 100
	for i := 0; i < len(operations); i += batchSize {
		end := i + batchSize
		if end > len(operations) {
			end = len(operations)
		}

		_, err := collection.BulkWrite(ctx, operations[i:end])
		if err != nil {
			return fmt.Errorf("failed to bulk save price data to MongoDB: %w", err)
		}
	}

	log.Printf("Saved price data for %d stocks to MongoDB Atlas", len(operations))
	return nil
}

// LoadPriceData loads price data for a single stock from MongoDB
func (m *MongoDBClient) LoadPriceData(code string) (*StockPriceFile, error) {
	if !m.IsConfigured() {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := m.database.Collection(MongoPriceDataCollection)

	var doc MongoPriceData
	err := collection.FindOne(ctx, bson.M{"_id": code}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("price data not found for %s in MongoDB", code)
		}
		return nil, fmt.Errorf("failed to load price data for %s from MongoDB: %w", code, err)
	}

	return &StockPriceFile{
		Code:        doc.Code,
		LastUpdated: doc.UpdatedAt.Format(time.RFC3339),
		DataCount:   doc.DataCount,
		Prices:      doc.Prices,
		Indicators:  doc.Indicators,
	}, nil
}

// LoadAllPriceData loads all price data from MongoDB
func (m *MongoDBClient) LoadAllPriceData() (map[string]*StockPriceFile, error) {
	if !m.IsConfigured() {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	collection := m.database.Collection(MongoPriceDataCollection)

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to query price data from MongoDB: %w", err)
	}
	defer cursor.Close(ctx)

	result := make(map[string]*StockPriceFile)
	for cursor.Next(ctx) {
		var doc MongoPriceData
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		result[doc.Code] = &StockPriceFile{
			Code:        doc.Code,
			LastUpdated: doc.UpdatedAt.Format(time.RFC3339),
			DataCount:   doc.DataCount,
			Prices:      doc.Prices,
			Indicators:  doc.Indicators,
		}
	}

	log.Printf("Loaded price data for %d stocks from MongoDB Atlas", len(result))
	return result, nil
}

// GetPriceDataCount returns the count of price data documents
func (m *MongoDBClient) GetPriceDataCount() (int64, error) {
	if !m.IsConfigured() {
		return 0, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := m.database.Collection(MongoPriceDataCollection)
	return collection.CountDocuments(ctx, bson.M{})
}

// ==================== Indicators Operations ====================

// SaveIndicatorSummary saves all indicators to MongoDB
func (m *MongoDBClient) SaveIndicatorSummary(indicators map[string]*ExtendedStockIndicators) error {
	if !m.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	doc := MongoIndicatorSummary{
		ID:        "indicators_summary",
		UpdatedAt: time.Now(),
		Count:     len(indicators),
		Stocks:    indicators,
	}

	collection := m.database.Collection(MongoIndicatorsCollection)
	opts := options.Replace().SetUpsert(true)

	_, err := collection.ReplaceOne(ctx, bson.M{"_id": "indicators_summary"}, doc, opts)
	if err != nil {
		return fmt.Errorf("failed to save indicators to MongoDB: %w", err)
	}

	log.Printf("Saved indicators for %d stocks to MongoDB Atlas", len(indicators))
	return nil
}

// LoadIndicatorSummary loads all indicators from MongoDB
func (m *MongoDBClient) LoadIndicatorSummary() (map[string]*ExtendedStockIndicators, time.Time, error) {
	if !m.IsConfigured() {
		return nil, time.Time{}, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := m.database.Collection(MongoIndicatorsCollection)

	var doc MongoIndicatorSummary
	err := collection.FindOne(ctx, bson.M{"_id": "indicators_summary"}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, time.Time{}, fmt.Errorf("indicators not found in MongoDB")
		}
		return nil, time.Time{}, fmt.Errorf("failed to load indicators from MongoDB: %w", err)
	}

	log.Printf("Loaded indicators for %d stocks from MongoDB Atlas (updated: %s)", len(doc.Stocks), doc.UpdatedAt.Format(time.RFC3339))
	return doc.Stocks, doc.UpdatedAt, nil
}

// GetIndicatorsMetadata returns metadata about the indicators
func (m *MongoDBClient) GetIndicatorsMetadata() (count int, updatedAt time.Time, err error) {
	if !m.IsConfigured() {
		return 0, time.Time{}, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := m.database.Collection(MongoIndicatorsCollection)

	var doc struct {
		Count     int       `bson:"count"`
		UpdatedAt time.Time `bson:"updated_at"`
	}
	err = collection.FindOne(ctx, bson.M{"_id": "indicators_summary"}, options.FindOne().SetProjection(bson.M{"count": 1, "updated_at": 1})).Decode(&doc)
	if err != nil {
		return 0, time.Time{}, err
	}

	return doc.Count, doc.UpdatedAt, nil
}

// ==================== Utility Functions ====================

// SyncLocalToMongoDB syncs all local data to MongoDB
func (m *MongoDBClient) SyncLocalToMongoDB() error {
	if !m.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	log.Println("Starting sync from local storage to MongoDB Atlas...")

	// 1. Sync stock list
	stocks, err := LoadStocksFromFile()
	if err == nil && len(stocks) > 0 {
		if err := m.SaveStockList(stocks); err != nil {
			log.Printf("Warning: failed to sync stock list to MongoDB: %v", err)
		}
	}

	// 2. Sync price data
	priceFiles := make(map[string]*StockPriceFile)
	if GlobalPriceService != nil {
		files, err := os.ReadDir(StockPriceDir)
		if err == nil {
			for _, file := range files {
				if file.IsDir() || len(file.Name()) < 6 {
					continue
				}
				code := file.Name()[:len(file.Name())-5] // Remove .json
				if priceFile, err := GlobalPriceService.LoadStockPrice(code); err == nil {
					priceFiles[code] = priceFile
				}
			}
		}
	}
	if len(priceFiles) > 0 {
		if err := m.SaveAllPriceData(priceFiles); err != nil {
			log.Printf("Warning: failed to sync price data to MongoDB: %v", err)
		}
	}

	// 3. Sync indicators
	if GlobalIndicatorService != nil {
		summary, err := GlobalIndicatorService.LoadIndicatorSummary()
		if err == nil && summary != nil && len(summary.Stocks) > 0 {
			if err := m.SaveIndicatorSummary(summary.Stocks); err != nil {
				log.Printf("Warning: failed to sync indicators to MongoDB: %v", err)
			}
		}
	}

	log.Println("Completed sync to MongoDB Atlas")
	return nil
}

// SyncMongoDBToLocal syncs all data from MongoDB to local storage
func (m *MongoDBClient) SyncMongoDBToLocal() error {
	if !m.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	log.Println("Starting sync from MongoDB Atlas to local storage...")

	// 1. Sync stock list
	stocks, err := m.LoadStockList()
	if err == nil && len(stocks) > 0 {
		if err := SaveStocksToFile(stocks); err != nil {
			log.Printf("Warning: failed to save stock list locally: %v", err)
		}
	}

	// 2. Sync price data
	priceFiles, err := m.LoadAllPriceData()
	if err == nil && len(priceFiles) > 0 {
		for code, priceFile := range priceFiles {
			if priceFile == nil {
				continue
			}
			data, err := json.MarshalIndent(priceFile, "", "  ")
			if err != nil {
				continue
			}
			filePath := fmt.Sprintf("%s/%s.json", StockPriceDir, code)
			os.WriteFile(filePath, data, 0644)
		}
		log.Printf("Saved %d price files locally", len(priceFiles))
	}

	// 3. Sync indicators
	indicators, updatedAt, err := m.LoadIndicatorSummary()
	if err == nil && len(indicators) > 0 {
		summary := IndicatorSummaryFile{
			UpdatedAt: updatedAt.Format(time.RFC3339),
			Count:     len(indicators),
			Stocks:    indicators,
		}
		data, err := json.MarshalIndent(summary, "", "  ")
		if err == nil {
			os.WriteFile("data/indicators_summary.json", data, 0644)
			log.Printf("Saved indicators summary locally (%d stocks)", len(indicators))
		}
	}

	log.Println("Completed sync from MongoDB Atlas to local storage")
	return nil
}

// GetMongoDBStats returns statistics about MongoDB collections
func (m *MongoDBClient) GetMongoDBStats() (map[string]interface{}, error) {
	if !m.IsConfigured() {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	stats := make(map[string]interface{})

	// Stock list stats
	stockCount, stockUpdated, err := m.GetStockListMetadata()
	if err == nil {
		stats["stock_list"] = map[string]interface{}{
			"count":      stockCount,
			"updated_at": stockUpdated.Format(time.RFC3339),
		}
	}

	// Price data stats
	priceCount, err := m.GetPriceDataCount()
	if err == nil {
		stats["price_data"] = map[string]interface{}{
			"count": priceCount,
		}
	}

	// Indicators stats
	indCount, indUpdated, err := m.GetIndicatorsMetadata()
	if err == nil {
		stats["indicators"] = map[string]interface{}{
			"count":      indCount,
			"updated_at": indUpdated.Format(time.RFC3339),
		}
	}

	stats["connected"] = m.IsConfigured()

	return stats, nil
}
