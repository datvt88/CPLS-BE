-- Stock Prices Table (stores historical price data)
CREATE TABLE IF NOT EXISTS stock_prices (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(20) NOT NULL,
    date DATE NOT NULL,
    open DECIMAL(15,2),
    high DECIMAL(15,2),
    low DECIMAL(15,2),
    close DECIMAL(15,2),
    volume BIGINT,
    value DECIMAL(20,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(code, date)
);

-- Index for fast queries
CREATE INDEX IF NOT EXISTS idx_stock_prices_code ON stock_prices(code);
CREATE INDEX IF NOT EXISTS idx_stock_prices_date ON stock_prices(date DESC);
CREATE INDEX IF NOT EXISTS idx_stock_prices_code_date ON stock_prices(code, date DESC);

-- Stock Indicators Table (stores calculated technical indicators)
CREATE TABLE IF NOT EXISTS stock_indicators (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(20) NOT NULL UNIQUE,

    -- Price info
    current_price DECIMAL(15,2),
    price_change DECIMAL(10,4),

    -- Relative Strength (raw % change)
    rs_3d DECIMAL(10,4),
    rs_1m DECIMAL(10,4),
    rs_3m DECIMAL(10,4),
    rs_1y DECIMAL(10,4),

    -- Relative Strength Ranks (1-100)
    rs_3d_rank DECIMAL(5,2),
    rs_1m_rank DECIMAL(5,2),
    rs_3m_rank DECIMAL(5,2),
    rs_1y_rank DECIMAL(5,2),
    rs_avg DECIMAL(5,2),

    -- MACD
    macd DECIMAL(15,4),
    macd_signal DECIMAL(15,4),
    macd_hist DECIMAL(15,4),

    -- Volume
    avg_vol BIGINT,
    vol_ratio DECIMAL(10,4),

    -- RSI
    rsi DECIMAL(5,2),

    -- Moving Averages
    ma_10 DECIMAL(15,2),
    ma_30 DECIMAL(15,2),
    ma_50 DECIMAL(15,2),
    ma_200 DECIMAL(15,2),

    -- Metadata
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for filtering/sorting
CREATE INDEX IF NOT EXISTS idx_stock_indicators_rs_avg ON stock_indicators(rs_avg DESC);
CREATE INDEX IF NOT EXISTS idx_stock_indicators_rsi ON stock_indicators(rsi);
CREATE INDEX IF NOT EXISTS idx_stock_indicators_macd_hist ON stock_indicators(macd_hist);

-- System Config Table (stores scheduler config, sync status, etc.)
CREATE TABLE IF NOT EXISTS system_config (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Comments
COMMENT ON TABLE stock_prices IS 'Historical stock price data from VNDirect';
COMMENT ON TABLE stock_indicators IS 'Calculated technical indicators for stock screening';
COMMENT ON TABLE system_config IS 'System configuration storage (scheduler, sync config, etc.)';
