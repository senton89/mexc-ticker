package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/senton89/mexc-ticker/internal/model"
)

const upsertSQL = `
INSERT INTO candles (symbol, ts, open, high, low, close, volume)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (symbol, ts) DO UPDATE SET
    open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
    close = EXCLUDED.close, volume = EXCLUDED.volume`

const querySQL = `
SELECT symbol, ts, open, high, low, close, volume
FROM candles
WHERE symbol = $1 AND ts >= $2 AND ts < $3
ORDER BY ts`

// Store пишет свечи в PostgreSQL батчами и читает их для API.
type Store struct {
	pool      *pgxpool.Pool
	batchSize int
	flushEach time.Duration
	log       *slog.Logger
}

// New открывает пул соединений и проверяет доступность БД.
func New(ctx context.Context, dsn string, batchSize int, flushEach time.Duration, log *slog.Logger) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{pool: pool, batchSize: batchSize, flushEach: flushEach, log: log}, nil
}

// Close закрывает пул. Вызывать после того, как Run завершился.
func (s *Store) Close() { s.pool.Close() }

// Run читает свечи из in до закрытия, копит буфер и флашит по размеру или таймеру.
// При закрытии in дописывает остаток на контексте без отмены.
func (s *Store) Run(ctx context.Context, in <-chan model.Candle) error {
	buf := make([]model.Candle, 0, s.batchSize)
	ticker := time.NewTicker(s.flushEach)
	defer ticker.Stop()

	flush := func(fctx context.Context) {
		if len(buf) == 0 {
			return
		}
		if err := s.Upsert(fctx, buf); err != nil {
			s.log.Error("flush failed", "candles", len(buf), "err", err)
			// TODO(README): буфер теряется — осознанно, чтобы не расти без границ;
			// retry с backoff — «что дальше».
		}
		buf = buf[:0]
	}

	for {
		select {
		case c, ok := <-in:
			if !ok {
				fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				flush(fctx)
				return nil
			}
			buf = append(buf, c)
			if len(buf) >= s.batchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

// Upsert записывает свечи одним батчем: INSERT … ON CONFLICT DO UPDATE.
func (s *Store) Upsert(ctx context.Context, candles []model.Candle) error {
	b := &pgx.Batch{}
	for _, c := range candles {
		b.Queue(upsertSQL, c.Symbol, c.Start, c.Open, c.High, c.Low, c.Close, c.Volume)
	}
	br := s.pool.SendBatch(ctx, b)
	defer func() { _ = br.Close() }()

	for range candles {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("store: upsert: %w", err)
		}
	}
	return nil
}

// QueryCandles возвращает свечи символа за [from, to) по возрастанию времени.
func (s *Store) QueryCandles(ctx context.Context, symbol string, from, to time.Time) ([]model.Candle, error) {
	rows, err := s.pool.Query(ctx, querySQL, symbol, from, to)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer rows.Close()

	var out []model.Candle
	for rows.Next() {
		var c model.Candle
		if err := rows.Scan(&c.Symbol, &c.Start, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, fmt.Errorf("store: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows: %w", err)
	}
	return out, nil
}
