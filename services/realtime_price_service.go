package services

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RealtimePriceData represents realtime price data
type RealtimePriceData struct {
	Code         string  `json:"code"`
	Price        float64 `json:"price"`
	Change       float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Volume       float64 `json:"volume"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	Open         float64 `json:"open"`
	RefPrice     float64 `json:"ref_price"`
	Timestamp    string  `json:"timestamp"`
}

// RealtimeIndicators represents calculated realtime indicators
type RealtimeIndicators struct {
	Code         string  `json:"code"`
	Price        float64 `json:"price"`
	Change       float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Volume       float64 `json:"volume"`
	AvgVol5D     float64 `json:"avg_vol_5d"`
	VolRatio     float64 `json:"vol_ratio"`
	RS1YRank     float64 `json:"rs_1y_rank"`
	RSAvg        float64 `json:"rs_avg"`
	MACDHist     float64 `json:"macd_hist"`
	RSI          float64 `json:"rsi"`
	InTopRS      bool    `json:"in_top_rs"` // Meets top RS criteria
	Timestamp    string  `json:"timestamp"`
}

// WebSocketMessage represents a message to broadcast
type WebSocketMessage struct {
	Type    string      `json:"type"` // "price", "indicators", "top_rs", "error"
	Data    interface{} `json:"data"`
	Time    string      `json:"time"`
}

// Client represents a WebSocket client
type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	subscribed map[string]bool // Subscribed stock codes
	mu        sync.RWMutex
}

// RealtimePriceService handles realtime price streaming
type RealtimePriceService struct {
	clients       map[*Client]bool
	broadcast     chan WebSocketMessage
	register      chan *Client
	unregister    chan *Client
	mu            sync.RWMutex
	upgrader      websocket.Upgrader
	isRunning     bool
	stopChan      chan struct{}

	// In-memory price cache for quick calculations
	priceCache    map[string]*RealtimePriceData
	priceMu       sync.RWMutex

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
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
		priceCache:      make(map[string]*RealtimePriceData),
		pollingInterval: 5 * time.Second, // Poll every 5 seconds
		stopChan:        make(chan struct{}),
	}

	// Start the hub
	go GlobalRealtimeService.run()

	log.Println("Realtime Price Service initialized")
	return nil
}

// run starts the WebSocket hub
func (s *RealtimePriceService) run() {
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = true
			s.mu.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", len(s.clients))

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
			s.mu.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", len(s.clients))

		case message := <-s.broadcast:
			data, err := json.Marshal(message)
			if err != nil {
				continue
			}

			s.mu.RLock()
			for client := range s.clients {
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(s.clients, client)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// HandleWebSocket handles WebSocket connections
func (s *RealtimePriceService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
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

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump(s)
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
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
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle client commands (subscribe/unsubscribe)
		var cmd struct {
			Action string   `json:"action"` // "subscribe", "unsubscribe", "get_top_rs"
			Codes  []string `json:"codes"`
		}
		if err := json.Unmarshal(message, &cmd); err == nil {
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
				// Send current top RS stocks
				s.sendTopRSToClient(c)
			}
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
	log.Printf("Started price polling for %d stocks", len(codes))
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

// fetchAndBroadcast fetches prices and broadcasts them
func (s *RealtimePriceService) fetchAndBroadcast() {
	s.mu.RLock()
	codes := s.stockCodes
	s.mu.RUnlock()

	if len(codes) == 0 {
		// Load top RS stocks if no codes specified
		codes = s.loadTopRSCodes()
	}

	// Fetch prices in batches
	batchSize := 20
	allPrices := make([]RealtimePriceData, 0)
	allIndicators := make([]RealtimeIndicators, 0)

	for i := 0; i < len(codes); i += batchSize {
		end := i + batchSize
		if end > len(codes) {
			end = len(codes)
		}
		batch := codes[i:end]

		for _, code := range batch {
			price, err := s.fetchCurrentPrice(code)
			if err != nil {
				continue
			}

			// Update cache
			s.priceMu.Lock()
			s.priceCache[code] = price
			s.priceMu.Unlock()

			allPrices = append(allPrices, *price)

			// Calculate realtime indicators
			indicator := s.calculateRealtimeIndicators(code, price)
			if indicator != nil {
				allIndicators = append(allIndicators, *indicator)
			}
		}

		// Small delay between batches
		time.Sleep(100 * time.Millisecond)
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
		// Filter for top RS criteria
		topRS := filterTopRS(allIndicators)

		s.broadcast <- WebSocketMessage{
			Type: "indicators",
			Data: allIndicators,
			Time: time.Now().Format(time.RFC3339),
		}

		s.broadcast <- WebSocketMessage{
			Type: "top_rs",
			Data: topRS,
			Time: time.Now().Format(time.RFC3339),
		}
	}
}

// fetchCurrentPrice fetches current price for a stock
func (s *RealtimePriceService) fetchCurrentPrice(code string) (*RealtimePriceData, error) {
	if GlobalPriceService == nil {
		return nil, fmt.Errorf("price service not initialized")
	}

	// Fetch latest price from VNDirect
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

// calculateRealtimeIndicators calculates indicators using historical data from DuckDB
func (s *RealtimePriceService) calculateRealtimeIndicators(code string, currentPrice *RealtimePriceData) *RealtimeIndicators {
	// Load historical indicators
	var avgVol5D, rs1YRank, rsAvg, macdHist, rsi float64

	// Try to get from indicator summary first (faster)
	if GlobalIndicatorService != nil {
		summary, err := GlobalIndicatorService.LoadIndicatorSummary()
		if err == nil && summary.Stocks[code] != nil {
			ind := summary.Stocks[code]
			avgVol5D = ind.AvgVol
			rs1YRank = ind.RS1YRank
			rsAvg = ind.RSAvg
			macdHist = ind.MACDHist
			rsi = ind.RSI
		}
	}

	// Calculate volume ratio with current volume
	volRatio := 0.0
	if avgVol5D > 0 && currentPrice.Volume > 0 {
		volRatio = math.Round((currentPrice.Volume/avgVol5D)*100) / 100
	}

	// Check if meets top RS criteria
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

// filterTopRS filters indicators that meet top RS criteria
func filterTopRS(indicators []RealtimeIndicators) []RealtimeIndicators {
	result := make([]RealtimeIndicators, 0)
	for _, ind := range indicators {
		if ind.InTopRS {
			result = append(result, ind)
		}
	}

	// Sort by RSAvg descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].RSAvg > result[i].RSAvg {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// loadTopRSCodes loads stock codes that meet top RS criteria
func (s *RealtimePriceService) loadTopRSCodes() []string {
	codes := make([]string, 0)

	if GlobalIndicatorService == nil {
		return codes
	}

	summary, err := GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		return codes
	}

	for code, ind := range summary.Stocks {
		if ind == nil {
			continue
		}
		// Apply top RS filter criteria
		if ind.AvgVol >= 600000 && ind.RS1YRank >= 80 && ind.RSAvg >= 40 && ind.MACDHist > -0.1 {
			codes = append(codes, code)
		}
	}

	return codes
}

// sendTopRSToClient sends current top RS stocks to a specific client
func (s *RealtimePriceService) sendTopRSToClient(c *Client) {
	codes := s.loadTopRSCodes()
	indicators := make([]RealtimeIndicators, 0)

	for _, code := range codes {
		s.priceMu.RLock()
		price := s.priceCache[code]
		s.priceMu.RUnlock()

		if price == nil {
			// Use cached indicator data
			if GlobalIndicatorService != nil {
				summary, err := GlobalIndicatorService.LoadIndicatorSummary()
				if err == nil && summary.Stocks[code] != nil {
					ind := summary.Stocks[code]
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
			}
		} else {
			indicator := s.calculateRealtimeIndicators(code, price)
			if indicator != nil {
				indicators = append(indicators, *indicator)
			}
		}
	}

	// Sort by RSAvg
	for i := 0; i < len(indicators)-1; i++ {
		for j := i + 1; j < len(indicators); j++ {
			if indicators[j].RSAvg > indicators[i].RSAvg {
				indicators[i], indicators[j] = indicators[j], indicators[i]
			}
		}
	}

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
