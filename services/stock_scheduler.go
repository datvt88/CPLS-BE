package services

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StockSchedulerConfig holds the scheduler configuration
type StockSchedulerConfig struct {
	Enabled      bool   `json:"enabled"`
	ScheduleTime string `json:"schedule_time"` // Format: "HH:MM" (e.g., "08:00")
	LastRun      string `json:"last_run"`
	NextRun      string `json:"next_run"`
}

// StockScheduler manages automatic stock syncing
type StockScheduler struct {
	config     StockSchedulerConfig
	configFile string
	stopChan   chan struct{}
	mu         sync.RWMutex
	running    bool
}

// Global stock scheduler instance
var GlobalStockScheduler *StockScheduler

const StockSchedulerConfigFile = "data/stock_scheduler.json"

// InitStockScheduler initializes the stock scheduler
func InitStockScheduler() error {
	GlobalStockScheduler = &StockScheduler{
		configFile: StockSchedulerConfigFile,
		stopChan:   make(chan struct{}),
	}

	// Load config from file
	if err := GlobalStockScheduler.LoadConfig(); err != nil {
		log.Printf("No scheduler config found, using defaults: %v", err)
		// Set default config
		GlobalStockScheduler.config = StockSchedulerConfig{
			Enabled:      false,
			ScheduleTime: "08:00",
		}
	}

	// Start the scheduler if enabled
	if GlobalStockScheduler.config.Enabled {
		GlobalStockScheduler.Start()
	}

	return nil
}

// LoadConfig loads scheduler config from file
func (s *StockScheduler) LoadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.configFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.config)
}

// SaveConfig saves scheduler config to file
func (s *StockScheduler) SaveConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.configFile, data, 0644)
}

// GetConfig returns the current scheduler configuration
func (s *StockScheduler) GetConfig() StockSchedulerConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// UpdateConfig updates the scheduler configuration
func (s *StockScheduler) UpdateConfig(enabled bool, scheduleTime string) error {
	s.mu.Lock()
	s.config.Enabled = enabled
	s.config.ScheduleTime = scheduleTime
	s.mu.Unlock()

	// Save to file
	if err := s.SaveConfig(); err != nil {
		return err
	}

	// Restart scheduler if needed
	if enabled {
		s.Stop()
		s.Start()
	} else {
		s.Stop()
	}

	return nil
}

// Start starts the scheduler
func (s *StockScheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.run()
	log.Printf("Stock scheduler started, schedule time: %s", s.config.ScheduleTime)
}

// Stop stops the scheduler
func (s *StockScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopChan)
	s.running = false
	log.Println("Stock scheduler stopped")
}

// IsRunning returns whether the scheduler is running
func (s *StockScheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// run is the main scheduler loop
func (s *StockScheduler) run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Calculate next run time
	s.updateNextRunTime()

	for {
		select {
		case <-s.stopChan:
			return
		case now := <-ticker.C:
			s.mu.RLock()
			scheduleTime := s.config.ScheduleTime
			enabled := s.config.Enabled
			s.mu.RUnlock()

			if !enabled {
				continue
			}

			// Parse schedule time
			hour, min := parseScheduleTime(scheduleTime)
			currentHour := now.Hour()
			currentMin := now.Minute()

			// Check if it's time to sync
			if currentHour == hour && currentMin == min {
				log.Println("Stock auto-sync triggered by scheduler")
				s.runSync()
			}
		}
	}
}

// runSync performs the actual sync
func (s *StockScheduler) runSync() {
	result, err := SyncStocksFromVNDirectToDuckDB()
	if err != nil {
		log.Printf("Scheduled stock sync failed: %v", err)
	} else {
		log.Printf("Scheduled stock sync completed: fetched=%d, created=%d, updated=%d",
			result.TotalFetched, result.Created, result.Updated)
	}

	// Update last run time
	s.mu.Lock()
	s.config.LastRun = time.Now().Format(time.RFC3339)
	s.mu.Unlock()

	s.updateNextRunTime()
	s.SaveConfig()
}

// updateNextRunTime calculates and updates the next run time
func (s *StockScheduler) updateNextRunTime() {
	s.mu.Lock()
	defer s.mu.Unlock()

	hour, min := parseScheduleTime(s.config.ScheduleTime)
	now := time.Now()

	nextRun := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())

	// If the time has passed today, schedule for tomorrow
	if now.After(nextRun) {
		nextRun = nextRun.Add(24 * time.Hour)
	}

	s.config.NextRun = nextRun.Format(time.RFC3339)
}

// parseScheduleTime parses "HH:MM" format into hour and minute
func parseScheduleTime(timeStr string) (int, int) {
	hour := 8
	min := 0

	if len(timeStr) >= 5 {
		var h, m int
		if _, err := parseTimeFormat(timeStr, &h, &m); err == nil {
			hour = h
			min = m
		}
	}

	return hour, min
}

// parseTimeFormat parses time in HH:MM format
func parseTimeFormat(timeStr string, hour, min *int) (bool, error) {
	if len(timeStr) < 5 {
		return false, nil
	}

	// Parse hour
	*hour = int(timeStr[0]-'0')*10 + int(timeStr[1]-'0')
	// Parse minute
	*min = int(timeStr[3]-'0')*10 + int(timeStr[4]-'0')

	return true, nil
}

// RunSyncNow triggers an immediate sync (for manual trigger)
func (s *StockScheduler) RunSyncNow() (*StockSyncResult, error) {
	result, err := SyncStocksFromVNDirectToDuckDB()
	if err != nil {
		return nil, err
	}

	// Update last run time
	s.mu.Lock()
	s.config.LastRun = time.Now().Format(time.RFC3339)
	s.mu.Unlock()

	s.SaveConfig()
	return result, nil
}
