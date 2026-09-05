package agg

import (
	"context"
	"log/slog"
	"time"

	"github.com/senton89/mexc-ticker/internal/model"
)

// Aggregator собирает поток сделок в минутные свечи OHLCV.
// Один воркер: все символы в одной map, без мьютексов — читатель у канала один.
type Aggregator struct {
	grace time.Duration
	now   func() time.Time // подменяется в тестах
	log   *slog.Logger
}

// New создаёт агрегатор с grace-периодом закрытия «молчащих» свечей.
func New(grace time.Duration, log *slog.Logger) *Aggregator {
	return &Aggregator{grace: grace, now: time.Now, log: log}
}

// Run читает in до закрытия, отдаёт закрытые свечи в out и закрывает out при выходе.
// Завершается по закрытию in (не по ctx) — чтобы дописать открытые свечи при shutdown.
func (a *Aggregator) Run(ctx context.Context, in <-chan model.Trade, out chan<- model.Candle) error {
	defer close(out)

	open := make(map[string]*model.Candle)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case t, ok := <-in:
			if !ok {
				for _, c := range open {
					out <- *c
				}
				return nil
			}
			if c, closed := apply(open, t, a.log); closed {
				out <- c
			}
		case <-ticker.C:
			deadline := a.now().Add(-time.Minute - a.grace)
			for sym, c := range open {
				if c.Start.Before(deadline) {
					out <- *c
					delete(open, sym)
				}
			}
		}
	}
}

// apply добавляет сделку в открытую свечу символа.
// Если сделка открывает новую минуту — возвращает предыдущую свечу и closed=true.
// Чистая функция без I/O — именно её покрывает тест в Ф4.
func apply(open map[string]*model.Candle, t model.Trade, log *slog.Logger) (model.Candle, bool) {
	start := t.Time.Truncate(time.Minute)
	c, exists := open[t.Symbol]

	switch {
	case !exists:
		open[t.Symbol] = newCandle(t, start)
		return model.Candle{}, false
	case start.After(c.Start):
		prev := *c
		open[t.Symbol] = newCandle(t, start)
		return prev, true
	case start.Before(c.Start):
		log.Warn("late trade dropped", "symbol", t.Symbol, "trade", t.Time, "candle", c.Start)
		return model.Candle{}, false
	default:
		c.High = max(c.High, t.Price)
		c.Low = min(c.Low, t.Price)
		c.Close = t.Price
		c.Volume += t.Qty
		c.Trades++
		return model.Candle{}, false
	}
}

// newCandle создаёт свечу из первой сделки минуты: O=H=L=C=price.
func newCandle(t model.Trade, start time.Time) *model.Candle {
	return &model.Candle{
		Symbol: t.Symbol, Start: start,
		Open: t.Price, High: t.Price, Low: t.Price, Close: t.Price,
		Volume: t.Qty, Trades: 1}
}
