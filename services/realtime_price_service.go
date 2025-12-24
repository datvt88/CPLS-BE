package services

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Constants for service configuration
const (
	MaxWebSocketClients   = 100             // Maximum concurrent WebSocket clients
	WebSocketWriteTimeout = 10 * time.Second
	WebSocketPongTimeout  = 60 * time.Second
	WebSocketPingInterval = 30 * time.Second
	DefaultPollInterval   = 5 * time.Second
	PriceFetchBatchSize   = 20
	PriceFetchBatchDelay  = 100 * time.Millisecond
)

// RealtimePriceData represents realtime price data
type RealtimePriceData struct {
	Code          string  `json:"code"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Volume        float64 `json:"volume"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Open          float64 `json:"open"`
	RefPrice      float64 `json:"ref_price"`
	Timestamp     string  `json:"timestamp"`
}

// RealtimeIndicators represents calculated realtime indicators
type RealtimeIndicators struct {
	Code          string  `json:"code"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Volume        float64 `json:"volume"`
	AvgVol5D      float64 `json:"avg_vol_5d"`
	VolRatio      float64 `json:"vol_ratio"`
	RS1YRank      float64 `json:"rs_1y_rank"`
	RSAvg         float64 `json:"rs_avg"`
	MACDHist      float64 `json:"macd_hist"`
	RSI           float64 `json:"rsi"`
	InTopRS       bool    `json:"in_top_rs"`
	Timestamp     string  `json:"timestamp"`
}

// WebSocketMessage represents a message to broadcast
type WebSocketMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
	Time string      `json:"time"`
}

// Client represents a WebSocket client
type Client struct {
	conn       *websocket.Conn
	send       chan []byte
	subscribed map[string]bool
	mu         sync.RWMutex
}

// RealtimePriceService handles realtime price streaming
type RealtimePriceService struct {
	clients   map[*Client]bool
	broadcast chan WebSocketMessage
	register  chan *Client
	unregister chan *Client
	shutdown  chan struct{}
	mu        sync.RWMutex
	upgrader  websocket.Upgrader
	isRunning bool
	stopChan  chan struct{}

	// In-memory caches
	priceCache     map[string]*RealtimePriceData
	priceMu        sync.RWMutex
	indicatorCache *IndicatorSummaryFile
	indicatorMu    sync.RWMutex
	lastCacheTime  time.Time

	// Polling config
	pollingInterval time.Duration
	stockCodes      []string
}

// Global realtime service
var GlobalRealtimeService *RealtimePriceService

// InitRealtimePriceService initializes the realtime price service
func InitRealtimePriceService() error {
	GlobalRealtimeService = &RealtimePriceService{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan WebSocketMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		shutdown:   make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		priceCache:      make(map[string]*RealtimePriceData),
		pollingInterval: DefaultPollInterval,
		stopChan:        make(chan struct{}),
	}

	// Start the hub
	go GlobalRealtimeService.run()

	log.Println("Realtime Price Service initialized")
	return nil
}

// Shutdown gracefully shuts down the service
func (s *RealtimePriceService) Shutdown() {
	s.StopPolling()
	close(s.shutdown)

	// Close all client connections
	s.mu.Lock()
	for client := range s.clients {
		close(client.send)
		client.conn.Close()
	}
	s.clients = make(map[*Client]bool)
	s.mu.Unlock()

	log.Println("Realtime Price Service shutdown complete")
}

// run starts the WebSocket hub
func (s *RealtimePriceService) run() {
	for {
		select {
		case <-s.shutdown:
			return

		case client := <-s.register:
			s.mu.Lock()
			// Check client limit
			if len(s.clients) >= MaxWebSocketClients {
				s.mu.Unlock()
				client.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "Server at capacity"))
				client.conn.Close()
				log.Printf("WebSocket client rejected: max clients reached (%d)", MaxWebSocketClients)
				continue
			}
			s.clients[client] = true
			clientCount := len(s.clients)
			s.mu.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", clientCount)

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
			clientCount := len(s.clients)
			s.mu.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", clientCount)

		case message := <-s.broadcast:
			data, err := json.Marshal(message)
			if err != nil {
				log.Printf("Error marshaling broadcast message: %v", err)
				continue
			}

			s.mu.Lock()
			deadClients := make([]*Client, 0)
			for client := range s.clients {
				select {
				case client.send <- data:
				default:
					// Client buffer full, mark for removal
					deadClients = append(deadClients, client)
				}
			}
			// Remove dead clients
			for _, client := range deadClients {
				delete(s.clients, client)
				close(client.send)
			}
			s.mu.Unlock()
		}
	}
}

// HandleWebSocket handles WebSocket connections
func (s *RealtimePriceService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check if at capacity before upgrading
	s.mu.RLock()
	atCapacity := len(s.clients) >= MaxWebSocketClients
	s.mu.RUnlock()

	if atCapacity {
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:       conn,
		send:       make(chan []byte, 256),
		subscribed: make(map[string]bool),
	}

	s.register <- client

	go client.writePump()
	go client.readPump(s)
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(WebSocketPingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(WebSocketWriteTimeout))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(WebSocketWriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump(s *RealtimePriceService) {
	defer func() {
		s.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(WebSocketPongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(WebSocketPongTimeout))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		var cmd struct {
			Action string   `json:"action"`
			Codes  []string `json:"codes"`
		}
		if err := json.Unmarshal(message, &cmd); err != nil {
			continue
		}

		switch cmd.Action {
		case "subscribe":
			c.mu.Lock()
			for _, code := range cmd.Codes {
				c.subscribed[code] = true
			}
			c.mu.Unlock()
		case "unsubscribe":
			c.mu.Lock()
			for _, code := range cmd.Codes {
				delete(c.subscribed, code)
			}
			c.mu.Unlock()
		case "get_top_rs":
			s.sendTopRSToClient(c)
		}
	}
}

// StartPolling starts polling prices from VNDirect
func (s *RealtimePriceService) StartPolling(codes []string) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("polling already running")
	}
	s.isRunning = true
	s.stockCodes = codes
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.pollPrices()

	codeCount := len(codes)
	if codeCount == 0 {
		codeCount = len(s.loadTopRSCodes())
	}
	log.Printf("Started price polling for %d stocks (interval: %v)", codeCount, s.pollingInterval)
	return nil
}

// StopPolling stops the polling
func (s *RealtimePriceService) StopPolling() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	close(s.stopChan)
	s.isRunning = false
	log.Println("Price polling stopped")
}

// pollPrices polls prices and broadcasts updates
func (s *RealtimePriceService) pollPrices() {
	ticker := time.NewTicker(s.pollingInterval)
	defer ticker.Stop()

	// Initial poll
	s.fetchAndBroadcast()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.fetchAndBroadcast()
		}
	}
}

// getIndicatorCache returns cached indicators, refreshing if needed
func (s *RealtimePriceService) getIndicatorCache() *IndicatorSummaryFile {
	s.indicatorMu.RLock()
	// Cache for 30 seconds
	if s.indicatorCache != nil && time.Since(s.lastCacheTime) < 30*time.Second {
		cache := s.indicatorCache
		s.indicatorMu.RUnlock()
		return cache
	}
	s.indicatorMu.RUnlock()

	// Refresh cache
	s.indicatorMu.Lock()
	defer s.indicatorMu.Unlock()

	// Double-check after acquiring write lock
	if s.indicatorCache != nil && time.Since(s.lastCacheTime) < 30*time.Second {
		return s.indicatorCache
	}

	if GlobalIndicatorService != nil {
		summary, err := GlobalIndicatorService.LoadIndicatorSummary()
		if err == nil {
			s.indicatorCache = summary
			s.lastCacheTime = time.Now()
			return summary
		}
	}
	return nil
}

// fetchAndBroadcast fetches prices and broadcasts them
func (s *RealtimePriceService) fetchAndBroadcast() {
	s.mu.RLock()
	codes := s.stockCodes
	s.mu.RUnlock()

	if len(codes) == 0 {
		codes = s.loadTopRSCodes()
	}

	if len(codes) == 0 {
		return
	}

	// Pre-load indicator cache once
	indicatorSummary := s.getIndicatorCache()

	allPrices := make([]RealtimePriceData, 0, len(codes))
	allIndicators := make([]RealtimeIndicators, 0, len(codes))

	for i := 0; i < len(codes); i += PriceFetchBatchSize {
		end := i + PriceFetchBatchSize
		if end > len(codes) {
			end = len(codes)
		}
		batch := codes[i:end]

		for _, code := range batch {
			price, err := s.fetchCurrentPrice(code)
			if err != nil {
				continue
			}

			s.priceMu.Lock()
			s.priceCache[code] = price
			s.priceMu.Unlock()

			allPrices = append(allPrices, *price)

			indicator := s.calculateIndicatorsWithCache(code, price, indicatorSummary)
			if indicator != nil {
				allIndicators = append(allIndicators, *indicator)
			}
		}

		// Small delay between batches to avoid rate limiting
		if end < len(codes) {
			time.Sleep(PriceFetchBatchDelay)
		}
	}

	// Broadcast price updates
	if len(allPrices) > 0 {
		s.broadcast <- WebSocketMessage{
			Type: "prices",
			Data: allPrices,
			Time: time.Now().Format(time.RFC3339),
		}
	}

	// Broadcast indicators
	if len(allIndicators) > 0 {
		topRS := filterTopRS(allIndicators)

		s.broadcast <- WebSocketMessage{
			Type: "indicators",
			Data: allIndicators,
			Time: time.Now().Format(time.RFC3339),
		}

		if len(topRS) > 0 {
			s.broadcast <- WebSocketMessage{
				Type: "top_rs",
				Data: topRS,
				Time: time.Now().Format(time.RFC3339),
			}
		}
	}
}

// fetchCurrentPrice fetches current price for a stock
func (s *RealtimePriceService) fetchCurrentPrice(code string) (*RealtimePriceData, error) {
	if GlobalPriceService == nil {
		return nil, fmt.Errorf("price service not initialized")
	}

	priceResp, err := GlobalPriceService.FetchStockPrice(code, 1)
	if err != nil {
		return nil, err
	}

	if len(priceResp.Data) == 0 {
		return nil, fmt.Errorf("no price data for %s", code)
	}

	data := priceResp.Data[0]
	return &RealtimePriceData{
		Code:          data.Code,
		Price:         data.Close,
		Change:        data.Change,
		ChangePercent: data.PctChange,
		Volume:        data.NmVolume,
		High:          data.High,
		Low:           data.Low,
		Open:          data.Open,
		RefPrice:      data.BasicPrice,
		Timestamp:     time.Now().Format(time.RFC3339),
	}, nil
}

// calculateIndicatorsWithCache calculates indicators using cached data
func (s *RealtimePriceService) calculateIndicatorsWithCache(code string, currentPrice *RealtimePriceData, cache *IndicatorSummaryFile) *RealtimeIndicators {
	var avgVol5D, rs1YRank, rsAvg, macdHist, rsi float64

	if cache != nil && cache.Stocks[code] != nil {
		ind := cache.Stocks[code]
		avgVol5D = ind.AvgVol
		rs1YRank = ind.RS1YRank
		rsAvg = ind.RSAvg
		macdHist = ind.MACDHist
		rsi = ind.RSI
	}

	volRatio := 0.0
	if avgVol5D > 0 && currentPrice.Volume > 0 {
		volRatio = math.Round((currentPrice.Volume/avgVol5D)*100) / 100
	}

	inTopRS := avgVol5D >= 600000 && rs1YRank >= 80 && rsAvg >= 40 && macdHist > -0.1

	return &RealtimeIndicators{
		Code:          code,
		Price:         currentPrice.Price,
		Change:        currentPrice.Change,
		ChangePercent: currentPrice.ChangePercent,
		Volume:        currentPrice.Volume,
		AvgVol5D:      avgVol5D,
		VolRatio:      volRatio,
		RS1YRank:      rs1YRank,
		RSAvg:         rsAvg,
		MACDHist:      macdHist,
		RSI:           rsi,
		InTopRS:       inTopRS,
		Timestamp:     time.Now().Format(time.RFC3339),
	}
}

// calculateRealtimeIndicators calculates indicators (backwards compatibility)
func (s *RealtimePriceService) calculateRealtimeIndicators(code string, currentPrice *RealtimePriceData) *RealtimeIndicators {
	return s.calculateIndicatorsWithCache(code, currentPrice, s.getIndicatorCache())
}

// filterTopRS filters indicators that meet top RS criteria
func filterTopRS(indicators []RealtimeIndicators) []RealtimeIndicators {
	result := make([]RealtimeIndicators, 0, len(indicators))
	for _, ind := range indicators {
		if ind.InTopRS {
			result = append(result, ind)
		}
	}

	// Use efficient sort.Slice instead of bubble sort
	sort.Slice(result, func(i, j int) bool {
		return result[i].RSAvg > result[j].RSAvg
	})

	return result
}

// loadTopRSCodes loads stock codes that meet top RS criteria
func (s *RealtimePriceService) loadTopRSCodes() []string {
	summary := s.getIndicatorCache()
	if summary == nil {
		return nil
	}

	codes := make([]string, 0, 100)
	for code, ind := range summary.Stocks {
		if ind == nil {
			continue
		}
		if ind.AvgVol >= 600000 && ind.RS1YRank >= 80 && ind.RSAvg >= 40 && ind.MACDHist > -0.1 {
			codes = append(codes, code)
		}
	}

	return codes
}

// sendTopRSToClient sends current top RS stocks to a specific client
func (s *RealtimePriceService) sendTopRSToClient(c *Client) {
	summary := s.getIndicatorCache()
	if summary == nil {
		return
	}

	codes := s.loadTopRSCodes()
	indicators := make([]RealtimeIndicators, 0, len(codes))

	for _, code := range codes {
		s.priceMu.RLock()
		price := s.priceCache[code]
		s.priceMu.RUnlock()

		if price == nil {
			if ind := summary.Stocks[code]; ind != nil {
				indicators = append(indicators, RealtimeIndicators{
					Code:      code,
					Price:     ind.CurrentPrice,
					AvgVol5D:  ind.AvgVol,
					RS1YRank:  ind.RS1YRank,
					RSAvg:     ind.RSAvg,
					MACDHist:  ind.MACDHist,
					RSI:       ind.RSI,
					InTopRS:   true,
					Timestamp: ind.UpdatedAt,
				})
			}
		} else {
			indicator := s.calculateIndicatorsWithCache(code, price, summary)
			if indicator != nil {
				indicators = append(indicators, *indicator)
			}
		}
	}

	// Use efficient sort
	sort.Slice(indicators, func(i, j int) bool {
		return indicators[i].RSAvg > indicators[j].RSAvg
	})

	msg := WebSocketMessage{
		Type: "top_rs",
		Data: indicators,
		Time: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case c.send <- data:
	default:
		// Client buffer full, skip
	}
}

// GetClientCount returns the number of connected clients
func (s *RealtimePriceService) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// IsPolling returns whether polling is active
func (s *RealtimePriceService) IsPolling() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// SetPollingInterval sets the polling interval
func (s *RealtimePriceService) SetPollingInterval(seconds int) {
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 300 {
		seconds = 300
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pollingInterval = time.Duration(seconds) * time.Second
}

// BroadcastMessage broadcasts a custom message to all clients
func (s *RealtimePriceService) BroadcastMessage(msgType string, data interface{}) {
	s.broadcast <- WebSocketMessage{
		Type: msgType,
		Data: data,
		Time: time.Now().Format(time.RFC3339),
	}
}

// GetStatus returns service status info
func (s *RealtimePriceService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"is_polling":       s.isRunning,
		"client_count":     len(s.clients),
		"max_clients":      MaxWebSocketClients,
		"poll_interval_sec": int(s.pollingInterval.Seconds()),
		"stock_codes":      len(s.stockCodes),
	}
}
