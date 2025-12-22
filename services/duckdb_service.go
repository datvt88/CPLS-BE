package services

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database constants (using SQLite for now, compatible with DuckDB interface)
const (
	DuckDBPath = "data/market.db"
)

// DuckDBClient handles DuckDB operations
type DuckDBClient struct {
	db *sql.DB
	mu sync.RWMutex
}

// Global DuckDB client
var GlobalDuckDB *DuckDBClient

// InitDuckDB initializes the DuckDB connection
func InitDuckDB() error {
	// Create data directory if not exists
	dir := filepath.Dir(DuckDBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	db, err := sql.Open("sqlite3", DuckDBPath)
	if err != nil {
		return fmt.Errorf("failed to open DuckDB: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping DuckDB: %w", err)
	}

	GlobalDuckDB = &DuckDBClient{db: db}

	// Create tables
	if err := GlobalDuckDB.CreateTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Create additional tables for price history and indicators
	if err := GlobalDuckDB.CreatePriceHistoryTable(); err != nil {
		log.Printf("Warning: failed to create price history table: %v", err)
	}
	if err := GlobalDuckDB.CreateIndicatorsTable(); err != nil {
		log.Printf("Warning: failed to create indicators table: %v", err)
	}
	if err := GlobalDuckDB.CreateConfigTable(); err != nil {
		log.Printf("Warning: failed to create config table: %v", err)
	}

	log.Printf("DuckDB initialized at %s", DuckDBPath)
	return nil
}

// Close closes the DuckDB connection
func (c *DuckDBClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// CreateTables creates the required tables
func (c *DuckDBClient) CreateTables() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create stocks table
	stocksTable := `
		CREATE TABLE IF NOT EXISTS stocks (
			code VARCHAR PRIMARY KEY,
			type VARCHAR,
			floor VARCHAR,
			isin VARCHAR,
			status VARCHAR,
			company_name VARCHAR,
			company_name_eng VARCHAR,
			short_name VARCHAR,
			short_name_eng VARCHAR,
			listed_date VARCHAR,
			delisted_date VARCHAR,
			company_id VARCHAR,
			tax_code VARCHAR,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_sync_at TIMESTAMP
		)
	`
	if _, err := c.db.Exec(stocksTable); err != nil {
		return fmt.Errorf("failed to create stocks table: %w", err)
	}

	// Create stock_prices table
	pricesTable := `
		CREATE TABLE IF NOT EXISTS stock_prices (
			ticker VARCHAR PRIMARY KEY,
			exchange VARCHAR,
			price DOUBLE,
			price_change DOUBLE,
			price_change_ratio DOUBLE,
			vol DOUBLE,
			highest_price DOUBLE,
			lowest_price DOUBLE,
			open_price DOUBLE,
			close_price DOUBLE,
			ref_price DOUBLE,
			ceiling_price DOUBLE,
			floor_price DOUBLE,
			foreign_buy_vol DOUBLE,
			foreign_sell_vol DOUBLE,
			total_val DOUBLE,
			market_cap DOUBLE,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := c.db.Exec(pricesTable); err != nil {
		return fmt.Errorf("failed to create stock_prices table: %w", err)
	}

	// Create sync_history table (using INTEGER PRIMARY KEY for auto-increment in SQLite)
	syncTable := `
		CREATE TABLE IF NOT EXISTS sync_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sync_type VARCHAR,
			total_fetched INTEGER,
			created INTEGER,
			updated INTEGER,
			errors VARCHAR,
			synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := c.db.Exec(syncTable); err != nil {
		return fmt.Errorf("failed to create sync_history table: %w", err)
	}

	log.Println("DuckDB tables created/verified")
	return nil
}

// === Stock Operations ===

// DuckDBStock represents a stock in DuckDB
type DuckDBStock struct {
	Code           string     `json:"code"`
	Type           string     `json:"type"`
	Floor          string     `json:"floor"`
	ISIN           string     `json:"isin"`
	Status         string     `json:"status"`
	CompanyName    string     `json:"company_name"`
	CompanyNameEng string     `json:"company_name_eng"`
	ShortName      string     `json:"short_name"`
	ShortNameEng   string     `json:"short_name_eng"`
	ListedDate     string     `json:"listed_date"`
	DelistedDate   string     `json:"delisted_date"`
	CompanyID      string     `json:"company_id"`
	TaxCode        string     `json:"tax_code"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastSyncAt     *time.Time `json:"last_sync_at"`
}

// UpsertStock inserts or updates a stock
func (c *DuckDBClient) UpsertStock(stock *DuckDBStock) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `
		INSERT OR REPLACE INTO stocks (
			code, type, floor, isin, status, company_name, company_name_eng,
			short_name, short_name_eng, listed_date, delisted_date, company_id,
			tax_code, is_active, updated_at, last_sync_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	_, err := c.db.Exec(query,
		stock.Code, stock.Type, stock.Floor, stock.ISIN, stock.Status,
		stock.CompanyName, stock.CompanyNameEng, stock.ShortName, stock.ShortNameEng,
		stock.ListedDate, stock.DelistedDate, stock.CompanyID, stock.TaxCode,
		stock.IsActive, now, now,
	)

	return err
}

// GetAllStocks returns all stocks
func (c *DuckDBClient) GetAllStocks() ([]DuckDBStock, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT code, type, floor, isin, status, company_name, company_name_eng,
		short_name, short_name_eng, listed_date, delisted_date, company_id, tax_code,
		is_active, created_at, updated_at, last_sync_at FROM stocks ORDER BY code`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []DuckDBStock
	for rows.Next() {
		var s DuckDBStock
		var createdAt, updatedAt sql.NullTime
		var lastSyncAt sql.NullTime

		err := rows.Scan(
			&s.Code, &s.Type, &s.Floor, &s.ISIN, &s.Status,
			&s.CompanyName, &s.CompanyNameEng, &s.ShortName, &s.ShortNameEng,
			&s.ListedDate, &s.DelistedDate, &s.CompanyID, &s.TaxCode,
			&s.IsActive, &createdAt, &updatedAt, &lastSyncAt,
		)
		if err != nil {
			return nil, err
		}

		if createdAt.Valid {
			s.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			s.UpdatedAt = updatedAt.Time
		}
		if lastSyncAt.Valid {
			s.LastSyncAt = &lastSyncAt.Time
		}

		stocks = append(stocks, s)
	}

	return stocks, nil
}

// GetStockByCode returns a stock by code
func (c *DuckDBClient) GetStockByCode(code string) (*DuckDBStock, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT code, type, floor, isin, status, company_name, company_name_eng,
		short_name, short_name_eng, listed_date, delisted_date, company_id, tax_code,
		is_active, created_at, updated_at, last_sync_at FROM stocks WHERE code = ?`

	var s DuckDBStock
	var createdAt, updatedAt sql.NullTime
	var lastSyncAt sql.NullTime

	err := c.db.QueryRow(query, code).Scan(
		&s.Code, &s.Type, &s.Floor, &s.ISIN, &s.Status,
		&s.CompanyName, &s.CompanyNameEng, &s.ShortName, &s.ShortNameEng,
		&s.ListedDate, &s.DelistedDate, &s.CompanyID, &s.TaxCode,
		&s.IsActive, &createdAt, &updatedAt, &lastSyncAt,
	)
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		s.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		s.UpdatedAt = updatedAt.Time
	}
	if lastSyncAt.Valid {
		s.LastSyncAt = &lastSyncAt.Time
	}

	return &s, nil
}

// DeleteStock deletes a stock by code
func (c *DuckDBClient) DeleteStock(code string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.db.Exec("DELETE FROM stocks WHERE code = ?", code)
	return err
}

// GetStockCount returns the total count of stocks
func (c *DuckDBClient) GetStockCount() (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var count int64
	err := c.db.QueryRow("SELECT COUNT(*) FROM stocks").Scan(&count)
	return count, err
}

// GetStockCountByFloor returns count of stocks by floor
func (c *DuckDBClient) GetStockCountByFloor() (map[string]int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT floor, COUNT(*) as count FROM stocks GROUP BY floor`
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var floor string
		var count int64
		if err := rows.Scan(&floor, &count); err != nil {
			return nil, err
		}
		counts[floor] = count
	}

	return counts, err
}

// GetStocksPaginated returns paginated stocks with search and filter
func (c *DuckDBClient) GetStocksPaginated(page, pageSize int, search, floor, sortBy, sortOrder string) ([]DuckDBStock, int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	// Build WHERE clause
	where := "WHERE 1=1"
	args := []interface{}{}

	if floor != "" && floor != "all" {
		where += " AND floor = ?"
		args = append(args, floor)
	}

	if search != "" {
		where += " AND (LOWER(code) LIKE ? OR LOWER(company_name) LIKE ? OR LOWER(short_name) LIKE ?)"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM stocks " + where
	var total int64
	if err := c.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Build ORDER BY
	validSortBy := map[string]bool{"code": true, "floor": true, "company_name": true, "listed_date": true}
	if !validSortBy[sortBy] {
		sortBy = "code"
	}
	if sortOrder != "desc" {
		sortOrder = "asc"
	}

	// Get stocks
	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`SELECT code, type, floor, isin, status, company_name, company_name_eng,
		short_name, short_name_eng, listed_date, delisted_date, company_id, tax_code,
		is_active, created_at, updated_at, last_sync_at FROM stocks %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, sortOrder)

	args = append(args, pageSize, offset)
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var stocks []DuckDBStock
	for rows.Next() {
		var s DuckDBStock
		var createdAt, updatedAt sql.NullTime
		var lastSyncAt sql.NullTime

		err := rows.Scan(
			&s.Code, &s.Type, &s.Floor, &s.ISIN, &s.Status,
			&s.CompanyName, &s.CompanyNameEng, &s.ShortName, &s.ShortNameEng,
			&s.ListedDate, &s.DelistedDate, &s.CompanyID, &s.TaxCode,
			&s.IsActive, &createdAt, &updatedAt, &lastSyncAt,
		)
		if err != nil {
			return nil, 0, err
		}

		if createdAt.Valid {
			s.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			s.UpdatedAt = updatedAt.Time
		}
		if lastSyncAt.Valid {
			s.LastSyncAt = &lastSyncAt.Time
		}

		stocks = append(stocks, s)
	}

	return stocks, total, nil
}

// SaveSyncHistory saves sync history
func (c *DuckDBClient) SaveSyncHistory(syncType string, totalFetched, created, updated int, errors string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `INSERT INTO sync_history (sync_type, total_fetched, created, updated, errors, synced_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := c.db.Exec(query, syncType, totalFetched, created, updated, errors, time.Now())
	return err
}

// GetLastSyncTime returns the last sync time for a specific type
func (c *DuckDBClient) GetLastSyncTime(syncType string) (*time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var syncedAt sql.NullTime
	query := `SELECT synced_at FROM sync_history WHERE sync_type = ? ORDER BY synced_at DESC LIMIT 1`
	err := c.db.QueryRow(query, syncType).Scan(&syncedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if syncedAt.Valid {
		return &syncedAt.Time, nil
	}
	return nil, nil
}

// ==================== Stock Price History ====================

// CreatePriceHistoryTable creates the price history table
func (c *DuckDBClient) CreatePriceHistoryTable() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `
		CREATE TABLE IF NOT EXISTS stock_price_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code VARCHAR NOT NULL,
			date VARCHAR NOT NULL,
			open REAL,
			high REAL,
			low REAL,
			close REAL,
			volume REAL,
			value REAL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(code, date)
		)
	`
	if _, err := c.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create stock_price_history table: %w", err)
	}

	// Create indexes
	c.db.Exec("CREATE INDEX IF NOT EXISTS idx_price_history_code ON stock_price_history(code)")
	c.db.Exec("CREATE INDEX IF NOT EXISTS idx_price_history_date ON stock_price_history(date DESC)")

	log.Println("stock_price_history table created/verified")
	return nil
}

// SavePriceHistory saves price history for a stock
func (c *DuckDBClient) SavePriceHistory(code string, prices []StockPriceData) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Delete existing prices for this code
	_, err := c.db.Exec("DELETE FROM stock_price_history WHERE code = ?", code)
	if err != nil {
		return fmt.Errorf("failed to delete old prices: %w", err)
	}

	// Insert new prices
	stmt, err := c.db.Prepare(`
		INSERT INTO stock_price_history (code, date, open, high, low, close, volume, value)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, p := range prices {
		_, err := stmt.Exec(code, p.Date, p.Open, p.High, p.Low, p.Close, p.NmVolume, p.NmValue)
		if err != nil {
			log.Printf("Warning: failed to insert price for %s on %s: %v", code, p.Date, err)
		}
	}

	return nil
}

// LoadPriceHistory loads price history for a stock
func (c *DuckDBClient) LoadPriceHistory(code string, limit int) ([]StockPriceData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT code, date, open, high, low, close, volume, value
		FROM stock_price_history WHERE code = ? ORDER BY date DESC LIMIT ?`

	rows, err := c.db.Query(query, code, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prices []StockPriceData
	for rows.Next() {
		var p StockPriceData
		err := rows.Scan(&p.Code, &p.Date, &p.Open, &p.High, &p.Low, &p.Close, &p.NmVolume, &p.NmValue)
		if err != nil {
			return nil, err
		}
		prices = append(prices, p)
	}

	return prices, nil
}

// GetStockCodesWithPriceHistory returns list of stock codes that have price history
func (c *DuckDBClient) GetStockCodesWithPriceHistory() ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT DISTINCT code FROM stock_price_history ORDER BY code`
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}

	return codes, nil
}

// GetPriceHistoryCount returns count of stocks with price history
func (c *DuckDBClient) GetPriceHistoryCount() (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var count int
	err := c.db.QueryRow("SELECT COUNT(DISTINCT code) FROM stock_price_history").Scan(&count)
	return count, err
}

// ==================== Stock Indicators ====================

// DuckDBIndicator represents indicator data in database
type DuckDBIndicator struct {
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
	AvgVol       float64 `json:"avg_vol"`
	VolRatio     float64 `json:"vol_ratio"`
	RSI          float64 `json:"rsi"`
	MA10         float64 `json:"ma_10"`
	MA30         float64 `json:"ma_30"`
	MA50         float64 `json:"ma_50"`
	MA200        float64 `json:"ma_200"`
	UpdatedAt    string  `json:"updated_at"`
}

// CreateIndicatorsTable creates the indicators table
func (c *DuckDBClient) CreateIndicatorsTable() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `
		CREATE TABLE IF NOT EXISTS stock_indicators (
			code VARCHAR PRIMARY KEY,
			current_price REAL,
			price_change REAL,
			rs_3d REAL,
			rs_1m REAL,
			rs_3m REAL,
			rs_1y REAL,
			rs_3d_rank REAL,
			rs_1m_rank REAL,
			rs_3m_rank REAL,
			rs_1y_rank REAL,
			rs_avg REAL,
			macd REAL,
			macd_signal REAL,
			macd_hist REAL,
			avg_vol REAL,
			vol_ratio REAL,
			rsi REAL,
			ma_10 REAL,
			ma_30 REAL,
			ma_50 REAL,
			ma_200 REAL,
			updated_at VARCHAR
		)
	`
	if _, err := c.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create stock_indicators table: %w", err)
	}

	// Create indexes for common queries
	c.db.Exec("CREATE INDEX IF NOT EXISTS idx_indicators_rs_avg ON stock_indicators(rs_avg DESC)")
	c.db.Exec("CREATE INDEX IF NOT EXISTS idx_indicators_rsi ON stock_indicators(rsi)")

	log.Println("stock_indicators table created/verified")
	return nil
}

// SaveIndicator saves a single indicator
func (c *DuckDBClient) SaveIndicator(ind *DuckDBIndicator) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `
		INSERT OR REPLACE INTO stock_indicators (
			code, current_price, price_change,
			rs_3d, rs_1m, rs_3m, rs_1y,
			rs_3d_rank, rs_1m_rank, rs_3m_rank, rs_1y_rank, rs_avg,
			macd, macd_signal, macd_hist,
			avg_vol, vol_ratio, rsi,
			ma_10, ma_30, ma_50, ma_200, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := c.db.Exec(query,
		ind.Code, ind.CurrentPrice, ind.PriceChange,
		ind.RS3D, ind.RS1M, ind.RS3M, ind.RS1Y,
		ind.RS3DRank, ind.RS1MRank, ind.RS3MRank, ind.RS1YRank, ind.RSAvg,
		ind.MACD, ind.MACDSignal, ind.MACDHist,
		ind.AvgVol, ind.VolRatio, ind.RSI,
		ind.MA10, ind.MA30, ind.MA50, ind.MA200, ind.UpdatedAt,
	)
	return err
}

// SaveAllIndicators saves all indicators
func (c *DuckDBClient) SaveAllIndicators(indicators map[string]*ExtendedStockIndicators) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear old data
	c.db.Exec("DELETE FROM stock_indicators")

	stmt, err := c.db.Prepare(`
		INSERT INTO stock_indicators (
			code, current_price, price_change,
			rs_3d, rs_1m, rs_3m, rs_1y,
			rs_3d_rank, rs_1m_rank, rs_3m_rank, rs_1y_rank, rs_avg,
			macd, macd_signal, macd_hist,
			avg_vol, vol_ratio, rsi,
			ma_10, ma_30, ma_50, ma_200, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	for code, ind := range indicators {
		if ind == nil {
			continue
		}
		_, err := stmt.Exec(
			code, ind.CurrentPrice, ind.PriceChange,
			ind.RS3D, ind.RS1M, ind.RS3M, ind.RS1Y,
			ind.RS3DRank, ind.RS1MRank, ind.RS3MRank, ind.RS1YRank, ind.RSAvg,
			ind.MACD, ind.MACDSignal, ind.MACDHist,
			ind.AvgVol, ind.VolRatio, ind.RSI,
			ind.MA10, ind.MA30, ind.MA50, ind.MA200, ind.UpdatedAt,
		)
		if err != nil {
			log.Printf("Warning: failed to save indicator for %s: %v", code, err)
		} else {
			count++
		}
	}

	log.Printf("Saved %d indicators to local database", count)
	return nil
}

// LoadAllIndicators loads all indicators from database
func (c *DuckDBClient) LoadAllIndicators() (map[string]*ExtendedStockIndicators, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT code, current_price, price_change,
		rs_3d, rs_1m, rs_3m, rs_1y,
		rs_3d_rank, rs_1m_rank, rs_3m_rank, rs_1y_rank, rs_avg,
		macd, macd_signal, macd_hist,
		avg_vol, vol_ratio, rsi,
		ma_10, ma_30, ma_50, ma_200, updated_at
		FROM stock_indicators ORDER BY code`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indicators := make(map[string]*ExtendedStockIndicators)
	for rows.Next() {
		var code string
		var ind ExtendedStockIndicators
		err := rows.Scan(
			&code, &ind.CurrentPrice, &ind.PriceChange,
			&ind.RS3D, &ind.RS1M, &ind.RS3M, &ind.RS1Y,
			&ind.RS3DRank, &ind.RS1MRank, &ind.RS3MRank, &ind.RS1YRank, &ind.RSAvg,
			&ind.MACD, &ind.MACDSignal, &ind.MACDHist,
			&ind.AvgVol, &ind.VolRatio, &ind.RSI,
			&ind.MA10, &ind.MA30, &ind.MA50, &ind.MA200, &ind.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		indicators[code] = &ind
	}

	return indicators, nil
}

// GetTopRSIndicators returns top stocks by RS average
func (c *DuckDBClient) GetTopRSIndicators(limit int) ([]DuckDBIndicator, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	query := `SELECT code, current_price, price_change,
		rs_3d, rs_1m, rs_3m, rs_1y,
		rs_3d_rank, rs_1m_rank, rs_3m_rank, rs_1y_rank, rs_avg,
		macd, macd_signal, macd_hist,
		avg_vol, vol_ratio, rsi,
		ma_10, ma_30, ma_50, ma_200, updated_at
		FROM stock_indicators ORDER BY rs_avg DESC LIMIT ?`

	rows, err := c.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DuckDBIndicator
	for rows.Next() {
		var ind DuckDBIndicator
		err := rows.Scan(
			&ind.Code, &ind.CurrentPrice, &ind.PriceChange,
			&ind.RS3D, &ind.RS1M, &ind.RS3M, &ind.RS1Y,
			&ind.RS3DRank, &ind.RS1MRank, &ind.RS3MRank, &ind.RS1YRank, &ind.RSAvg,
			&ind.MACD, &ind.MACDSignal, &ind.MACDHist,
			&ind.AvgVol, &ind.VolRatio, &ind.RSI,
			&ind.MA10, &ind.MA30, &ind.MA50, &ind.MA200, &ind.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, ind)
	}

	return results, nil
}

// GetIndicatorsCount returns count and last update time
func (c *DuckDBClient) GetIndicatorsCount() (int, string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var count int
	var updatedAt sql.NullString

	err := c.db.QueryRow("SELECT COUNT(*) FROM stock_indicators").Scan(&count)
	if err != nil {
		return 0, "", err
	}

	c.db.QueryRow("SELECT updated_at FROM stock_indicators ORDER BY updated_at DESC LIMIT 1").Scan(&updatedAt)

	return count, updatedAt.String, nil
}

// ==================== System Config ====================

// CreateConfigTable creates the system config table
func (c *DuckDBClient) CreateConfigTable() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `
		CREATE TABLE IF NOT EXISTS system_config (
			key VARCHAR PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := c.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create system_config table: %w", err)
	}

	log.Println("system_config table created/verified")
	return nil
}

// SaveConfig saves a config value
func (c *DuckDBClient) SaveConfig(key string, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `INSERT OR REPLACE INTO system_config (key, value, updated_at) VALUES (?, ?, ?)`
	_, err := c.db.Exec(query, key, value, time.Now())
	return err
}

// LoadConfig loads a config value
func (c *DuckDBClient) LoadConfig(key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var value string
	err := c.db.QueryRow("SELECT value FROM system_config WHERE key = ?", key).Scan(&value)
	return value, err
}
