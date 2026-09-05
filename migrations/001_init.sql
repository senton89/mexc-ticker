-- 001_init.sql
-- Минутные свечи OHLCV. Одна строка = один символ за одну минуту.
-- ts — начало минуты в UTC (timestamptz, truncated to minute).

CREATE TABLE IF NOT EXISTS candles (
    symbol  TEXT           NOT NULL,
    ts      TIMESTAMPTZ    NOT NULL,
    open    NUMERIC(20, 8) NOT NULL,
    high    NUMERIC(20, 8) NOT NULL,
    low     NUMERIC(20, 8) NOT NULL,
    close   NUMERIC(20, 8) NOT NULL,
    volume  NUMERIC(24, 8) NOT NULL DEFAULT 0,

    CONSTRAINT candles_pkey PRIMARY KEY (symbol, ts),
    CONSTRAINT candles_ohlc_check CHECK (low <= open AND low <= close
                                     AND high >= open AND high >= close
                                     AND volume >= 0)
);

-- PK (symbol, ts) уже покрывает запрос
-- WHERE symbol = $1 AND ts BETWEEN $2 AND $3 ORDER BY ts — отдельный индекс не нужен.