package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// SupabaseStorageService handles persistent storage in Supabase
type SupabaseStorageService struct {
	URL    string
	APIKey string
	client *http.Client
}

// Global storage service
var GlobalStorageService *SupabaseStorageService

// InitStorageService initializes the Supabase storage service
func InitStorageService() error {
	url := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_SERVICE_KEY")

	if url == "" || key == "" {
		log.Println("Supabase storage not configured, using local file storage")
		return nil
	}

	GlobalStorageService = &SupabaseStorageService{
		URL:    url,
		APIKey: key,
		client: &http.Client{Timeout: 30 * time.Second},
	}

	log.Println("Supabase Storage Service initialized")
	return nil
}

// IsConfigured returns true if Supabase storage is configured
func (s *SupabaseStorageService) IsConfigured() bool {
	return s != nil && s.URL != "" && s.APIKey != ""
}

// supabaseRequest makes a request to Supabase REST API
func (s *SupabaseStorageService) supabaseRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := fmt.Sprintf("%s/rest/v1/%s", s.URL, endpoint)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", s.APIKey)
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("supabase error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ==================== Stock Prices ====================

// StockPriceRow represents a row in stock_prices table
type StockPriceRow struct {
	ID        int64     `json:"id,omitempty"`
	Code      string    `json:"code"`
	Date      string    `json:"date"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
	Value     float64   `json:"value"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// SaveStockPrices saves price data to Supabase
func (s *SupabaseStorageService) SaveStockPrices(code string, prices []StockPriceData) error {
	if !s.IsConfigured() {
		return fmt.Errorf("supabase not configured")
	}

	// Convert to rows
	rows := make([]StockPriceRow, len(prices))
	for i, p := range prices {
		rows[i] = StockPriceRow{
			Code:   code,
			Date:   p.Date,
			Open:   p.Open,
			High:   p.High,
			Low:    p.Low,
			Close:  p.Close,
			Volume: int64(p.NmVolume),
			Value:  p.NmValue,
		}
	}

	// Delete existing prices for this stock
	_, err := s.supabaseRequest("DELETE", fmt.Sprintf("stock_prices?code=eq.%s", code), nil)
	if err != nil {
		log.Printf("Warning: failed to delete old prices for %s: %v", code, err)
	}

	// Insert new prices in batches
	batchSize := 100
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]

		_, err := s.supabaseRequest("POST", "stock_prices", batch)
		if err != nil {
			return fmt.Errorf("failed to save prices batch: %w", err)
		}
	}

	return nil
}

// LoadStockPrices loads price data from Supabase
func (s *SupabaseStorageService) LoadStockPrices(code string, limit int) ([]StockPriceData, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("supabase not configured")
	}

	endpoint := fmt.Sprintf("stock_prices?code=eq.%s&order=date.desc&limit=%d", code, limit)
	data, err := s.supabaseRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var rows []StockPriceRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}

	// Convert to StockPriceData
	prices := make([]StockPriceData, len(rows))
	for i, r := range rows {
		prices[i] = StockPriceData{
			Code:     r.Code,
			Date:     r.Date,
			Open:     r.Open,
			High:     r.High,
			Low:      r.Low,
			Close:    r.Close,
			NmVolume: float64(r.Volume),
			NmValue:  r.Value,
		}
	}

	return prices, nil
}

// GetStockCodesWithPrices returns list of stock codes that have price data
func (s *SupabaseStorageService) GetStockCodesWithPrices() ([]string, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("supabase not configured")
	}

	data, err := s.supabaseRequest("GET", "stock_prices?select=code&order=code", nil)
	if err != nil {
		return nil, err
	}

	var rows []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}

	// Unique codes
	codeMap := make(map[string]bool)
	for _, r := range rows {
		codeMap[r.Code] = true
	}

	codes := make([]string, 0, len(codeMap))
	for code := range codeMap {
		codes = append(codes, code)
	}

	return codes, nil
}

// ==================== Stock Indicators ====================

// StockIndicatorRow represents a row in stock_indicators table
type StockIndicatorRow struct {
	ID           int64   `json:"id,omitempty"`
	Code         string  `json:"code"`
	CurrentPrice float64 `json:"current_price"`
	PriceChange  float64 `json:"price_change"`
	RS3D         float64 `json:"rs_3d"`
	RS1M         float64 `json:"rs_1m"`
	RS3M         float64 `json:"rs_3m"`
	RS1Y         float64 `json:"rs_1y"`
	RS3DRank     float64 `json:"rs_3d_rank"`
	RS1MRank     float64 `json:"rs_1m_rank"`
	RS3MRank     float64 `json:"rs_3m_rank"`
	RS1YRank     float64 `json:"rs_1y_rank"`
	RSAvg        float64 `json:"rs_avg"`
	MACD         float64 `json:"macd"`
	MACDSignal   float64 `json:"macd_signal"`
	MACDHist     float64 `json:"macd_hist"`
	AvgVol       int64   `json:"avg_vol"`
	VolRatio     float64 `json:"vol_ratio"`
	RSI          float64 `json:"rsi"`
	MA10         float64 `json:"ma_10"`
	MA30         float64 `json:"ma_30"`
	MA50         float64 `json:"ma_50"`
	MA200        float64 `json:"ma_200"`
	UpdatedAt    string  `json:"updated_at,omitempty"`
}

// SaveIndicator saves a single stock indicator to Supabase
func (s *SupabaseStorageService) SaveIndicator(code string, ind *ExtendedStockIndicators) error {
	if !s.IsConfigured() {
		return fmt.Errorf("supabase not configured")
	}

	row := StockIndicatorRow{
		Code:         code,
		CurrentPrice: ind.CurrentPrice,
		PriceChange:  ind.PriceChange,
		RS3D:         ind.RS3D,
		RS1M:         ind.RS1M,
		RS3M:         ind.RS3M,
		RS1Y:         ind.RS1Y,
		RS3DRank:     ind.RS3DRank,
		RS1MRank:     ind.RS1MRank,
		RS3MRank:     ind.RS3MRank,
		RS1YRank:     ind.RS1YRank,
		RSAvg:        ind.RSAvg,
		MACD:         ind.MACD,
		MACDSignal:   ind.MACDSignal,
		MACDHist:     ind.MACDHist,
		AvgVol:       int64(ind.AvgVol),
		VolRatio:     ind.VolRatio,
		RSI:          ind.RSI,
		MA10:         ind.MA10,
		MA30:         ind.MA30,
		MA50:         ind.MA50,
		MA200:        ind.MA200,
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}

	// Upsert (insert or update)
	_, err := s.supabaseRequest("POST", "stock_indicators?on_conflict=code", row)
	return err
}

// SaveAllIndicators saves all indicators to Supabase
func (s *SupabaseStorageService) SaveAllIndicators(indicators map[string]*ExtendedStockIndicators) error {
	if !s.IsConfigured() {
		return fmt.Errorf("supabase not configured")
	}

	// Convert to rows
	rows := make([]StockIndicatorRow, 0, len(indicators))
	for code, ind := range indicators {
		if ind == nil {
			continue
		}
		rows = append(rows, StockIndicatorRow{
			Code:         code,
			CurrentPrice: ind.CurrentPrice,
			PriceChange:  ind.PriceChange,
			RS3D:         ind.RS3D,
			RS1M:         ind.RS1M,
			RS3M:         ind.RS3M,
			RS1Y:         ind.RS1Y,
			RS3DRank:     ind.RS3DRank,
			RS1MRank:     ind.RS1MRank,
			RS3MRank:     ind.RS3MRank,
			RS1YRank:     ind.RS1YRank,
			RSAvg:        ind.RSAvg,
			MACD:         ind.MACD,
			MACDSignal:   ind.MACDSignal,
			MACDHist:     ind.MACDHist,
			AvgVol:       int64(ind.AvgVol),
			VolRatio:     ind.VolRatio,
			RSI:          ind.RSI,
			MA10:         ind.MA10,
			MA30:         ind.MA30,
			MA50:         ind.MA50,
			MA200:        ind.MA200,
			UpdatedAt:    time.Now().Format(time.RFC3339),
		})
	}

	// Clear old indicators
	_, _ = s.supabaseRequest("DELETE", "stock_indicators?code=neq.''", nil)

	// Insert in batches
	batchSize := 100
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]

		_, err := s.supabaseRequest("POST", "stock_indicators", batch)
		if err != nil {
			return fmt.Errorf("failed to save indicators batch: %w", err)
		}
	}

	log.Printf("Saved %d indicators to Supabase", len(rows))
	return nil
}

// LoadAllIndicators loads all indicators from Supabase
func (s *SupabaseStorageService) LoadAllIndicators() (map[string]*ExtendedStockIndicators, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("supabase not configured")
	}

	data, err := s.supabaseRequest("GET", "stock_indicators?order=code", nil)
	if err != nil {
		return nil, err
	}

	var rows []StockIndicatorRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}

	indicators := make(map[string]*ExtendedStockIndicators)
	for _, r := range rows {
		indicators[r.Code] = &ExtendedStockIndicators{
			CurrentPrice: r.CurrentPrice,
			PriceChange:  r.PriceChange,
			RS3D:         r.RS3D,
			RS1M:         r.RS1M,
			RS3M:         r.RS3M,
			RS1Y:         r.RS1Y,
			RS3DRank:     r.RS3DRank,
			RS1MRank:     r.RS1MRank,
			RS3MRank:     r.RS3MRank,
			RS1YRank:     r.RS1YRank,
			RSAvg:        r.RSAvg,
			MACD:         r.MACD,
			MACDSignal:   r.MACDSignal,
			MACDHist:     r.MACDHist,
			AvgVol:       float64(r.AvgVol),
			VolRatio:     r.VolRatio,
			RSI:          r.RSI,
			MA10:         r.MA10,
			MA30:         r.MA30,
			MA50:         r.MA50,
			MA200:        r.MA200,
			UpdatedAt:    r.UpdatedAt,
		}
	}

	return indicators, nil
}

// GetTopRSStocks returns top stocks by RS average
func (s *SupabaseStorageService) GetTopRSStocks(limit int) ([]StockIndicatorRow, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("supabase not configured")
	}

	endpoint := fmt.Sprintf("stock_indicators?order=rs_avg.desc&limit=%d", limit)
	data, err := s.supabaseRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var rows []StockIndicatorRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}

	return rows, nil
}

// GetIndicatorsCount returns count of indicators in database
func (s *SupabaseStorageService) GetIndicatorsCount() (int, string, error) {
	if !s.IsConfigured() {
		return 0, "", fmt.Errorf("supabase not configured")
	}

	data, err := s.supabaseRequest("GET", "stock_indicators?select=code,updated_at&limit=1&order=updated_at.desc", nil)
	if err != nil {
		return 0, "", err
	}

	var rows []struct {
		Code      string `json:"code"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, "", err
	}

	// Get count
	countData, err := s.supabaseRequest("GET", "stock_indicators?select=count", nil)
	if err != nil {
		return 0, "", err
	}

	var countRows []struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(countData, &countRows); err != nil {
		return 0, "", err
	}

	count := 0
	if len(countRows) > 0 {
		count = countRows[0].Count
	}

	updatedAt := ""
	if len(rows) > 0 {
		updatedAt = rows[0].UpdatedAt
	}

	return count, updatedAt, nil
}

// ==================== System Config ====================

// SaveConfig saves a config value to Supabase
func (s *SupabaseStorageService) SaveConfig(key string, value interface{}) error {
	if !s.IsConfigured() {
		return fmt.Errorf("supabase not configured")
	}

	row := map[string]interface{}{
		"key":        key,
		"value":      value,
		"updated_at": time.Now().Format(time.RFC3339),
	}

	_, err := s.supabaseRequest("POST", "system_config?on_conflict=key", row)
	return err
}

// LoadConfig loads a config value from Supabase
func (s *SupabaseStorageService) LoadConfig(key string, dest interface{}) error {
	if !s.IsConfigured() {
		return fmt.Errorf("supabase not configured")
	}

	data, err := s.supabaseRequest("GET", fmt.Sprintf("system_config?key=eq.%s", key), nil)
	if err != nil {
		return err
	}

	var rows []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}

	if len(rows) == 0 {
		return fmt.Errorf("config not found: %s", key)
	}

	return json.Unmarshal(rows[0].Value, dest)
}
