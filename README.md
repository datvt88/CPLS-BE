# CPLS Backend - Vietnamese Stock Trading System

Hệ thống backend hoàn chỉnh cho giao dịch chứng khoán Việt Nam với khả năng backtesting và bot giao dịch tự động.

## ✨ Tính năng chính

### 📊 Dữ liệu chứng khoán
- Lấy dữ liệu từ HOSE, HNX, UPCOM
- Dữ liệu lịch sử và real-time
- Chỉ số thị trường (VN-Index, HNX-Index, UPCOM-Index)
- Top gainers/losers, Most active stocks

### 📈 Phân tích kỹ thuật
- Moving Averages (SMA, EMA)
- RSI (Relative Strength Index)
- MACD (Moving Average Convergence Divergence)
- Bollinger Bands
- Stochastic Oscillator

### 🤖 Bot giao dịch tự động
- Hỗ trợ nhiều chiến lược: SMA Crossover, RSI, MACD, Breakout
- Tự động tạo tín hiệu mua/bán
- Quản lý rủi ro tích hợp

### 📉 Backtesting Engine
- Kiểm tra hiệu quả chiến lược với dữ liệu lịch sử
- Tính toán Total Return, Sharpe Ratio, Win Rate, Max Drawdown
- Chi tiết từng giao dịch

### 💼 Portfolio Management
- Quản lý danh mục đầu tư
- Tính toán P&L tự động

## 🏗 Kiến trúc

```
CPLS-BE/
├── config/           # Database & environment config
├── models/           # Database models
├── services/         # Business logic
│   ├── datafetcher/  # Stock data fetching
│   ├── analysis/     # Technical analysis
│   ├── backtesting/  # Backtesting engine
│   └── trading/      # Trading bot
├── controllers/      # API controllers
├── routes/           # API routes
├── scheduler/        # Scheduled jobs
└── main.go          # Entry point
```

## 🚀 Quick Start

### Prerequisites
- Go 1.20+
- PostgreSQL 14+ (or Supabase)

### Installation

```bash
# Install dependencies
go mod tidy

# Setup environment
cp .env.example .env
# Edit .env with your database credentials

# Run application
go run main.go
```

Server runs on `http://localhost:8080`

## 📚 API Endpoints

### Stock Data
- `GET /api/v1/stocks` - List all stocks
- `GET /api/v1/stocks/search?q=VNM` - Search stocks
- `GET /api/v1/stocks/:symbol` - Get stock details
- `GET /api/v1/stocks/:symbol/prices` - Historical prices
- `GET /api/v1/stocks/:symbol/quote` - Real-time quote
- `GET /api/v1/stocks/:symbol/indicators` - Technical indicators
- `POST /api/v1/stocks/:symbol/fetch-historical` - Fetch historical data

### Market Data
- `GET /api/v1/market/indices` - Market indices
- `GET /api/v1/market/top-gainers` - Top gaining stocks
- `GET /api/v1/market/top-losers` - Top losing stocks
- `GET /api/v1/market/most-active` - Most active stocks

### Trading Strategies
- `GET /api/v1/strategies` - List strategies
- `POST /api/v1/strategies` - Create strategy
- `PUT /api/v1/strategies/:id` - Update strategy
- `DELETE /api/v1/strategies/:id` - Delete strategy

### Backtesting
- `POST /api/v1/backtests` - Run backtest
- `GET /api/v1/backtests` - List backtests
- `GET /api/v1/backtests/:id` - Backtest details

### Trading Bot
- `POST /api/v1/trading/bot/start` - Start bot
- `POST /api/v1/trading/bot/stop` - Stop bot
- `GET /api/v1/trading/bot/status` - Bot status
- `POST /api/v1/trading/manual` - Manual trade
- `GET /api/v1/trading/trades` - Trade history
- `GET /api/v1/trading/portfolio` - Portfolio

### Signals
- `GET /api/v1/signals` - Trading signals

## 🎯 Usage Examples

### Create a Trading Strategy

```bash
curl -X POST http://localhost:8080/api/v1/strategies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "SMA 20/50 Crossover",
    "type": "sma_crossover",
    "parameters": "{\"short_period\": 20, \"long_period\": 50}",
    "is_active": true
  }'
```

### Run Backtest

```bash
curl -X POST http://localhost:8080/api/v1/backtests \
  -H "Content-Type: application/json" \
  -d '{
    "strategy_id": 1,
    "start_date": "2024-01-01",
    "end_date": "2024-10-31",
    "initial_capital": 100000000,
    "symbols": ["VNM", "VIC", "HPG"],
    "commission": 0.0015,
    "risk_per_trade": 0.02
  }'
```

### Start Trading Bot

```bash
curl -X POST http://localhost:8080/api/v1/trading/bot/start
```

## 🔧 Performance Optimizations

- Database indexing on (stock_id, date)
- Pagination for large datasets
- Efficient query patterns with GORM
- Batch processing for multiple stocks
- Scheduled jobs for data updates

## 📝 TODO

- [ ] Redis caching layer
- [ ] Authentication & authorization
- [ ] Real Vietnamese exchange API integration
- [ ] More technical indicators
- [ ] WebSocket for real-time updates
- [ ] Unit tests & integration tests
- [ ] Docker Compose setup

## 🚢 Deployment

```bash
# Cloud Run deployment
gcloud builds submit --config cloudbuild.yaml .
```

## 📄 License

MIT License

---

**Note**: This is a demo/educational system. For production use, you need real exchange API connections, proper authentication, and regulatory compliance.