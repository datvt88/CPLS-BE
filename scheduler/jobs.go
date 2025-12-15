package scheduler

import (
	"log"
	"time"

	"go_backend_project/models"
	"go_backend_project/services/analysis"
	"go_backend_project/services/datafetcher"
	"github.com/go-co-op/gocron"
	"gorm.io/gorm"
)

// Scheduler manages scheduled jobs
type Scheduler struct {
	cron              *gocron.Scheduler
	db                *gorm.DB
	dataFetcher       *datafetcher.DataFetcher
	technicalAnalysis *analysis.TechnicalAnalysis
}

// NewScheduler creates a new scheduler instance
func NewScheduler(db *gorm.DB) *Scheduler {
	return &Scheduler{
		cron:              gocron.NewScheduler(time.UTC),
		db:                db,
		dataFetcher:       datafetcher.NewDataFetcher(db),
		technicalAnalysis: analysis.NewTechnicalAnalysis(db),
	}
}

// Start starts all scheduled jobs
func (s *Scheduler) Start() {
	log.Println("Starting scheduler...")

	// Fetch market data every 5 minutes during trading hours
	s.cron.Every(5).Minutes().Do(func() {
		if isMarketOpen() {
			s.fetchRealtimeData()
		}
	})

	// Fetch historical data daily at 16:00 (after market close)
	s.cron.Every(1).Day().At("16:00").Do(func() {
		s.fetchDailyHistoricalData()
	})

	// Calculate technical indicators daily at 16:30
	s.cron.Every(1).Day().At("16:30").Do(func() {
		s.calculateDailyIndicators()
	})

	// Update market indices every minute during trading hours
	s.cron.Every(1).Minute().Do(func() {
		if isMarketOpen() {
			s.dataFetcher.FetchMarketIndices()
		}
	})

	// Check and trigger user alerts every 5 minutes
	s.cron.Every(5).Minutes().Do(func() {
		if isMarketOpen() {
			s.checkUserAlerts()
		}
	})

	// Cleanup old data weekly on Sunday at 01:00
	s.cron.Every(1).Week().Sunday().At("01:00").Do(func() {
		s.cleanupOldData()
	})

	s.cron.StartAsync()
	log.Println("Scheduler started successfully")
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Scheduler stopped")
}

// fetchRealtimeData fetches real-time data for all active stocks
func (s *Scheduler) fetchRealtimeData() {
	log.Println("Fetching real-time data...")

	var stocks []models.Stock
	if err := s.db.Where("status = ?", "active").Find(&stocks).Error; err != nil {
		log.Printf("Error loading stocks: %v", err)
		return
	}

	for _, stock := range stocks {
		// In production, this would call actual real-time APIs
		_, err := s.dataFetcher.FetchRealtimeQuote(stock.Symbol)
		if err != nil {
			log.Printf("Error fetching quote for %s: %v", stock.Symbol, err)
		}
	}

	log.Printf("Fetched real-time data for %d stocks", len(stocks))
}

// fetchDailyHistoricalData fetches historical data for the previous day
func (s *Scheduler) fetchDailyHistoricalData() {
	log.Println("Fetching daily historical data...")

	today := time.Now()
	yesterday := today.AddDate(0, 0, -1)

	var stocks []models.Stock
	if err := s.db.Where("status = ?", "active").Find(&stocks).Error; err != nil {
		log.Printf("Error loading stocks: %v", err)
		return
	}

	for _, stock := range stocks {
		err := s.dataFetcher.FetchHistoricalData(stock.Symbol, yesterday, today)
		if err != nil {
			log.Printf("Error fetching historical data for %s: %v", stock.Symbol, err)
		}
	}

	log.Printf("Fetched historical data for %d stocks", len(stocks))
}

// calculateDailyIndicators calculates technical indicators for all stocks
func (s *Scheduler) calculateDailyIndicators() {
	log.Println("Calculating daily technical indicators...")

	var stocks []models.Stock
	if err := s.db.Where("status = ?", "active").Find(&stocks).Error; err != nil {
		log.Printf("Error loading stocks: %v", err)
		return
	}

	today := time.Now()
	for _, stock := range stocks {
		if err := s.technicalAnalysis.CalculateAllIndicators(stock.ID, today); err != nil {
			log.Printf("Error calculating indicators for %s: %v", stock.Symbol, err)
		}
	}

	log.Printf("Calculated indicators for %d stocks", len(stocks))
}

// checkUserAlerts checks and triggers user price alerts
func (s *Scheduler) checkUserAlerts() {
	log.Println("Checking user alerts...")

	var alerts []models.UserAlert
	if err := s.db.Where("is_active = ? AND is_triggered = ?", true, false).
		Preload("Stock").Find(&alerts).Error; err != nil {
		log.Printf("Error loading alerts: %v", err)
		return
	}

	for _, alert := range alerts {
		// Get latest price
		var latestPrice models.StockPrice
		if err := s.db.Where("stock_id = ?", alert.StockID).Order("date DESC").First(&latestPrice).Error; err != nil {
			continue
		}

		shouldTrigger := false
		switch alert.AlertType {
		case "price_above":
			shouldTrigger = latestPrice.Close.GreaterThanOrEqual(alert.TargetValue)
		case "price_below":
			shouldTrigger = latestPrice.Close.LessThanOrEqual(alert.TargetValue)
		case "percent_change":
			shouldTrigger = latestPrice.ChangePercent.Abs().GreaterThanOrEqual(alert.TargetValue)
		}

		if shouldTrigger {
			now := time.Now()
			s.db.Model(&alert).Updates(map[string]interface{}{
				"is_triggered": true,
				"triggered_at": now,
			})

			// Here you would send email/push notification
			log.Printf("Alert triggered for user %d, stock %s", alert.UserID, alert.Stock.Symbol)
		}
	}
}

// cleanupOldData removes old data to save storage
func (s *Scheduler) cleanupOldData() {
	log.Println("Cleaning up old data...")

	// Delete price data older than 5 years
	fiveYearsAgo := time.Now().AddDate(-5, 0, 0)
	if err := s.db.Where("date < ?", fiveYearsAgo).Delete(&models.StockPrice{}).Error; err != nil {
		log.Printf("Error cleaning up old prices: %v", err)
	}

	// Delete old signals (keep last 3 months)
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	if err := s.db.Where("created_at < ?", threeMonthsAgo).Delete(&models.Signal{}).Error; err != nil {
		log.Printf("Error cleaning up old signals: %v", err)
	}

	// Delete triggered alerts older than 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	if err := s.db.Where("is_triggered = ? AND triggered_at < ?", true, thirtyDaysAgo).
		Delete(&models.UserAlert{}).Error; err != nil {
		log.Printf("Error cleaning up old alerts: %v", err)
	}

	log.Println("Cleanup completed")
}

// isMarketOpen checks if Vietnamese stock market is currently open
func isMarketOpen() bool {
	now := time.Now()

	// Check if weekend
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}

	// Vietnamese stock market hours: 9:00 - 15:00 (local time)
	hour := now.Hour()
	return hour >= 9 && hour < 15
}
