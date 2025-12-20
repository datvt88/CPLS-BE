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
