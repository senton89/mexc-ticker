package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/senton89/mexc-ticker/internal/model"
	"github.com/senton89/mexc-ticker/internal/source"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	src := source.New("https://api.mexc.com", []string{"BTCUSDT", "ETHUSDT"}, 2*time.Second, log)
	out := make(chan model.Trade, 1024)

	errCh := make(chan error, 1)
	go func() { errCh <- src.Run(ctx, out) }()

	n := 0
	for t := range out {
		n++
		if n <= 10 {
			fmt.Printf("%s %s price=%.2f qty=%.6f\n", t.Time.Format("15:04:05.000"), t.Symbol, t.Price, t.Qty)
		}
	}
	log.Info("source stopped", "err", <-errCh, "trades", n)
}
