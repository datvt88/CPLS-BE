# CPLS Admin UI Guide

## Tổng quan

CPLS Admin UI là giao diện quản trị web hiện đại để quản lý toàn bộ hệ thống giao dịch chứng khoán Việt Nam, bao gồm:

- Quản lý dữ liệu cổ phiếu
- Tạo và quản lý chiến lược giao dịch
- Chạy backtest với dữ liệu lịch sử
- Điều khiển bot giao dịch tự động
- Giám sát tín hiệu và giao dịch real-time

## Khởi động hệ thống

### 1. Cài đặt môi trường

```bash
# Cài đặt dependencies
go mod tidy

# Cấu hình database
cp .env.example .env
# Chỉnh sửa .env với thông tin database của bạn
```

### 2. Chạy ứng dụng

```bash
go run main.go
```

Server sẽ khởi động tại: `http://localhost:8080`

### 3. Truy cập Admin UI

Mở trình duyệt và truy cập:
```
http://localhost:8080/admin
```

## Các trang chức năng

### 1. Dashboard (Trang chủ)

**URL**: `/admin`

**Chức năng**:
- Hiển thị thống kê tổng quan:
  - Tổng số cổ phiếu
  - Số lượng chiến lược
  - Số lượng backtest đã chạy
  - Tổng số giao dịch
- Quick Actions:
  - Initialize Stock Data: Tải dữ liệu mẫu cho các cổ phiếu hàng đầu
  - Create Strategy: Tạo chiến lược mới
  - Run Backtest: Chạy backtest
- Trading Bot Status: Hiển thị và điều khiển trạng thái bot

**Hướng dẫn sử dụng**:
1. Click "Initialize Stock Data" để tải dữ liệu mẫu lần đầu
2. Kiểm tra bot status - start/stop bot theo nhu cầu
3. Sử dụng quick actions để nhanh chóng truy cập các chức năng chính

### 2. Stocks Management

**URL**: `/admin/stocks`

**Chức năng**:
- Xem danh sách tất cả cổ phiếu
- Lọc theo exchange (HOSE, HNX, UPCOM)
- Fetch historical data cho từng cổ phiếu
- Xem thông tin chi tiết cổ phiếu

**Hướng dẫn sử dụng**:

#### Fetch dữ liệu lịch sử:
1. Click nút "Fetch Historical Data" hoặc icon download bên cạnh cổ phiếu
2. Nhập:
   - Stock Symbol (VD: VNM, VIC, HPG)
   - Start Date (VD: 2024-01-01)
   - End Date (VD: 2024-11-12)
3. Click "Fetch Data"
4. Hệ thống sẽ tải dữ liệu từ API (trong production sẽ từ SSI/VNDirect)

#### Xem chi tiết cổ phiếu:
- Click icon "eye" để xem thông tin JSON chi tiết qua API

### 3. Trading Strategies

**URL**: `/admin/strategies`

**Chức năng**:
- Xem tất cả chiến lược đã tạo
- Tạo chiến lược mới với parameters tùy chỉnh
- Chạy backtest cho chiến lược
- Xóa chiến lược

**Các loại chiến lược được hỗ trợ**:

#### 1. SMA Crossover Strategy
Chiến lược giao dịch dựa trên việc đường SMA ngắn hạn cắt đường SMA dài hạn.

**Parameters**:
```json
{
  "short_period": 20,
  "long_period": 50
}
```

**Tín hiệu**:
- BUY: Khi SMA20 cắt lên trên SMA50 (Golden Cross)
- SELL: Khi SMA20 cắt xuống dưới SMA50 (Death Cross)

**Phù hợp**: Thị trường có xu hướng rõ ràng

#### 2. RSI Strategy
Chiến lược dựa trên chỉ báo RSI (Relative Strength Index).

**Parameters**:
```json
{
  "oversold": 30,
  "overbought": 70
}
```

**Tín hiệu**:
- BUY: Khi RSI < 30 (oversold - quá bán)
- SELL: Khi RSI > 70 (overbought - quá mua)

**Phù hợp**: Thị trường sideway, dao động

#### 3. MACD Strategy
Chiến lược dựa trên MACD histogram.

**Parameters**:
```json
{
  "fast": 12,
  "slow": 26,
  "signal": 9
}
```

**Tín hiệu**:
- BUY: Khi MACD cắt lên trên signal line (histogram dương)
- SELL: Khi MACD cắt xuống dưới signal line (histogram âm)

**Phù hợp**: Thị trường có xu hướng và momentum mạnh

#### 4. Breakout Strategy
Chiến lược dựa trên việc phá vỡ resistance/support.

**Parameters**:
```json
{
  "period": 20,
  "threshold": 0.02
}
```

**Tín hiệu**:
- BUY: Khi giá phá vỡ đỉnh 20 ngày
- SELL: Khi giá phá vỡ đáy 20 ngày

**Phù hợp**: Thị trường có biến động mạnh, breakout rõ ràng

**Hướng dẫn tạo chiến lược**:
1. Click "Create Strategy"
2. Nhập tên và mô tả
3. Chọn loại chiến lược
4. Parameters sẽ tự động điền, bạn có thể chỉnh sửa
5. Check "Active" nếu muốn bot sử dụng chiến lược này
6. Click "Create Strategy"

### 4. Backtesting

**URL**: `/admin/backtests`

**Chức năng**:
- Chạy backtest cho chiến lược với dữ liệu lịch sử
- Xem kết quả backtest với các metrics chi tiết
- So sánh hiệu quả giữa các chiến lược

**Hướng dẫn chạy backtest**:

1. **Chọn Strategy**: Chọn chiến lược đã tạo
2. **Chọn thời gian**:
   - Start Date: Ngày bắt đầu test (VD: 2024-01-01)
   - End Date: Ngày kết thúc test (VD: 2024-10-31)
3. **Initial Capital**: Vốn ban đầu (VD: 100,000,000 VND)
4. **Stocks**: Danh sách mã cổ phiếu cách nhau bởi dấu phẩy (VD: VNM,VIC,HPG,VCB)
5. Click "Run Backtest"

**Metrics được tính toán**:

- **Total Return**: Tổng lợi nhuận (%)
- **Annual Return**: Lợi nhuận hàng năm (%)
- **Win Rate**: Tỷ lệ giao dịch thắng (%)
- **Max Drawdown**: Mức sụt giảm tối đa (%)
- **Sharpe Ratio**: Tỷ lệ Sharpe (rủi ro/lợi nhuận)
- **Profit Factor**: Tỷ lệ lãi/lỗ
- **Average Win/Loss**: Lãi/lỗ trung bình mỗi giao dịch

**Giải thích kết quả**:

```
Total Return: 15.5%
→ Vốn tăng 15.5% trong khoảng thời gian test

Win Rate: 65%
→ 65% số giao dịch có lãi

Max Drawdown: -8.2%
→ Tối đa tài khoản giảm 8.2% so với đỉnh

Sharpe Ratio: 1.8
→ Tốt (>1.5 là tốt, >2 là rất tốt)
```

### 5. Trading Bot Control

**URL**: `/admin/trading-bot`

**Chức năng**:
- Khởi động/Dừng trading bot
- Xem tín hiệu real-time
- Theo dõi giao dịch được thực hiện
- Giám sát trạng thái bot

**Bot Settings**:
- Check Interval: 1 phút (kiểm tra thị trường mỗi phút)
- Trading Hours: 9:00 - 15:00 (giờ giao dịch chứng khoán VN)
- Risk Per Trade: 2% (mỗi lệnh không quá 2% vốn)
- Commission: 0.15% (phí giao dịch)

**Hướng dẫn sử dụng bot**:

1. **Trước khi start bot**:
   - Đảm bảo đã có dữ liệu lịch sử (fetch historical data)
   - Tạo ít nhất 1 strategy và set Active = true
   - Kiểm tra bot settings phù hợp với risk tolerance

2. **Start bot**:
   - Click "Start Bot"
   - Bot sẽ tự động:
     - Quét thị trường mỗi phút
     - Tính toán technical indicators
     - Generate signals từ active strategies
     - Thực hiện giao dịch nếu confidence > 70%

3. **Monitor bot**:
   - Recent Signals: Xem tín hiệu được tạo
   - Recent Trades: Xem giao dịch được thực hiện
   - Trang tự động refresh mỗi 30 giây khi bot đang chạy

4. **Stop bot**:
   - Click "Stop Bot" khi muốn dừng
   - Bot sẽ ngừng generate signals và giao dịch

**Giải thích Signals**:

```
Time: 10:15:23
Stock: VNM
Type: BUY
Price: 75,500
Confidence: 85%
Reason: SMA20 crossed above SMA50
```

- **Confidence**: Độ tin cậy của signal (0-100%)
- Bot chỉ thực hiện giao dịch khi confidence > 70%
- Reason: Lý do tại sao signal được tạo

## API Integration

Admin UI sử dụng các API endpoints:

### Stock APIs
```
GET  /api/v1/stocks
GET  /api/v1/stocks/:symbol
POST /api/v1/stocks/:symbol/fetch-historical
```

### Strategy APIs
```
GET    /api/v1/strategies
POST   /api/v1/strategies
DELETE /api/v1/strategies/:id
```

### Backtest APIs
```
POST /api/v1/backtests
GET  /api/v1/backtests
GET  /api/v1/backtests/:id
```

### Trading Bot APIs
```
POST /api/v1/trading/bot/start
POST /api/v1/trading/bot/stop
GET  /api/v1/trading/bot/status
GET  /api/v1/signals
GET  /api/v1/trading/trades
```

## Workflow khuyến nghị

### Workflow cơ bản:

1. **Khởi tạo dữ liệu** (Dashboard)
   - Click "Initialize Stock Data"
   - Đợi dữ liệu được tải về

2. **Tạo Strategy** (Strategies)
   - Tạo strategy với parameters phù hợp
   - Set Active = true

3. **Chạy Backtest** (Backtests)
   - Test strategy với dữ liệu lịch sử
   - Xem kết quả và điều chỉnh parameters nếu cần

4. **Start Bot** (Trading Bot)
   - Khi đã hài lòng với backtest results
   - Start bot để giao dịch tự động

5. **Monitor** (Trading Bot)
   - Theo dõi signals và trades
   - Điều chỉnh strategies nếu cần

### Workflow nâng cao:

1. **A/B Testing Strategies**:
   - Tạo nhiều variations của cùng một strategy
   - Chạy backtest cho tất cả
   - So sánh metrics để chọn strategy tốt nhất

2. **Multi-Strategy Trading**:
   - Tạo nhiều strategies khác nhau (SMA, RSI, MACD)
   - Set tất cả là Active
   - Bot sẽ combine signals từ tất cả strategies

3. **Risk Management**:
   - Giám sát Max Drawdown trong backtests
   - Điều chỉnh Risk Per Trade nếu cần
   - Stop bot nếu thị trường quá volatile

## Tips & Best Practices

### 1. Data Management
- Fetch historical data cho ít nhất 6 tháng trở lên
- Update data hàng ngày (scheduler tự động làm việc này)
- Backup database định kỳ

### 2. Strategy Development
- Bắt đầu với strategies đơn giản (SMA Crossover)
- Test kỹ với backtest trước khi dùng bot
- Điều chỉnh parameters dựa trên kết quả backtest

### 3. Backtesting
- Test với nhiều khoảng thời gian khác nhau
- Test cả bull market và bear market
- Win Rate > 55% là tốt, > 60% là rất tốt
- Max Drawdown < 15% là an toàn

### 4. Bot Trading
- Không start bot trong giờ giao dịch đầu tiên (9:00-9:30)
- Monitor bot chặt chẽ trong tuần đầu tiên
- Có stop loss strategy luôn sẵn sàng

### 5. Risk Management
- Không risk quá 2% vốn cho 1 giao dịch
- Diversify - Trade nhiều cổ phiếu khác nhau
- Có exit plan rõ ràng

## Troubleshooting

### Bot không start được:
- Kiểm tra có ít nhất 1 active strategy
- Kiểm tra database connection
- Xem logs trong console

### Backtest chạy lâu:
- Giảm số lượng stocks
- Rút ngắn thời gian test
- Đợi - một số strategies phức tạp cần nhiều thời gian

### Không có signals:
- Kiểm tra đã có historical data chưa
- Kiểm tra strategy parameters
- Thị trường có thể đang sideway - chưa có cơ hội

### Trades không được execute:
- Kiểm tra confidence threshold (phải > 70%)
- Kiểm tra đủ cash trong tài khoản
- Kiểm tra bot đang running

## Kết luận

Admin UI cung cấp một giao diện toàn diện để quản lý hệ thống giao dịch chứng khoán tự động. Với các chức năng từ quản lý dữ liệu đến điều khiển bot, bạn có thể:

- Nhanh chóng test và deploy trading strategies
- Giám sát performance real-time
- Điều chỉnh parameters dựa trên kết quả
- Tự động hóa toàn bộ quá trình giao dịch

Sử dụng Admin UI cùng với API endpoints để xây dựng một hệ thống giao dịch mạnh mẽ và hiệu quả!
