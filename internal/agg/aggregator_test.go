package agg

import (
	"log/slog"
	"testing"
	"time"

	"github.com/senton89/mexc-ticker/internal/model"
)

// base — начало минуты 12:00 UTC; все сделки задаются смещением от неё.
var base = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func trade(sym string, offset time.Duration, price, qty float64) model.Trade {
	return model.Trade{Symbol: sym, Time: base.Add(offset), Price: price, Qty: qty}
}

func TestApply(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	tests := []struct {
		name       string
		trades     []model.Trade
		wantClosed []model.Candle          // что apply вернула с closed=true, по порядку
		wantOpen   map[string]model.Candle // что осталось открытым по символам
	}{
		{
			name: "same minute aggregates",
			trades: []model.Trade{
				trade("BTCUSDT", 0, 100, 1),
				trade("BTCUSDT", 20*time.Second, 120, 2),
				trade("BTCUSDT", 40*time.Second, 90, 3),
			},
			wantClosed: nil,
			wantOpen: map[string]model.Candle{
				"BTCUSDT": {Symbol: "BTCUSDT", Start: base, Open: 100, High: 120, Low: 90, Close: 90, Volume: 6, Trades: 3},
			},
		},
		{
			name: "next minute closes previous",
			trades: []model.Trade{
				trade("BTCUSDT", 0, 100, 1),
				trade("BTCUSDT", 30*time.Second, 110, 2),
				trade("BTCUSDT", 60*time.Second, 105, 3),
			},
			wantClosed: []model.Candle{
				{Symbol: "BTCUSDT", Start: base, Open: 100, High: 110, Low: 100, Close: 110, Volume: 3, Trades: 2},
				// ЗАПОЛНИ: свеча минуты 12:00 после первых двух сделок
			},
			wantOpen: map[string]model.Candle{
				"BTCUSDT": {Symbol: "BTCUSDT", Start: base.Add(time.Minute), Open: 105, High: 105, Low: 105, Close: 105, Volume: 3, Trades: 1},
			},
		},
		{
			name: "late trade is dropped",
			trades: []model.Trade{
				trade("BTCUSDT", 60*time.Second, 105, 1),
				trade("BTCUSDT", 30*time.Second, 200, 5), // минута 12:00 уже позади — дроп
			},
			wantClosed: nil,
			wantOpen: map[string]model.Candle{
				"BTCUSDT": {Symbol: "BTCUSDT", Start: base.Add(time.Minute), Open: 105, High: 105, Low: 105, Close: 105, Volume: 1, Trades: 1},
			},
		},
		{
			name: "symbols are independent",
			trades: []model.Trade{
				trade("BTCUSDT", 0, 100, 1),
				trade("ETHUSDT", 0, 10, 2),
				trade("BTCUSDT", 60*time.Second, 101, 1),
			},
			wantClosed: []model.Candle{
				{Symbol: "BTCUSDT", Start: base, Open: 100, High: 100, Low: 100, Close: 100, Volume: 1, Trades: 1},
			},
			wantOpen: map[string]model.Candle{
				"BTCUSDT": {Symbol: "BTCUSDT", Start: base.Add(time.Minute), Open: 101, High: 101, Low: 101, Close: 101, Volume: 1, Trades: 1},
				"ETHUSDT": {Symbol: "ETHUSDT", Start: base, Open: 10, High: 10, Low: 10, Close: 10, Volume: 2, Trades: 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			open := make(map[string]*model.Candle)
			var closed []model.Candle
			for _, tr := range tc.trades {
				if c, ok := apply(open, tr, log); ok {
					closed = append(closed, c)
				}
			}

			if len(closed) != len(tc.wantClosed) {
				t.Fatalf("closed: got %d candles, want %d: %+v", len(closed), len(tc.wantClosed), closed)
			}
			for i := range closed {
				if closed[i] != tc.wantClosed[i] {
					t.Errorf("closed[%d]:\n got  %+v\n want %+v", i, closed[i], tc.wantClosed[i])
				}
			}

			if len(open) != len(tc.wantOpen) {
				t.Fatalf("open: got %d symbols, want %d", len(open), len(tc.wantOpen))
			}
			for sym, want := range tc.wantOpen {
				got, ok := open[sym]
				if !ok {
					t.Fatalf("open[%s]: missing", sym)
				}
				if *got != want {
					t.Errorf("open[%s]:\n got  %+v\n want %+v", sym, *got, want)
				}
			}
		})
	}
}
