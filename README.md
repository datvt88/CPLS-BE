# CPLS Backend - Vietnamese Stock Trading System

Hệ thống backend hoàn chỉnh cho giao dịch chứng khoán Việt Nam với khả năng backtesting và bot giao dịch tự động. Tích hợp Supabase cho quản lý user và hỗ trợ triển khai trên Vercel.

## ✨ Tính năng chính

### 👤 Quản lý User (Supabase Integration)
- Xác thực người dùng qua Supabase Auth
- Quản lý profile, preferences
- Watchlist cổ phiếu
- Price alerts tùy chỉnh
- Session management

### 📊 Dữ liệu chứng khoán
- Lấy dữ liệu từ HOSE, HNX, UPCOM
- Dữ liệu lịch sử và real-time
- Chỉ số thị trường (VN-Index, HNX-Index, UPCOM-Index)
- Top gainers/losers, Most active stocks

### 🔍 Stock Screener (Bộ lọc cổ phiếu)
- Lọc theo sàn, ngành, sector
- Lọc theo giá, khối lượng, market cap
- Lọc theo chỉ báo kỹ thuật (RSI, SMA, MACD)
- Preset screeners: Oversold, Overbought, Bullish, Golden Cross
- Volume spike detection
- New high/low filtering

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

### 💳 Subscription Plans
- Quản lý gói dịch vụ (Free, Basic, Premium, Pro)
- Theo dõi thanh toán
- Feature limits theo plan

## 🏗 Kiến trúc

```
CPLS-BE/
├── api/              # Vercel serverless handler
├── config/           # Database & environment config
├── models/           # Database models
│   ├── stock.go      # Stock, StockPrice, TechnicalIndicator
│   ├── user.go       # User, Watchlist, UserAlert
│   ├── trading.go    # Strategy, Trade, Portfolio, Signal
│   ├── subscription.go # Plans, Subscriptions, Payments
│   └── supabase_config.go # Supabase client config
├── services/         # Business logic
│   ├── datafetcher/  # Stock data fetching
│   ├── analysis/     # Technical analysis
│   ├── backtesting/  # Backtesting engine
│   ├── screener/     # Stock filtering/screening
│   └── trading/      # Trading bot
├── controllers/      # API controllers
├── routes/           # API routes
├── scheduler/        # Scheduled jobs
├── admin/            # Admin UI
├── vercel.json       # Vercel deployment config
├── Dockerfile        # Docker config
└── main.go           # Entry point
```

## 🚀 Quick Start

### Prerequisites
- Go 1.23+
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

### User Management
- `GET /api/v1/users` - List users
- `GET /api/v1/users/:id` - Get user by ID
- `POST /api/v1/users` - Create user
- `PUT /api/v1/users/:id` - Update user
- `DELETE /api/v1/users/:id` - Deactivate user
- `POST /api/v1/users/sync` - Sync from Supabase Auth
- `GET /api/v1/users/:id/watchlist` - Get watchlist
- `POST /api/v1/users/:id/watchlist` - Add to watchlist
- `DELETE /api/v1/users/:id/watchlist/:stock_id` - Remove from watchlist
- `GET /api/v1/users/:id/alerts` - Get price alerts
- `POST /api/v1/users/:id/alerts` - Create alert
- `DELETE /api/v1/users/:id/alerts/:alert_id` - Delete alert

### Subscription Management
- `GET /api/v1/subscriptions/plans` - List plans
- `GET /api/v1/subscriptions/plans/:id` - Get plan details
- `POST /api/v1/subscriptions/plans` - Create plan (admin)
- `GET /api/v1/subscriptions/user/:user_id` - Get user subscription
- `POST /api/v1/subscriptions/subscribe` - Subscribe to plan
- `POST /api/v1/subscriptions/cancel` - Cancel subscription
- `GET /api/v1/subscriptions/payments/:user_id` - Payment history

### Stock Screener
- `POST /api/v1/screener/screen` - Custom screening
- `GET /api/v1/screener/presets` - List preset screeners
- `GET /api/v1/screener/presets/:id` - Run preset screener
- `GET /api/v1/screener/top-gainers` - Top gainers
- `GET /api/v1/screener/top-losers` - Top losers
- `GET /api/v1/screener/most-active` - Most active
- `GET /api/v1/screener/oversold` - Oversold stocks (RSI < 30)
- `GET /api/v1/screener/overbought` - Overbought stocks (RSI > 70)
- `GET /api/v1/screener/bullish` - Bullish trend stocks
- `GET /api/v1/screener/volume-spike` - Volume spike stocks

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

### Stock Screening

```bash
# Custom screening - find oversold stocks above SMA50
curl -X POST http://localhost:8080/api/v1/screener/screen \
  -H "Content-Type: application/json" \
  -d '{
    "max_rsi": 30,
    "above_sma50": true,
    "min_volume": 1000000,
    "exchange": ["HOSE"],
    "sort_by": "volume",
    "sort_order": "desc",
    "page": 1,
    "limit": 20
  }'

# Get preset screeners
curl http://localhost:8080/api/v1/screener/presets
```

### User Management with Supabase

```bash
# Sync user from Supabase
curl -X POST http://localhost:8080/api/v1/users/sync \
  -H "Content-Type: application/json" \
  -d '{
    "supabase_user_id": "auth0|123456",
    "email": "user@example.com",
    "full_name": "John Doe"
  }'

# Add stock to watchlist
curl -X POST http://localhost:8080/api/v1/users/1/watchlist \
  -H "Content-Type: application/json" \
  -d '{
    "stock_id": 1,
    "notes": "Watching for breakout",
    "alert_price": 50000,
    "alert_type": "above"
  }'

# Create price alert
curl -X POST http://localhost:8080/api/v1/users/1/alerts \
  -H "Content-Type: application/json" \
  -d '{
    "stock_id": 1,
    "alert_type": "price_above",
    "target_value": 55000,
    "notify_email": true
  }'
```

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

## 🚢 Deployment

### Vercel Deployment

```bash
# Install Vercel CLI
npm i -g vercel

# Deploy
vercel

# For production
vercel --prod
```

### Docker Deployment

```bash
# Build
docker build -t cpls-backend .

# Run
docker run -p 8080:8080 --env-file .env cpls-backend
```

### Google Cloud Run

```bash
gcloud builds submit --config cloudbuild.yaml .
```

## ⚙️ Environment Variables

```env
# Server
PORT=8080
ENVIRONMENT=production

# Database (Supabase PostgreSQL)
DB_HOST=db.xxxx.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-password
DB_NAME=postgres

# Supabase
SUPABASE_URL=https://xxxx.supabase.co
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_KEY=your-service-key
SUPABASE_JWT_SECRET=your-jwt-secret

# JWT
JWT_SECRET=your-jwt-secret

# Trading
DEFAULT_COMMISSION_RATE=0.0015
DEFAULT_TAX_RATE=0.001
```

## 🔧 Performance Optimizations

- Database indexing on (stock_id, date)
- Pagination for large datasets
- Efficient query patterns with GORM
- Batch processing for multiple stocks
- Scheduled jobs for data updates
- Serverless deployment support

## 📄 License

MIT License

---

**Note**: This is a demo/educational system. For production use, you need real exchange API connections, proper authentication, and regulatory compliance.