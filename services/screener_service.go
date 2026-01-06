package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ScreenerService provides stock screening functionality using MongoDB
type ScreenerService struct {
	mongoDB *mongo.Database
}

// FilterParams represents screener filter parameters
type FilterParams struct {
	// Price filters
	MinPrice *float64 `json:"min_price,omitempty"`
	MaxPrice *float64 `json:"max_price,omitempty"`

	// Volume filters
	MinVolume *int64 `json:"min_volume,omitempty"`
	MaxVolume *int64 `json:"max_volume,omitempty"`

	// Technical indicator filters
	MinRSI *float64 `json:"min_rsi,omitempty"`
	MaxRSI *float64 `json:"max_rsi,omitempty"`

	// Moving average filters
	PriceAboveMA20  *bool `json:"price_above_ma20,omitempty"`
	PriceAboveMA50  *bool `json:"price_above_ma50,omitempty"`
	PriceAboveMA200 *bool `json:"price_above_ma200,omitempty"`

	// MACD filter
	MACDBullish *bool `json:"macd_bullish,omitempty"`

	// Exchange filter
	Exchange []string `json:"exchange,omitempty"`

	// Pagination
	Page  int `json:"page"`
	Limit int `json:"limit"`

	// Sorting
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"` // "asc" or "desc"
}

// ScreenerResult represents a single screener result
type ScreenerResult struct {
	Code        string               `json:"code" bson:"_id"`
	Exchange    string               `json:"exchange" bson:"exchange"`
	Name        string               `json:"name" bson:"name"`
	LatestPrice float64              `json:"latest_price" bson:"latest_price"`
	Volume      int64                `json:"volume" bson:"volume"`
	Indicators  TechnicalIndicators  `json:"indicators" bson:"indicators"`
	UpdatedAt   time.Time            `json:"updated_at" bson:"updated_at"`
}

// ScreenerResponse represents the API response
type ScreenerResponse struct {
	Results    []ScreenerResult `json:"results"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	TotalPages int              `json:"total_pages"`
}

// NewScreenerService creates a new screener service
func NewScreenerService(mongoDB *mongo.Database) *ScreenerService {
	return &ScreenerService{
		mongoDB: mongoDB,
	}
}

// GetScreener performs stock screening based on filter parameters
func (ss *ScreenerService) GetScreener(filter FilterParams) (*ScreenerResponse, error) {
	if ss.mongoDB == nil {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	// Set defaults
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "desc"
	}
	if filter.SortBy == "" {
		filter.SortBy = "volume"
	}

	// Build MongoDB query
	query := ss.buildMongoQuery(filter)

	// Build sort
	sortOrder := -1 // descending
	if filter.SortOrder == "asc" {
		sortOrder = 1
	}

	sortField := "volume"
	switch filter.SortBy {
	case "price":
		sortField = "latest_price"
	case "rsi":
		sortField = "indicators.rsi_14"
	case "volume":
		sortField = "volume"
	}

	// Calculate pagination
	skip := int64((filter.Page - 1) * filter.Limit)
	limit := int64(filter.Limit)

	// Execute query
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := ss.mongoDB.Collection("stock_indicators")

	// Count total documents matching filter
	total, err := collection.CountDocuments(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	// Find documents with pagination and sorting
	findOptions := options.Find().
		SetSort(bson.D{{Key: sortField, Value: sortOrder}}).
		SetSkip(skip).
		SetLimit(limit)

	cursor, err := collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer cursor.Close(ctx)

	var results []ScreenerResult
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode results: %w", err)
	}

	// Calculate total pages
	totalPages := int(total) / filter.Limit
	if int(total)%filter.Limit > 0 {
		totalPages++
	}

	return &ScreenerResponse{
		Results:    results,
		Total:      total,
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: totalPages,
	}, nil
}

// buildMongoQuery builds a MongoDB query from filter parameters
func (ss *ScreenerService) buildMongoQuery(filter FilterParams) bson.M {
	query := bson.M{}

	// Price filters
	if filter.MinPrice != nil || filter.MaxPrice != nil {
		priceQuery := bson.M{}
		if filter.MinPrice != nil {
			priceQuery["$gte"] = *filter.MinPrice
		}
		if filter.MaxPrice != nil {
			priceQuery["$lte"] = *filter.MaxPrice
		}
		query["latest_price"] = priceQuery
	}

	// Volume filters
	if filter.MinVolume != nil || filter.MaxVolume != nil {
		volumeQuery := bson.M{}
		if filter.MinVolume != nil {
			volumeQuery["$gte"] = *filter.MinVolume
		}
		if filter.MaxVolume != nil {
			volumeQuery["$lte"] = *filter.MaxVolume
		}
		query["volume"] = volumeQuery
	}

	// RSI filters (e.g., RSI < 30 for oversold, RSI > 70 for overbought)
	if filter.MinRSI != nil || filter.MaxRSI != nil {
		rsiQuery := bson.M{}
		if filter.MinRSI != nil {
			rsiQuery["$gte"] = *filter.MinRSI
		}
		if filter.MaxRSI != nil {
			rsiQuery["$lte"] = *filter.MaxRSI
		}
		query["indicators.rsi_14"] = rsiQuery
	}

	// Price above MA filters - combine multiple conditions using $and
	var exprConditions []bson.M
	
	if filter.PriceAboveMA20 != nil && *filter.PriceAboveMA20 {
		exprConditions = append(exprConditions, bson.M{
			"$gt": bson.A{"$latest_price", "$indicators.ma_20"},
		})
	}

	if filter.PriceAboveMA50 != nil && *filter.PriceAboveMA50 {
		exprConditions = append(exprConditions, bson.M{
			"$gt": bson.A{"$latest_price", "$indicators.ma_50"},
		})
	}

	if filter.PriceAboveMA200 != nil && *filter.PriceAboveMA200 {
		exprConditions = append(exprConditions, bson.M{
			"$gt": bson.A{"$latest_price", "$indicators.ma_200"},
		})
	}

	// Combine all $expr conditions using $and
	if len(exprConditions) > 0 {
		if len(exprConditions) == 1 {
			query["$expr"] = exprConditions[0]
		} else {
			query["$expr"] = bson.M{
				"$and": exprConditions,
			}
		}
	}

	// MACD Bullish (histogram > 0)
	if filter.MACDBullish != nil && *filter.MACDBullish {
		query["indicators.macd_histogram"] = bson.M{"$gt": 0}
	}

	// Exchange filter
	if len(filter.Exchange) > 0 {
		query["exchange"] = bson.M{"$in": filter.Exchange}
	}

	return query
}

// GetOversoldStocks returns stocks with RSI < 30
func (ss *ScreenerService) GetOversoldStocks(limit int) (*ScreenerResponse, error) {
	maxRSI := 30.0
	return ss.GetScreener(FilterParams{
		MaxRSI:    &maxRSI,
		Page:      1,
		Limit:     limit,
		SortBy:    "rsi",
		SortOrder: "asc",
	})
}

// GetOverboughtStocks returns stocks with RSI > 70
func (ss *ScreenerService) GetOverboughtStocks(limit int) (*ScreenerResponse, error) {
	minRSI := 70.0
	return ss.GetScreener(FilterParams{
		MinRSI:    &minRSI,
		Page:      1,
		Limit:     limit,
		SortBy:    "rsi",
		SortOrder: "desc",
	})
}

// GetHighVolumeStocks returns stocks with volume > threshold
func (ss *ScreenerService) GetHighVolumeStocks(minVolume int64, limit int) (*ScreenerResponse, error) {
	return ss.GetScreener(FilterParams{
		MinVolume: &minVolume,
		Page:      1,
		Limit:     limit,
		SortBy:    "volume",
		SortOrder: "desc",
	})
}

// GetBullishStocks returns stocks with bullish indicators
func (ss *ScreenerService) GetBullishStocks(limit int) (*ScreenerResponse, error) {
	priceAboveMA20 := true
	macdBullish := true

	return ss.GetScreener(FilterParams{
		PriceAboveMA20: &priceAboveMA20,
		MACDBullish:    &macdBullish,
		Page:           1,
		Limit:          limit,
		SortBy:         "volume",
		SortOrder:      "desc",
	})
}

// GetStockIndicators returns indicator data for a specific stock
func (ss *ScreenerService) GetStockIndicators(code string) (*ScreenerResult, error) {
	if ss.mongoDB == nil {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := ss.mongoDB.Collection("stock_indicators")

	var result ScreenerResult
	err := collection.FindOne(ctx, bson.M{"_id": code}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("stock not found: %s", code)
		}
		return nil, fmt.Errorf("failed to get stock indicators: %w", err)
	}

	return &result, nil
}

// GetStatistics returns overall market statistics
func (ss *ScreenerService) GetStatistics() (map[string]interface{}, error) {
	if ss.mongoDB == nil {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := ss.mongoDB.Collection("stock_indicators")

	stats := make(map[string]interface{})

	// Total stocks
	total, err := collection.CountDocuments(ctx, bson.M{})
	if err == nil {
		stats["total_stocks"] = total
	}

	// Oversold stocks (RSI < 30)
	oversold, err := collection.CountDocuments(ctx, bson.M{"indicators.rsi_14": bson.M{"$lt": 30}})
	if err == nil {
		stats["oversold_stocks"] = oversold
	}

	// Overbought stocks (RSI > 70)
	overbought, err := collection.CountDocuments(ctx, bson.M{"indicators.rsi_14": bson.M{"$gt": 70}})
	if err == nil {
		stats["overbought_stocks"] = overbought
	}

	// Bullish stocks (MACD histogram > 0)
	bullish, err := collection.CountDocuments(ctx, bson.M{"indicators.macd_histogram": bson.M{"$gt": 0}})
	if err == nil {
		stats["bullish_stocks"] = bullish
	}

	// Average RSI
	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":        nil,
			"avg_rsi":    bson.M{"$avg": "$indicators.rsi_14"},
			"avg_volume": bson.M{"$avg": "$volume"},
			"avg_price":  bson.M{"$avg": "$latest_price"},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err == nil {
		defer cursor.Close(ctx)
		if cursor.Next(ctx) {
			var result bson.M
			if err := cursor.Decode(&result); err == nil {
				stats["avg_rsi"] = result["avg_rsi"]
				stats["avg_volume"] = result["avg_volume"]
				stats["avg_price"] = result["avg_price"]
			}
		}
	}

	log.Printf("Market statistics: %+v", stats)
	return stats, nil
}
